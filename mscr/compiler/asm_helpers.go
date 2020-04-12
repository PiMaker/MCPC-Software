package compiler

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/alecthomas/participle/lexer"
	"github.com/logrusorgru/aurora"
	"github.com/mileusna/conditional"
)

var regexpAsmExtract = regexp.MustCompile(`(?s)_asm\s*\{(.*?)\}`)
var regexpAsmExtractCmds = regexp.MustCompile(`\s*(\S+)\s*`)

func runtimeValueToAsmParam(val *RuntimeValue) *asmParam {
	// TODO: Maybe add more shortcut options?
	// FIXME: every runtime param is a calc anyway, this function should probably be a no-op in the future?
	//valueString := ""

	// if val.Eval != nil {
	// 	valueString = *val.Eval
	// } else if val.FunctionCall != nil {
	// 	valueString = val.FunctionCall.FunctionName + "("
	// 	for i, parmesan := range val.FunctionCall.Parameters {
	// 		if i > 0 {
	// 			valueString += ","
	// 		}
	// 		valueString += runtimeValueToAsmParam(parmesan).value
	// 	}
	// 	valueString += ")"
	// } else if val.Number != nil {
	// 	valueString = strconv.Itoa(*val.Number)
	// } else if val.Variable != nil {
	// 	// Shortcut for variables
	// 	return &asmParam{
	// 		asmParamType: asmParamTypeVarRead,
	// 		value:        *val.Variable,
	// 	}
	// }

	return &asmParam{
		asmParamType: asmParamTypeCalc,
		subAST:       val,
		//value:        valueString,
	}
}

func addVariable(varName string, varType string, state *asmTransformState) {
	scopeSlice, scopeExists := state.variableMap[state.currentFunction]

	asmType, ok := state.typeMap[varType]
	if !ok {
		panic(fmt.Sprintf("ERROR: Invalid type '%s' given to variable '%s' (scope: %s)", varType, varName, state.currentFunction))
	}

	newVar := &asmVar{
		name:        varName,
		orderNumber: 0,
		asmType:     asmType,
	}

	if scopeExists {

		for _, v := range scopeSlice {
			if v.name == varName {
				panic(fmt.Sprintf("ERROR: Redefinition of variable '%s' in scope '%s'", varName, state.currentFunction))
			}

			if v.orderNumber >= newVar.orderNumber {
				newVar.orderNumber = v.orderNumber + newVar.asmType.size
			}
		}

		state.variableMap[state.currentFunction] = append(scopeSlice, *newVar)
	} else {

		state.variableMap[state.currentFunction] = []asmVar{
			*newVar,
		}
	}

	state.currentScopeVariableCount++
}

func makeAsmExpression(asm string) *Expression {
	asm = fmt.Sprintf("_asm { %s }", asm)
	return &Expression{
		Asm: &asm,
		Pos: lexer.Position{
			Filename: "Meta",
		},
	}
}

func isResolved(cmds []*asmCmd) bool {
	for _, cmd := range cmds {
		for _, p := range cmd.params {
			if p.asmParamType != asmParamTypeRaw {
				return false
			}
		}
	}

	return true
}

func rawAsmParam(content string) *asmParam {
	return &asmParam{
		asmParamType: asmParamTypeRaw,
		value:        content,
	}
}

func containsInt(slice []int, value int) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}

	return false
}

func getNameForRegister(reg int, state *asmTransformState) *string {
	for name, assignedReg := range state.scopeRegisterAssignment {
		if assignedReg == reg {
			return &name
		}
	}

	return nil
}

// If 'chain' were "data.member1.member2" then 'baseType' should describe the type of "data",
// e.g. the offset calculation starts at "member1" not "data"
func getMemberInfo(chain string, baseType *asmType, scope string) (offset int, size int) {
	split := strings.Split(chain, ".")

	if len(split) <= 1 {
		return 0, baseType.size
	}

	offset = 0
	for _, member := range split[1:] {
		for _, typeMember := range baseType.members {
			if member == typeMember.name {
				recOffset, recSize := getMemberInfo(strings.Join(split[1:], "."), typeMember.asmType, scope)
				return offset + recOffset, recSize
			} else {
				offset += typeMember.asmType.size
			}
		}
	}

	panic(fmt.Sprintf("ERROR: Type '%s' does not contain a member called '%s' (scope: %s)", baseType.name, chain[1], scope))
}

func getAsmVar(name string, scope string, state *asmTransformState) (*asmVar, int) {
	nameSplit := strings.Split(name, ".")

	var avar *asmVar
	for _, v := range state.variableMap[scope] {
		if v.name == nameSplit[0] {
			avar = &v
			break
		}
	}

	if avar == nil {
		if len(nameSplit) > 1 {
			// FIXME
			panic("FIXME: Typed globals currently not supported")
		}

		// Search for global if locally scoped variabled couldn't be found
		// This is safe, because it is guaranteed at this stage that no variable can be named the same as any given global
		for gname, addr := range state.globalMemoryMap {
			if gname == "global_"+name {
				avar = &asmVar{
					name:        name,
					orderNumber: addr,
					isGlobal:    true,
					asmType:     state.typeMap["word"],
				}
			}
		}

		if avar == nil {
			panic(fmt.Sprintf("ERROR: Invalid variable name in resolve: %s (scope: %s)\n", name, scope))
		}

		return avar, 0
	} else if len(nameSplit) > 1 {
		// struct member acces, calculate offset
		offset, _ := getMemberInfo(name, avar.asmType, scope)
		return avar, offset
	}

	return avar, 0
}

// Fixes globals and strings incorrectly being detected as variable identifiers
func (cmd *asmCmd) fixGlobalAndStringParamTypes(state *asmTransformState) {
	if cmd.params != nil && len(cmd.params) > 0 {
		for _, p := range cmd.params {
			if p.asmParamType == asmParamTypeVarRead || p.asmParamType == asmParamTypeVarAddr {
				for global, addr := range state.globalMemoryMap {
					if global == "global_"+p.value {
						p.asmParamType = conditional.Int(p.asmParamType == asmParamTypeVarRead, asmParamTypeGlobalRead, asmParamTypeGlobalAddr)
						p.addrCache = addr
						break
					}
				}

				for str, addr := range state.stringMap {
					if str == "global_"+p.value {
						p.asmParamType = conditional.Int(p.asmParamType == asmParamTypeVarRead, asmParamTypeStringRead, asmParamTypeStringAddr)
						p.addrCache = addr
						break
					}
				}
			} else if p.asmParamType == asmParamTypeVarWrite {
				for global, addr := range state.globalMemoryMap {
					if global == "global_"+p.value {
						p.asmParamType = asmParamTypeGlobalWrite
						p.addrCache = addr
						break
					}
				}

				for str := range state.stringMap {
					if str == p.value {
						panic(fmt.Sprintf("ERROR: Cannot write to a string variable: '%s'", p.value))
					}
				}
			}
		}
	}
}

func typeIsWord(asmType *asmType, accessor string, scope string) bool {
	// As long as the size is 1 we can treat it as a word
	// This allows aliasing the word type
	_, size := getMemberInfo(accessor, asmType, scope)
	return size == 1
}

// Generates valid MCPC assembly from an asmCmd
func (cmd *asmCmd) asmString() string {
	retval := cmd.ins

	if strings.HasPrefix(retval, "__") {
		// Internal command, ignore
		return ""
	}

	if cmd.params != nil && len(cmd.params) > 0 {
		for _, p := range cmd.params {
			if p.asmParamType != asmParamTypeRaw {
				panic(fmt.Sprintf("ERROR: Unconverted asmParam found (type: %d, value: %v). How did you get here?\n", p.asmParamType, p))
			}

			retval += " " + p.value
		}
	}

	if cmd.comment != "" {
		retval += fmt.Sprintf(" ;%s", strings.TrimRight(cmd.comment, "\n"))
	}

	return retval
}

func getFuncLabel(node Function) string {
	return fmt.Sprintf("mscr_function_%s_params_%d", node.Name, len(node.Parameters))
}

func getFuncLabelSpecific(functionName string, parameters int) string {
	return fmt.Sprintf("mscr_function_%s_params_%d", functionName, parameters)
}

func getConditionalLabelEnd(cond Conditional) string {
	return fmt.Sprintf("mscr_cond_end_%s_%d_%d_%d", cond.Pos.Filename, cond.Pos.Line, cond.Pos.Column, cond.Pos.Offset)
}

func getConditionalLabelElse(cond Conditional) string {
	return fmt.Sprintf("mscr_cond_else_%s_%d_%d_%d", cond.Pos.Filename, cond.Pos.Line, cond.Pos.Column, cond.Pos.Offset)
}

func getWhileLoopLabelStart(cond WhileLoop) string {
	return fmt.Sprintf("mscr_while_start_%s_%d_%d_%d", cond.Pos.Filename, cond.Pos.Line, cond.Pos.Column, cond.Pos.Offset)
}

func getWhileLoopLabelEnd(cond WhileLoop) string {
	return fmt.Sprintf("mscr_while_end_%s_%d_%d_%d", cond.Pos.Filename, cond.Pos.Line, cond.Pos.Column, cond.Pos.Offset)
}

var rnumLookup = []string{
	"A",
	"B",
	"C",
	"D",
	"E",
	"F",
	"G",
	"H",
}

func toReg(rnum int) string {
	return rnumLookup[rnum]
}

// Debug information for an asmCmd in pre-formatted string form
func (cmd *asmCmd) String() string {
	return cmd.StringWithIndent(0)
}

func (cmd *asmCmd) StringWithIndent(i int) string {
	var retval string
	if strings.HasPrefix(cmd.ins, "__") {
		retval = aurora.Brown(cmd.ins).String()
	} else {
		retval = aurora.Blue(cmd.ins).String()
	}

	if cmd.params != nil && len(cmd.params) > 0 {
		for _, p := range cmd.params {
			formatted := p.value
			switch p.asmParamType {
			case asmParamTypeCalc:
				formatted = "[" + formatted + "]"
			case asmParamTypeGlobalRead:
				formatted = fmt.Sprintf("g(%s,mode=r,addr=%d)", formatted, p.addrCache)
			case asmParamTypeGlobalAddr:
				formatted = fmt.Sprintf("g(%s,mode=a,addr=%d)", formatted, p.addrCache)
			case asmParamTypeGlobalWrite:
				formatted = fmt.Sprintf("g(%s,mode=w,addr=%d)", formatted, p.addrCache)
			case asmParamTypeVarRead:
				formatted = "var(" + formatted + ",mode=r)"
			case asmParamTypeVarWrite:
				formatted = "var(" + formatted + ",mode=w)"
			case asmParamTypeVarAddr:
				formatted = "var(" + formatted + ",mode=a)"
			case asmParamTypeStringAddr:
				formatted = fmt.Sprintf("s(%s,mode=a,addr=%d)", formatted, p.addrCache)
			case asmParamTypeStringRead:
				formatted = fmt.Sprintf("s(%s,mode=r,addr=%d)", formatted, p.addrCache)
			case asmParamTypeScopeVarCount:
				formatted = "varCount(scope=" + cmd.scope + ")"
			}

			if p.asmParamType == asmParamTypeRaw {
				if strings.HasPrefix(p.value, "0x") {
					retval += " " + aurora.Brown(formatted).String()
				} else {
					retval += " " + aurora.Red(formatted).String()
				}
			} else {
				retval += " " + aurora.Magenta(formatted).String()
			}
		}
	}

	if cmd.ins == "__ASSUMESCOPE" || cmd.ins == "__FORCESCOPE" {
		retval += aurora.Magenta(fmt.Sprintf(" {var: %s, reg: %d}", cmd.scopeAnnotationName, cmd.scopeAnnotationRegister)).String()
	}

	if cmd.ins == "__SET_DIRECT" {
		retval += aurora.Magenta(fmt.Sprintf(" {var: %s}", cmd.scopeAnnotationName)).String()
	}

	if cmd.comment != "" {
		retval += aurora.Green(fmt.Sprintf("   ;%s", cmd.comment)).String()
	}

	for ind := 0; ind < cmd.printIndent; ind++ {
		retval = "  " + retval
	}

	for ind := 0; ind < i; ind++ {
		retval = "    " + retval
	}

	return retval
}
