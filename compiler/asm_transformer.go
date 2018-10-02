package compiler

import (
	"fmt"
	"log"
	"math"
	"reflect"
	"regexp"
	"strings"

	"github.com/alecthomas/participle/lexer"
)

var regexpAsmExtract = regexp.MustCompile(`(?s)_asm\s*\{(.*?)\}`)
var regexpAsmExtractCmds = regexp.MustCompile(`\s*(\S+)\s*`)

const asmParamTypeRaw = 0
const asmParamTypeVarRead = 1
const asmParamTypeVarWrite = 2
const asmParamTypeCalc = 4
const asmParamTypeGlobalWrite = 8
const asmParamTypeGlobalRead = 16
const asmParamTypeScopeVarCount = 32
const asmParamTypeStringRead = 64

type asmCmd struct {
	ins    string
	params []*asmParam

	scope string

	// For __ASSUMESCOPE and __FORCESCOPE only
	scopeAnnotationName     string
	scopeAnnotationRegister int

	comment     string
	printIndent int
}

type asmParam struct {
	asmParamType int
	value        string

	// For resolving globals and strings
	addrCache int
}

type asmTransformState struct {
	currentFunction           string
	currentScopeVariableCount int

	functionTableVar  []string
	functionTableVoid []string

	globalMemoryMap map[string]int
	maxDataAddr     int

	variableMap map[string][]asmVar
	stringMap   map[string]int

	specificInitializationAsm []*asmCmd
	binData                   []int16

	scopeRegisterAssignment map[string]int
	scopeRegisterDirty      map[int]bool

	printIndent int
}

type asmVar struct {
	name        string
	orderNumber int
	isGlobal    bool
}

func asmForNodePre(nodeInterface interface{}, state *asmTransformState) []*asmCmd {
	newAsm := make([]*asmCmd, 0)

	switch astNode := nodeInterface.(type) {

	// Function declaration
	case *Function:
		functionLabel := getFuncLabel(*astNode)
		newAsm = append(newAsm, &asmCmd{
			ins:         fmt.Sprintf(".%s __LABEL_SET", functionLabel),
			printIndent: -1,
		})
		state.currentFunction = astNode.Name

		newAsm = append(newAsm, &asmCmd{
			ins: "__CLEARSCOPE",
		})
		newAsm = append(newAsm, funcPushState(state)...)

		// Function parameter handling
		// Loop for legacy reasons
		for i := 0; i < int(math.Min(1, float64(len(astNode.Parameters)))); i++ {
			// Manually set scope via meta-instruction,
			// data is set via register parameter calling conventions
			newAsm = append(newAsm, &asmCmd{
				ins:                     "__ASSUMESCOPE",
				scopeAnnotationName:     astNode.Parameters[i],
				scopeAnnotationRegister: i,
			})

			// Register parameters as variables
			addVariable(astNode.Parameters[i], state)
		}

		for i := 1; i < len(astNode.Parameters); i++ {
			// varFromStack scopes automatically (via asmParamTypeVarWrite)
			newAsm = append(newAsm, varFromStack(astNode.Parameters[i], state)...)
			addVariable(astNode.Parameters[i], state)
		}

		state.printIndent++

	case *FunctionCall:
		// Special handling for _reg_assign
		if astNode.FunctionName == "_reg_assign" {
			if len(astNode.Parameters) != 2 {
				log.Fatalln("ERROR: A call to _reg_assign must have two parameters (register, variable). Source: " + astNode.Pos.String())
			}

			regParam := astNode.Parameters[0]
			varParam := astNode.Parameters[1]

			if regParam.Number == nil {
				log.Fatalln("ERROR: A call to _reg_assign must have a register number as its first parameter. Source: " + astNode.Pos.String())
			}

			if varParam.Ident == nil {
				log.Fatalln("ERROR: A call to _reg_assign must have a variable as its second parameter. Source: " + astNode.Pos.String())
			}

			newAsm = append(newAsm, []*asmCmd{
				&asmCmd{
					ins:                     "__FORCESCOPE",
					scopeAnnotationName:     *varParam.Ident,
					scopeAnnotationRegister: *regParam.Number,
				},
			}...)

			break
		}

		newAsm = append(newAsm, callFunc(astNode.FunctionName, astNode.Parameters, state)...)

	// Global variable
	case *Global:
		var newData []int16
		if astNode.Value != nil && astNode.Value.Text != nil {
			// String global
			state.stringMap[astNode.Name] = state.maxDataAddr

			newData = make([]int16, len(*astNode.Value.Text)+1)
			for i, c := range *astNode.Value.Text {
				newData[i] = int16(c)
			}

		} else {
			// Numerical or empty (and thus 0) initialized global
			var val int
			if astNode.Value == nil || astNode.Value.Number == nil {
				val = 0
			} else {
				val = *astNode.Value.Number
			}

			state.globalMemoryMap[astNode.Name] = state.maxDataAddr
			newData = []int16{int16(val)}
		}

		state.maxDataAddr += len(newData)
		state.binData = append(state.binData, newData...)

	case *Variable:
		addVariable(astNode.Name, state)
		if astNode.Value != nil {
			// (Take) Note: Variables without initial assignment are *not* assigned a value!
			newAsm = append(newAsm, &asmCmd{
				ins: "MOV",
				params: []*asmParam{
					runtimeValueToAsmParam(astNode.Value),
					&asmParam{
						asmParamType: asmParamTypeVarWrite,
						value:        astNode.Name,
					},
				},
			})
		}

	case *Conditional:
		/*if astNode.BodyElse != nil && len(*astNode.BodyElse) == 0 {
			log.Fatalln("Empty 'else' block not allowed (Source: " + astNode.Pos.String() + ")")
		}*/

		newAsm = append(newAsm, &asmCmd{
			ins: "JMPEZ",
			params: []*asmParam{
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        "." + getConditionalLabelElse(*astNode),
				},
				&asmParam{
					asmParamType: asmParamTypeCalc,
					value:        astNode.Condition,
				},
			},
			printIndent: -1,
		})

		ifEndAsm := "JMP ." + getConditionalLabelEnd(*astNode)

		elseStartAsm := "." + getConditionalLabelElse(*astNode)
		elseStartAsm += " __LABEL_SET"

		astNode.BodyElse = append([]*Expression{
			makeAsmExpression(ifEndAsm),
			makeAsmExpression(elseStartAsm),
		}, astNode.BodyElse...)

		state.printIndent++

	case *WhileLoop:
		newAsm = append(newAsm, &asmCmd{
			ins: fmt.Sprintf(".%s JMPEZ", getWhileLoopLabelStart(*astNode)),
			params: []*asmParam{
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        "." + getWhileLoopLabelEnd(*astNode),
				},
				&asmParam{
					asmParamType: asmParamTypeCalc,
					value:        astNode.Condition,
				},
			},
			printIndent: -1,
		})

		state.printIndent++

	case *Assignment:
		if astNode.Value == nil {
			log.Fatalln("ERROR: Cannot assign nothing as value")
		}

		if astNode.Operator == "=" {
			newAsm = append(newAsm, &asmCmd{
				ins: "MOV",
				params: []*asmParam{
					runtimeValueToAsmParam(astNode.Value),
					&asmParam{
						asmParamType: asmParamTypeVarWrite,
						value:        astNode.Name,
					},
				},
			})
		} else {
			operatorCalcParam := runtimeValueToAsmParam(astNode.Value)
			operatorCalcParam = &asmParam{
				asmParamType: asmParamTypeCalc,
				value:        fmt.Sprintf("[%s %s (%s)]", astNode.Name, astNode.Operator[0:1], operatorCalcParam.value),
			}

			newAsm = append(newAsm, &asmCmd{
				ins: "MOV",
				params: []*asmParam{
					operatorCalcParam,
					&asmParam{
						asmParamType: asmParamTypeVarWrite,
						value:        astNode.Name,
					},
				},
			})
		}

	case *Expression:
		// Raw ASM
		if astNode.Asm != nil {
			extractedAsm := strings.Split(regexpAsmExtract.FindAllStringSubmatch(*astNode.Asm, -1)[0][1], "\n")
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

					newAsm[0].params = append(newAsm[0].params, &asmParam{
						value:        cmd[1],
						asmParamType: asmParamTypeRaw,
					})
				}
			}
		} else if astNode.Return != nil {
			// Return
			newAsm = append(newAsm, &asmCmd{
				ins: "MOV",
				params: []*asmParam{
					runtimeValueToAsmParam(astNode.Return),
					&asmParam{
						asmParamType: asmParamTypeRaw,
						value:        "A",
					},
				},
			})
			newAsm = append(newAsm, funcPopState(state)...)
			newAsm = append(newAsm, &asmCmd{
				ins: "RET",
			})
		}

	case *TopExpression, *RuntimeValue, *Value, *RVFunctionCall, lexer.Position:
		// Ignored instructions (don't generate asm)
		// These are usually handled otherwise (e.g. as subexpressions of other instructions)
		break

	default:
		// Instruction unsupported, bad path
		log.Println("Instruction currently unsupported: " + reflect.TypeOf(astNode).String())
		newAsm = append(newAsm, &asmCmd{
			ins:         fmt.Sprintf(";UNSUPPORTED INSTRUCTION (%s);", reflect.TypeOf(astNode).String()),
			params:      make([]*asmParam, 0),
			printIndent: -1000000,
		})
	}

	// Set instruction scope
	for i := range newAsm {
		newAsm[i].scope = state.currentFunction
		newAsm[i].printIndent += state.printIndent
	}

	return newAsm
}

func asmForNodePost(nodeInterface interface{}, state *asmTransformState) []*asmCmd {
	switch node := nodeInterface.(type) {
	case *Function:
		state.printIndent--
		retval := make([]*asmCmd, 0)

		// For void functions, append mandatory return
		// TODO: Append only when necessary (not already present from user code)
		isVoid := false
		fLabel := getFuncLabelSpecific(node.Name, len(node.Parameters))
		for _, vf := range state.functionTableVoid {
			if vf == fLabel {
				isVoid = true
				break
			}
		}

		if isVoid {
			retval = append(retval, funcPopState(state)...)
			retval = append(retval, &asmCmd{
				ins: "RET",
			})
		}

		// Append fault
		retval = append(retval, &asmCmd{
			ins: "FAULT",
			params: []*asmParam{
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        FAULT_NO_RETURN,
				},
			},
			comment: " Ending function: " + node.Name,
		})

		// Formatting
		for _, cmd := range retval {
			cmd.printIndent = state.printIndent + 1
		}

		// Clear scope
		state.currentFunction = ""
		state.currentScopeVariableCount = 0

		return retval

	case *Conditional:
		state.printIndent--

		return []*asmCmd{
			&asmCmd{
				ins:         fmt.Sprintf(".%s __LABEL_SET", getConditionalLabelEnd(*node)),
				printIndent: state.printIndent + 1,
			}}
	case *WhileLoop:
		state.printIndent--

		return []*asmCmd{
			&asmCmd{
				ins:         fmt.Sprintf("JMP .%s", getWhileLoopLabelStart(*node)),
				printIndent: state.printIndent + 1,
			},
			&asmCmd{
				ins:         fmt.Sprintf(".%s __LABEL_SET", getWhileLoopLabelEnd(*node)),
				printIndent: state.printIndent + 1,
			},
		}
	}

	return nil
}
