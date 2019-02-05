package compiler

import (
	"fmt"
	"log"
	"reflect"

	"github.com/logrusorgru/aurora"

	"github.com/alecthomas/participle/lexer"
)

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

		// Temporarily store return address in E
		newAsm = append(newAsm, &asmCmd{
			ins: "POP",
			params: []*asmParam{
				rawAsmParam("E"),
			},
		})

		// Read parameters from stack (in reverse order)
		for i := len(astNode.Parameters) - 1; i >= 0; i-- {
			// varFromStack scopes automatically (via asmParamTypeVarWrite)
			newAsm = append(newAsm, varFromStack(astNode.Parameters[i], state)...)
			addVariable(astNode.Parameters[i], state)
		}

		// Push return address back
		newAsm = append(newAsm, &asmCmd{
			ins: "PUSH",
			params: []*asmParam{
				rawAsmParam("E"),
			},
		})

		state.printIndent++

	case *FunctionCall:
		// Function call is only used when the return value is ignored! Otherwise function calls are handled as calc expressions! (see asm_calc_resolver.go)

		// Special handling for meta-functions
		if astNode.FunctionName == "_reg_assign" {
			// _reg_assign forcibly assigns a variable to a register
			// Useful for _asm blocks
			if len(astNode.Parameters) != 2 {
				panic("ERROR: A call to _reg_assign must have two parameters (register, variable). Source: " + astNode.Pos.String())
			}

			regParam := astNode.Parameters[0]
			varParam := astNode.Parameters[1]

			if regParam.Number == nil {
				panic("ERROR: A call to _reg_assign must have a register number as its first parameter. Source: " + astNode.Pos.String())
			}

			if varParam.Ident == nil {
				panic("ERROR: A call to _reg_assign must have a variable as its second parameter. Source: " + astNode.Pos.String())
			}

			newAsm = append(newAsm, &asmCmd{
				ins:                     "__FORCESCOPE",
				scopeAnnotationName:     *varParam.Ident,
				scopeAnnotationRegister: *regParam.Number,
				comment:                 " _reg_assign",
			})

			break

		} else if astNode.FunctionName == "$$" {
			// $$ creates references in calc blocks,
			// here it sets values to an address
			// e.g. $$(0xA, 0xB) sets memory location 0xA to value 0xB

			if len(astNode.Parameters) != 2 {
				panic("ERROR: A call to $$ must have two parameters (address, value). Source: " + astNode.Pos.String())
			}

			addrParam := astNode.Parameters[0]
			valParam := astNode.Parameters[1]

			addrAsmParam := runtimeValueToAsmParam(addrParam)
			valAsmParam := runtimeValueToAsmParam(valParam)

			newAsm = append(newAsm, &asmCmd{
				ins:     "PUSH",
				comment: " call to $$",
				params: []*asmParam{
					valAsmParam,
				},
			})

			newAsm = append(newAsm, &asmCmd{
				ins:     "MOV",
				comment: " call to $$",
				params: []*asmParam{
					addrAsmParam,
					rawAsmParam("F"),
				},
			})

			newAsm = append(newAsm, &asmCmd{
				ins:     "POP",
				comment: " call to $$",
				params: []*asmParam{
					rawAsmParam("G"),
				},
			})

			newAsm = append(newAsm, &asmCmd{
				ins:     "STOR",
				comment: " call to $$",
				params: []*asmParam{
					rawAsmParam("G"),
					rawAsmParam("F"),
				},
			})

			break

		} else if astNode.FunctionName == "$" {
			// $ dereferences, only valid in calc blocks
			panic("ERROR: Cannot use special function '$' in non-value context (e.g. calling $ as a void function/standalone. Use calc context [] instead.)")
		}

		newAsm = append(newAsm, callFunc(astNode.FunctionName, astNode.Parameters, state)...)

	// Global variable
	case *Global:
		var newData []int16
		if astNode.Value != nil && astNode.Value.Text != nil {
			// String global
			state.stringMap["global_"+astNode.Name] = state.maxDataAddr

			newData = make([]int16, len(*astNode.Value.Text)+1) // len(*) + 1 automatically null-terminates the string representation (since int16 is default 0 initialized in go)
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

			state.globalMemoryMap["global_"+astNode.Name] = state.maxDataAddr
			newData = []int16{int16(val)}
		}

		state.maxDataAddr += len(newData)
		state.binData = append(state.binData, newData...)

	case *View:
		// Basically just an alias
		state.globalMemoryMap["global_"+astNode.Name] = astNode.Address

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
		// Flush scope now to start conditional body "clean"
		newAsm = append(newAsm, &asmCmd{
			ins:   "__FLUSHSCOPE",
			scope: state.currentFunction,
		})

		newAsm = append(newAsm, &asmCmd{
			ins:   "__CLEARSCOPE",
			scope: state.currentFunction,
		})

		// Conditional jump to else block
		// Note: Technically, this could check out variables again and make our __FLUSHSCOPE above useless
		// However, variables checked out in a pure calc context are always read-only, thus never allowing a dirty checkout hereafter
		newAsm = append(newAsm, &asmCmd{
			ins: "JMPEZ",
			params: []*asmParam{
				rawAsmParam("." + getConditionalLabelElse(*astNode)),
				&asmParam{
					asmParamType: asmParamTypeCalc,
					value:        astNode.Condition,
				},
			},
			printIndent: -1,
		})

		// Do a __FLUSHSCOPE before the if-jump to avoid clearing variables that have not been evicted on jump to cond-end
		ifEndAsm := "__FLUSHSCOPE\nJMP ." + getConditionalLabelEnd(*astNode)

		elseStartAsm := "." + getConditionalLabelElse(*astNode)
		elseStartAsm += " __LABEL_SET"

		clearscopeString := "_asm { __CLEARSCOPE }"

		astNode.BodyElse = append([]*Expression{
			makeAsmExpression(ifEndAsm),

			// Start else block "clean" (flush is happening above, before condition calc and check)
			&Expression{
				Asm: &clearscopeString,
			},

			makeAsmExpression(elseStartAsm),
		}, astNode.BodyElse...)

		state.printIndent++

	case *WhileLoop:
		// Flush scope now to start loop "clean"
		newAsm = append(newAsm, &asmCmd{
			ins:   "__FLUSHSCOPE",
			scope: state.currentFunction,
		})

		newAsm = append(newAsm, &asmCmd{
			ins:   "__CLEARSCOPE",
			scope: state.currentFunction,
		})

		// Add start label as __LABEL_SET to accomodate calc parameter expansion
		newAsm = append(newAsm, &asmCmd{
			ins: "." + getWhileLoopLabelStart(*astNode) + " __LABEL_SET",
		})

		// Move conditional value to F, since G might be used by __FLUSHSCOPE (and we're guaranteed not to have any calcs in there)
		newAsm = append(newAsm, &asmCmd{
			ins: "MOV",
			params: []*asmParam{
				&asmParam{
					asmParamType: asmParamTypeCalc,
					value:        astNode.Condition,
				},
				rawAsmParam("F"),
			},
		})

		newAsm = append(newAsm, &asmCmd{
			ins: "JMPEZ",
			params: []*asmParam{
				rawAsmParam("." + getWhileLoopLabelEnd(*astNode)),
				rawAsmParam("F"),
			},
		})

		state.printIndent++

	case *Assignment:
		if astNode.Value == nil {
			panic("ERROR: Cannot assign nothing as value")
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
			newAsm = append(newAsm, toRawAsm(*astNode.Asm)...)
		} else if astNode.Return != nil {
			// Return (TODO: Maybe handle void functions differently?)
			newAsm = append(newAsm, &asmCmd{
				ins: "MOV",
				params: []*asmParam{
					runtimeValueToAsmParam(astNode.Return),
					rawAsmParam("A"),
				},
				scope: state.currentFunction,
			})
			newAsm = append(newAsm, funcPopState(state)...)
			newAsm = append(newAsm, &asmCmd{
				ins:   "__FLUSHGLOBALS",
				scope: state.currentFunction,
			})
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
		log.Println(aurora.Red("WARNING: Instruction currently unsupported: " + reflect.TypeOf(astNode).String()))
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
				ins:   "__FLUSHGLOBALS",
				scope: state.currentFunction,
			})
			retval = append(retval, &asmCmd{
				ins: "RET",
			})
		}

		// Append fault
		retval = append(retval, &asmCmd{
			ins: "FAULT",
			params: []*asmParam{
				rawAsmParam(FAULT_NO_RETURN),
			},
			scope:   state.currentFunction,
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
				ins:   "__FLUSHSCOPE",
				scope: state.currentFunction,
			},
			&asmCmd{
				// Clear scope on exit of else
				ins:   "__CLEARSCOPE",
				scope: state.currentFunction,
			},
			&asmCmd{
				ins:         fmt.Sprintf(".%s __LABEL_SET", getConditionalLabelEnd(*node)),
				printIndent: state.printIndent + 1,
			}}
	case *WhileLoop:
		state.printIndent--

		return []*asmCmd{
			&asmCmd{
				ins:   "__FLUSHSCOPE",
				scope: state.currentFunction,
			},
			&asmCmd{
				ins:         fmt.Sprintf("JMP .%s", getWhileLoopLabelStart(*node)),
				printIndent: state.printIndent + 1,
			},
			&asmCmd{
				ins:         fmt.Sprintf(".%s __LABEL_SET", getWhileLoopLabelEnd(*node)),
				printIndent: state.printIndent + 1,
			},
			&asmCmd{
				ins:   "__CLEARSCOPE",
				scope: state.currentFunction,
			},
		}
	}

	return nil
}
