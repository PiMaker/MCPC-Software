package compiler

import (
	"fmt"
	"log"
	"strconv"
	"strings"
)

const AssigneableRegisters = 4

func (cmd *asmCmd) resolve(initAsm []*asmCmd, state *asmTransformState) []*asmCmd {
	output := make([]*asmCmd, 0)

	// Meta-instructions
	// Return early if they are not reflected in output ASM
	if cmd.ins == "__CLEARSCOPE" {
		// This fixes so many issues but is horribly inperformant
		// Nevermind though, sprinkle that shit all over
		// I'm not debugging my algorithm any further my dudes
		// Just liberally put workarounds all over the place
		state.scopeRegisterAssignment = make(map[string]int, 0)
		state.scopeRegisterDirty = make(map[int]bool, AssigneableRegisters)
		return output
	}

	if cmd.ins == "__ASSUMESCOPE" {
		state.scopeRegisterAssignment[cmd.scopeAnnotationName] = cmd.scopeAnnotationRegister
		state.scopeRegisterDirty[cmd.scopeAnnotationRegister] = true
		return output
	}

	if cmd.ins == "__FLUSHSCOPE" {
		// Save entire scope to VarHeap
		for i, dirty := range state.scopeRegisterDirty {
			if dirty {
				for varName, varReg := range state.scopeRegisterAssignment {
					if i == varReg {
						// Match for dirty var and corresponding register, save to heap
						output = append(output, varToHeap(getAsmVar(varName, cmd.scope, state), toReg(i), state, cmd.scope)...)
					}
				}
			}
		}
		state.scopeRegisterDirty = make(map[int]bool, AssigneableRegisters)
		return output
	}

	if cmd.ins == "__FLUSHGLOBALS" {
		// Save only globals to VarHeap
		for i, dirty := range state.scopeRegisterDirty {
			if dirty {
				for varName, varReg := range state.scopeRegisterAssignment {
					if i == varReg {
						// Match for dirty var and corresponding register, save to heap
						asmVar := getAsmVar(varName, cmd.scope, state)
						if asmVar.isGlobal {
							output = append(output, varToHeap(asmVar, toReg(i), state, cmd.scope)...)
						}
					}
				}
			}
		}
		state.scopeRegisterDirty = make(map[int]bool, AssigneableRegisters)
		return output
	}

	if cmd.ins == "__FORCESCOPE" {
		// Force variable into specific register

		found := false
		for _, v := range state.variableMap[cmd.scope] {
			if v.name == cmd.scopeAnnotationName {
				found = true
				break
			}
		}

		if !found {
			// Unknown variable
			log.Fatalf("ERROR: Tried to force unknown variable '%s' into register '%s/%d'. (Note: _reg_assign(register, variable) only works with function local variables, not globals)\n",
				cmd.scopeAnnotationName, toReg(cmd.scopeAnnotationRegister), cmd.scopeAnnotationRegister)
		}

		// Always assume dirty since probably used in ASM command
		state.scopeRegisterDirty[cmd.scopeAnnotationRegister] = true

		// Check if variable already checked out
		for varName, varReg := range state.scopeRegisterAssignment {
			if varName == cmd.scopeAnnotationName {
				if varReg == cmd.scopeAnnotationRegister {
					// Variable already present, nothing to do
					return output
				}

				// Variable present, but in wrong register
				otherName := getNameForRegister(cmd.scopeAnnotationRegister, state)
				if otherName == nil {
					//log.Println("ERROR: No name found for register: " + toReg(cmd.scopeAnnotationRegister))
					//log.Fatalln("ERROR: Variable<>Register assignment failure; Internal error, scopeRegisterAssignment map inconsistent.")

					// Wait I think I just realized this just means we need to move the var over since the target register is empty anyway
					output = append(output, []*asmCmd{
						&asmCmd{
							ins: "MOV",
							params: []*asmParam{
								rawAsmParam(toReg(varReg)),
								rawAsmParam(toReg(cmd.scopeAnnotationRegister)),
							},
						},
					}...)

					// Updates state
					state.scopeRegisterDirty[varReg] = false
					state.scopeRegisterAssignment[varName] = cmd.scopeAnnotationRegister
				} else {
					// Commence swapping
					output = append(output, []*asmCmd{
						&asmCmd{
							ins: "MOV",
							params: []*asmParam{
								rawAsmParam(toReg(cmd.scopeAnnotationRegister)),
								rawAsmParam("G"),
							},
						},
						&asmCmd{
							ins: "MOV",
							params: []*asmParam{
								rawAsmParam(toReg(varReg)),
								rawAsmParam(toReg(cmd.scopeAnnotationRegister)),
							},
						},
						&asmCmd{
							ins: "MOV",
							params: []*asmParam{
								rawAsmParam("G"),
								rawAsmParam(toReg(varReg)),
							},
						},
					}...)

					// Update state
					state.scopeRegisterAssignment[varName] = state.scopeRegisterAssignment[*otherName]
					state.scopeRegisterAssignment[*otherName] = varReg
				}

				return output
			}
		}

		// If we're here, the variable is currently not checked out
		conflictingName := getNameForRegister(cmd.scopeAnnotationRegister, state)
		if conflictingName != nil && state.scopeRegisterDirty[cmd.scopeAnnotationRegister] {
			// Target register not empty needs flushing, evict it first
			output = append(output, varToHeap(getAsmVar(*conflictingName, cmd.scope, state), toReg(cmd.scopeAnnotationRegister), state, cmd.scope)...)

			// Update state for consistency
			delete(state.scopeRegisterAssignment, *conflictingName)
		}

		// Finally load variable into correct register
		output = append(output, varFromHeap(getAsmVar(cmd.scopeAnnotationName, cmd.scope, state), toReg(cmd.scopeAnnotationRegister), state, cmd.scope)...)

		return output
	}

	// Handle calc parameters first to avoid glitches with scoped variable assignment later on
	// Self-Recursive resolving will take care of the rest
	processedCalc := false
	for _, p := range cmd.params {
		if p.asmParamType == asmParamTypeCalc {
			if processedCalc {
				log.Fatalln("ERROR: Two calc parameters found in one meta-assembly instruction, invalid state.")
			}

			// Resolve calculation to assembly
			// This will put result in "F"
			calcAsm := resolveCalc(p.value, cmd.scope, state)
			output = append(output, calcAsm...)

			p.asmParamType = asmParamTypeRaw
			p.value = "F" // F is calcOut register
			processedCalc = true
		}
	}

	// Calc found - exit early
	if processedCalc {
		// Special case of SETREG which doesn't accept registers, but could be used to set a "calc literal" to a register
		if cmd.ins == "SETREG" {
			cmd.ins = "MOV"
			cmd.params[0], cmd.params[1] = cmd.params[1], cmd.params[0]
		}

		return append(output, cmd)
	}

	// Parameter translation (meta asm->real asm)
	cmdAssignedRegisters := make([]int, 0)
	for _, p := range cmd.params {
		switch p.asmParamType {
		case asmParamTypeScopeVarCount:
			p.asmParamType = asmParamTypeCalc
			p.value = fmt.Sprintf("[%d]", len(state.variableMap[cmd.scope]))

		// Variable/Global access
		case asmParamTypeVarRead, asmParamTypeVarWrite, asmParamTypeGlobalRead, asmParamTypeGlobalWrite:

			//if p.asmParamType == asmParamTypeGlobalRead {
			//	fmt.Println("DEBUG")
			//}

			asmVar := getAsmVar(p.value, cmd.scope, state)

			// Check if variable already checked out into register
			found := false
			for varName, varReg := range state.scopeRegisterAssignment {
				if varName == asmVar.name {
					// Found
					p.value = toReg(varReg)

					// Mark dirty on write
					state.scopeRegisterDirty[varReg] = p.asmParamType == asmParamTypeVarWrite || p.asmParamType == asmParamTypeGlobalWrite

					found = true
				}
			}

			if found {
				p.asmParamType = asmParamTypeRaw
				break
			}

			// Assign register to variable
			reg := asmVar.orderNumber % AssigneableRegisters
			startReg := reg
			for containsInt(cmdAssignedRegisters, reg) {
				reg = (reg + 1) % AssigneableRegisters
				if reg == startReg {
					log.Fatalf("ERROR: Variable<>Register assignment failure; Internal error, too many variables attached to one meta-asm command. In scope: %s\n", cmd.scope)
				}
			}

			p.value = toReg(reg)
			cmdAssignedRegisters = append(cmdAssignedRegisters, reg)

			// If marked dirty, flush to VarHeap before loading new value
			if state.scopeRegisterDirty[reg] {
				nameForReg := getNameForRegister(reg, state)
				if nameForReg == nil {
					log.Fatalln("ERROR: Variable<>Register assignment failure; Internal error, scopeRegisterAssignment map inconsistent with register dirty state.")
				}

				output = append(output, varToHeap(getAsmVar(*nameForReg, cmd.scope, state), toReg(reg), state, cmd.scope)...)

				// Update state for consistency
				delete(state.scopeRegisterAssignment, *nameForReg)
			}

			// Set dirty on write
			state.scopeRegisterDirty[reg] = p.asmParamType == asmParamTypeVarWrite || p.asmParamType == asmParamTypeGlobalWrite

			// Load value (only on read, on write it will be overwritten anyway)
			if p.asmParamType == asmParamTypeVarRead || p.asmParamType == asmParamTypeGlobalRead {
				output = append(output, varFromHeap(asmVar, toReg(reg), state, cmd.scope)...)
			}

			// Update state
			state.scopeRegisterAssignment[asmVar.name] = reg

			// Set paramType to raw last to avoid errors above
			p.asmParamType = asmParamTypeRaw

		case asmParamTypeStringRead:
			// This is always a pointer to the start of the specfied string data
			p.asmParamType = asmParamTypeCalc
			p.value = strconv.Itoa(state.stringMap[p.value])

		case asmParamTypeCalc:
			log.Fatalln("ERROR: Calc parameter was not resolved. This is most likely a compiler error.")
		}
	}

	// Note to self: Any change above here but below for loop should be reflected in early exits as well (especially calc early-out)
	return append(output, cmd)
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

func getAsmVar(name string, scope string, state *asmTransformState) *asmVar {
	var avar *asmVar
	for _, v := range state.variableMap[scope] {
		if v.name == name {
			avar = &v
			break
		}
	}

	if avar == nil {
		// Search for global if locally scoped variabled couldn't be found
		// This is safe, because it is guaranteed at this stage that no variable can be named the same as any given global
		for gname, addr := range state.globalMemoryMap {
			if gname == name {
				avar = &asmVar{
					name:        name,
					orderNumber: addr,
					isGlobal:    true,
				}
			}
		}

		if avar == nil {
			log.Fatalf("ERROR: Invalid variable name in resolve: %s (scope: %s)\n", name, scope)
		}
	}

	return avar
}

func varToHeap(v *asmVar, register string, state *asmTransformState, cmdScope string) []*asmCmd {
	if v.isGlobal {
		return []*asmCmd{
			&asmCmd{
				ins: "SETREG",
				params: []*asmParam{
					&asmParam{
						asmParamType: asmParamTypeRaw,
						value:        "G",
					},
					&asmParam{
						asmParamType: asmParamTypeRaw,
						value:        fmt.Sprintf("0x%x", v.orderNumber), // orderNumber of global is memory address directly
					},
				},
				scope: cmdScope,
			},
			&asmCmd{
				ins: "STOR",
				params: []*asmParam{
					&asmParam{
						asmParamType: asmParamTypeRaw,
						value:        register,
					},
					&asmParam{
						asmParamType: asmParamTypeRaw,
						value:        "G",
					},
				},
				scope: cmdScope,
			},
		}
	}

	return []*asmCmd{
		&asmCmd{
			ins: "SETREG",
			params: []*asmParam{
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        "G",
				},
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        fmt.Sprintf("0x%x", v.orderNumber),
				},
			},
			scope: cmdScope,
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
					value:        "G",
				},
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        "G",
				},
			},
			scope: cmdScope,
		},
		&asmCmd{
			ins: "STOR",
			params: []*asmParam{
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        register,
				},
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        "G",
				},
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
		SETREG G <orderNumber alias address>
		STOR <register> G
	*/
}

func varFromHeap(v *asmVar, register string, state *asmTransformState, cmdScope string) []*asmCmd {
	if v.isGlobal {
		// For doc on global handling see varToHeap
		return []*asmCmd{
			&asmCmd{
				ins: "SETREG",
				params: []*asmParam{
					&asmParam{
						asmParamType: asmParamTypeRaw,
						value:        "G",
					},
					&asmParam{
						asmParamType: asmParamTypeRaw,
						value:        fmt.Sprintf("0x%x", v.orderNumber),
					},
				},
				scope: cmdScope,
			},
			&asmCmd{
				ins: "LOAD",
				params: []*asmParam{
					&asmParam{
						asmParamType: asmParamTypeRaw,
						value:        register,
					},
					&asmParam{
						asmParamType: asmParamTypeRaw,
						value:        "G",
					},
				},
				scope: cmdScope,
			},
		}
	}

	return []*asmCmd{
		&asmCmd{
			ins: "SETREG",
			params: []*asmParam{
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        "G",
				},
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        fmt.Sprintf("0x%x", v.orderNumber),
				},
			},
			scope: cmdScope,
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
					value:        "G",
				},
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        "G",
				},
			},
			scope: cmdScope,
		},
		&asmCmd{
			ins: "LOAD",
			params: []*asmParam{
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        register,
				},
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        "G",
				},
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

// Fixes globals and strings incorrectly being detected as variable identifiers
func (cmd *asmCmd) fixGlobalAndStringParamTypes(state *asmTransformState) {
	if cmd.params != nil && len(cmd.params) > 0 {
		for _, p := range cmd.params {
			if p.asmParamType == asmParamTypeVarRead {
				for global, addr := range state.globalMemoryMap {
					if global == p.value {
						p.asmParamType = asmParamTypeGlobalRead
						p.addrCache = addr
						break
					}
				}

				for str, addr := range state.stringMap {
					if str == p.value {
						p.asmParamType = asmParamTypeStringRead
						p.addrCache = addr
						break
					}
				}
			} else if p.asmParamType == asmParamTypeVarWrite {
				for global, addr := range state.globalMemoryMap {
					if global == p.value {
						p.asmParamType = asmParamTypeGlobalWrite
						p.addrCache = addr
						break
					}
				}

				for str := range state.stringMap {
					if str == p.value {
						log.Fatalf("ERROR: Cannot write to a string variable: '%s'", p.value)
					}
				}
			}
		}
	}
}

// Generates valid MCPC assembly from an asmCmd
func (cmd *asmCmd) asmString() string {
	retval := cmd.ins

	if cmd.params != nil && len(cmd.params) > 0 {
		for _, p := range cmd.params {
			if p.asmParamType != asmParamTypeRaw {
				log.Fatalf("Unconverted asmParam found (type: %d, value: %v). How did you get here?\n", p.asmParamType, p)
			}

			retval += " " + p.value
		}
	}

	if cmd.comment != "" {
		retval += fmt.Sprintf(" ;%s", strings.TrimRight(cmd.comment, "\n"))
	}

	return retval
}

// Debug information for an asmCmd in pre-formatted string form
func (cmd *asmCmd) String() string {
	retval := cmd.ins

	if cmd.params != nil && len(cmd.params) > 0 {
		for _, p := range cmd.params {
			formatted := p.value
			switch p.asmParamType {
			case asmParamTypeCalc:
				formatted = "[" + formatted + "]"
			case asmParamTypeGlobalRead:
				formatted = fmt.Sprintf("g(%s,r,addr=%d)", formatted, p.addrCache)
			case asmParamTypeGlobalWrite:
				formatted = fmt.Sprintf("g(%s,w,addr=%d)", formatted, p.addrCache)
			case asmParamTypeVarRead:
				formatted = "var(" + formatted + ",r)"
			case asmParamTypeVarWrite:
				formatted = "var(" + formatted + ",w)"
			case asmParamTypeScopeVarCount:
				formatted = "varCount(scope=" + cmd.scope + ")"
			case asmParamTypeStringRead:
				formatted = fmt.Sprintf("s(%s,r,addr=%d)", formatted, p.addrCache)
			}
			retval += " " + formatted
		}
	}

	if cmd.ins == "__ASSUMESCOPE" || cmd.ins == "__FORCESCOPE" {
		retval += fmt.Sprintf(" {var: %s, reg: %d}", cmd.scopeAnnotationName, cmd.scopeAnnotationRegister)
	}

	if cmd.comment != "" {
		retval += fmt.Sprintf(" ;%s", cmd.comment)
	}

	for ind := 0; ind < cmd.printIndent; ind++ {
		retval = "  " + retval
	}

	return retval
}
