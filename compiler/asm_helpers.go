package compiler

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/alecthomas/participle/lexer"
)

var regexpAsmExtract = regexp.MustCompile(`(?s)_asm\s*\{(.*?)\}`)
var regexpAsmExtractCmds = regexp.MustCompile(`\s*(\S+)\s*`)

func toRawAsm(asm string) []*asmCmd {
	newAsm := make([]*asmCmd, 0)
	extractedAsm := strings.Split(regexpAsmExtract.FindAllStringSubmatch(asm, -1)[0][1], "\n")
	for _, line := range extractedAsm {
		lineCmdMatches := regexpAsmExtractCmds.FindAllStringSubmatch(line, -1)
		if len(lineCmdMatches) == 0 {
			continue
		}

		newAsm = append(newAsm, &asmCmd{
			ins:    lineCmdMatches[0][1],
			params: make([]*asmParam, 0),
		})
		for i, cmd := range lineCmdMatches {
			if i == 0 {
				continue
			}

			newAsm[len(newAsm)-1].params = append(newAsm[len(newAsm)-1].params, &asmParam{
				value:        cmd[1],
				asmParamType: asmParamTypeRaw,
			})
		}
	}

	return newAsm
}

func callFunc(funcName string, parameters []*RuntimeValue, state *asmTransformState) []*asmCmd {
	retval := make([]*asmCmd, 0)

	// Push parameters to stack
	for i := 0; i < len(parameters); i++ {
		paramAsAsmCalc := runtimeValueToAsmParam(parameters[i])
		retval = append(retval, &asmCmd{
			ins: "PUSH",
			params: []*asmParam{
				paramAsAsmCalc,
			},
		})
	}

	retval = append(retval, &asmCmd{
		ins: "__FLUSHSCOPE",
	})

	retval = append(retval, &asmCmd{
		ins: "__CLEARSCOPE",
	})

	fLabel := getFuncLabelSpecific(funcName, len(parameters))
	function := ""
	//isVar := false
	for _, varFunc := range state.functionTableVar {
		if varFunc == fLabel {
			//isVar = true
			function = varFunc
			break
		}
	}

	if function == "" {
		for _, voidFunc := range state.functionTableVoid {
			if voidFunc == fLabel {
				function = voidFunc
				break
			}
		}

		if function == "" {
			log.Printf("WARNING: Cannot find function to call: Function '%s' with %d parameters (Assuming extern function)\n", funcName, len(parameters))
			function = fLabel
		}
	}

	retval = append(retval, &asmCmd{
		ins: "CALL",
		params: []*asmParam{
			&asmParam{
				asmParamType: asmParamTypeRaw,
				value:        "." + function,
			},
		},
	})

	return append(retval, &asmCmd{
		ins: "__CLEARSCOPE",
	})
}

func runtimeValueToAsmParam(val *RuntimeValue) *asmParam {
	// TODO: Maybe add more shortcut options?
	valueString := ""

	if val.Eval != nil {
		valueString = *val.Eval
	} else if val.FunctionCall != nil {
		valueString = val.FunctionCall.FunctionName + "("
		for i, parmesan := range val.FunctionCall.Parameters {
			if i > 0 {
				valueString += ","
			}
			valueString += runtimeValueToAsmParam(parmesan).value
		}
		valueString += ")"
	} else if val.Number != nil {
		valueString = strconv.Itoa(*val.Number)
	} else if val.Ident != nil {
		// Shortcut for variables
		return &asmParam{
			asmParamType: asmParamTypeVarRead,
			value:        *val.Ident,
		}
	}

	// For now, make everything a calc, because simplicity and fucking over my future self who'll have to implement calc parsing
	return &asmParam{
		asmParamType: asmParamTypeCalc,
		value:        valueString,
	}
}

func funcPushState(state *asmTransformState) []*asmCmd {

	return []*asmCmd{
		&asmCmd{
			ins: "SETREG",
			params: []*asmParam{
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        "G",
				},
				&asmParam{
					asmParamType: asmParamTypeScopeVarCount,
					value:        state.currentFunction,
				},
			},
		},
		&asmCmd{
			ins: "ADD",
			params: []*asmParam{
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        "G",
				},
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        "H",
				},
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        "H",
				},
			},
		},
	}

	/*
		ADD <scopeVarCount> H H
	*/
}

func funcPopState(state *asmTransformState) []*asmCmd {

	return []*asmCmd{
		&asmCmd{
			ins: "SETREG",
			params: []*asmParam{
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        "G",
				},
				&asmParam{
					asmParamType: asmParamTypeScopeVarCount,
					value:        state.currentFunction,
				},
			},
		},
		&asmCmd{
			ins: "SUB",
			params: []*asmParam{
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        "H",
				},
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        "H",
				},
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        "G",
				},
			},
		},
	}

	/*
		SUB H H <scopeVarCount>
	*/
}

func varToStack(varName string, state *asmTransformState) []*asmCmd {
	return []*asmCmd{
		&asmCmd{
			ins: "PUSH",
			params: []*asmParam{
				&asmParam{
					asmParamType: asmParamTypeVarRead,
					value:        varName,
				},
			},
		},
	}
}

func varFromStack(varName string, state *asmTransformState) []*asmCmd {
	return []*asmCmd{
		&asmCmd{
			ins: "POP",
			params: []*asmParam{
				&asmParam{
					asmParamType: asmParamTypeVarWrite,
					value:        varName,
				},
			},
		},
	}
}

func addVariable(varName string, state *asmTransformState) {
	scopeSlice, scopeExists := state.variableMap[state.currentFunction]
	if scopeExists {
		newVar := &asmVar{
			name:        varName,
			orderNumber: 0,
		}

		for _, v := range scopeSlice {
			if v.name == varName {
				log.Fatalf("ERROR: Redefinition of variable '%s' in scope '%s'", varName, state.currentFunction)
			}

			if v.orderNumber >= newVar.orderNumber {
				newVar.orderNumber = v.orderNumber + 1
			}
		}

		/*for g := range state.globalMemoryMap {
			if g[len("global_"):] == varName {
				log.Fatalf("ERROR: Redefinition of global as variable '%s' in scope '%s'", varName, state.currentFunction)
			}
		}*/

		state.variableMap[state.currentFunction] = append(scopeSlice, *newVar)
	} else {

		/*for g := range state.globalMemoryMap {
			if g[len("global_"):] == varName {
				log.Fatalf("ERROR: Redefinition of global as variable '%s' in scope '%s'", varName, state.currentFunction)
			}
		}*/

		state.variableMap[state.currentFunction] = []asmVar{
			asmVar{
				name:        varName,
				orderNumber: 0,
			},
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
