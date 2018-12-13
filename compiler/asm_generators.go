package compiler

import (
	"fmt"
	"log"
	"strings"
)

func varToHeap(v *asmVar, register string, state *asmTransformState, cmdScope string) []*asmCmd {
	if v.isGlobal {
		return []*asmCmd{
			&asmCmd{
				ins: "SETREG",
				params: []*asmParam{
					rawAsmParam("G"),
					rawAsmParam(fmt.Sprintf("0x%x", v.orderNumber)), // orderNumber of global is memory address directly
				},
				scope: cmdScope,
			},
			&asmCmd{
				ins: "STOR",
				params: []*asmParam{
					rawAsmParam(register),
					rawAsmParam("G"),
				},
				scope: cmdScope,
			},
		}
	}

	return []*asmCmd{
		&asmCmd{
			ins: "SETREG",
			params: []*asmParam{
				rawAsmParam("G"),
				rawAsmParam(fmt.Sprintf("0x%x", v.orderNumber)),
			},
			scope: cmdScope,
		},
		&asmCmd{
			ins: "SUB",
			params: []*asmParam{
				rawAsmParam("H"),
				rawAsmParam("G"),
				rawAsmParam("G"),
			},
			scope: cmdScope,
		},
		&asmCmd{
			ins: "STOR",
			params: []*asmParam{
				rawAsmParam(register),
				rawAsmParam("G"),
			},
			scope: cmdScope,
		},
	}

	/*
		; Non-global case:
		SETREG G <orderNumber>
		SUB H G G
		STOR <register> G

		; Global case
		SETREG G <orderNumber aka address>
		STOR <register> G
	*/
}

func varFromHeap(v *asmVar, register string, state *asmTransformState, cmdScope string) []*asmCmd {
	if v.isGlobal {
		// For (more-ish) doc on global handling see varToHeap above
		return []*asmCmd{
			&asmCmd{
				ins: "SETREG",
				params: []*asmParam{
					rawAsmParam("G"),
					rawAsmParam(fmt.Sprintf("0x%x", v.orderNumber)),
				},
				scope: cmdScope,
			},
			&asmCmd{
				ins: "LOAD",
				params: []*asmParam{
					rawAsmParam(register),
					rawAsmParam("G"),
				},
				scope: cmdScope,
			},
		}
	}

	return []*asmCmd{
		&asmCmd{
			ins: "SETREG",
			params: []*asmParam{
				rawAsmParam("G"),
				rawAsmParam(fmt.Sprintf("0x%x", v.orderNumber)),
			},
			scope: cmdScope,
		},
		&asmCmd{
			ins: "SUB",
			params: []*asmParam{
				rawAsmParam("H"),
				rawAsmParam("G"),
				rawAsmParam("G"),
			},
			scope: cmdScope,
		},
		&asmCmd{
			ins: "LOAD",
			params: []*asmParam{
				rawAsmParam(register),
				rawAsmParam("G"),
			},
			scope: cmdScope,
		},
	}

	/*
		SETREG G <orderNumber>
		SUB H G G
		LOAD <register> G
	*/
}

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

			newAsm[len(newAsm)-1].params = append(newAsm[len(newAsm)-1].params, rawAsmParam(cmd[1]))
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
			rawAsmParam("." + function),
		},
	})

	return append(retval, &asmCmd{
		ins: "__CLEARSCOPE",
	})
}

func funcPushState(state *asmTransformState) []*asmCmd {

	return []*asmCmd{
		&asmCmd{
			ins: "SETREG",
			params: []*asmParam{
				rawAsmParam("G"),
				&asmParam{
					asmParamType: asmParamTypeScopeVarCount,
					value:        state.currentFunction,
				},
			},
		},
		&asmCmd{
			ins: "ADD",
			params: []*asmParam{
				rawAsmParam("G"),
				rawAsmParam("H"),
				rawAsmParam("H"),
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
				rawAsmParam("G"),
				&asmParam{
					asmParamType: asmParamTypeScopeVarCount,
					value:        state.currentFunction,
				},
			},
		},
		&asmCmd{
			ins: "SUB",
			params: []*asmParam{
				rawAsmParam("H"),
				rawAsmParam("H"),
				rawAsmParam("G"),
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

func evictRegister(reg int, scope string, state *asmTransformState) []*asmCmd {
	nameForReg := getNameForRegister(reg, state)
	if nameForReg == nil {
		panic("HELP")
		log.Fatalln("ERROR: Variable<>Register assignment failure; Internal error, scopeRegisterAssignment map inconsistent with register dirty state. (Tried to evict register with no variable assigned)")
	}

	return varToHeap(getAsmVar(*nameForReg, scope, state), toReg(reg), state, scope)
}
