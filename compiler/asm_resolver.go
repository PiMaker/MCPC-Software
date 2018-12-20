package compiler

import (
	"fmt"
	"log"
	"strconv"
)

func (cmd *asmCmd) resolve(initAsm []*asmCmd, state *asmTransformState) []*asmCmd {
	output := make([]*asmCmd, 0)

	// Meta-instructions
	// Return early if they are not reflected in output ASM
	// (We return the original instruction for verbose printing, it will not actually show up in raw asm output)
	if cmd.ins == "__CLEARSCOPE" {
		// This fixes so many issues but is horribly inperformant
		// Nevermind though, sprinkle that shit all over
		// I'm not debugging my algorithm any further my dudes
		// Just liberally put workarounds all over the place
		// (Also used for scope initialization BTW)
		state.scopeRegisterAssignment = make(map[string]int, 0)
		state.scopeRegisterDirty = make(map[int]bool, AssigneableRegisters)
		state.scopeVariableDirectMarks = make(map[string]bool, 0)
		return append([]*asmCmd{cmd}, output...)
	}

	if cmd.ins == "__ASSUMESCOPE" {
		state.scopeRegisterAssignment[cmd.scopeAnnotationName] = cmd.scopeAnnotationRegister
		state.scopeRegisterDirty[cmd.scopeAnnotationRegister] = true
		return append([]*asmCmd{cmd}, output...)
	}

	if cmd.ins == "__SET_DIRECT" {
		asmVar := getAsmVar(cmd.scopeAnnotationName, cmd.scope, state)
		if asmVar.isGlobal {
			// Variables are implicitly directly-assigned, no further action needed
			return append([]*asmCmd{cmd}, output...)
		}

		// Mark variable name as directly-assigned in current scope
		state.scopeVariableDirectMarks[cmd.scopeAnnotationName] = true

		// Check if variable currently checked out dirty, if so immediately evict back
		for varName, varReg := range state.scopeRegisterAssignment {
			if varName == cmd.scopeAnnotationName {
				// Checked out
				if state.scopeRegisterDirty[varReg] {
					// Dirty, evict and reset dirty state
					// (Not the checked out state however, we don't deliberately clear the register)
					evictionAsm := evictRegister(varReg, cmd.scope, state)
					evictionAsm[0].comment += " (reg_alloc: __SET_DIRECT forced eviction)"
					output = append(output, evictionAsm...)
					break // Stop searching
				}
			}
		}

		return append([]*asmCmd{cmd}, output...)
	}

	if cmd.ins == "__FLUSHSCOPE" {
		// Save entire scope to VarHeap
		for i, dirty := range state.scopeRegisterDirty {
			if dirty {
				for varName, varReg := range state.scopeRegisterAssignment {
					if i == varReg {
						// Match for dirty var and corresponding register, save to heap
						toAppend := varToHeap(getAsmVar(varName, cmd.scope, state), toReg(i), state, cmd.scope)
						for _, a := range toAppend {
							a.comment = fmt.Sprintf(" __FLUSHSCOPE (flushing: %s)", varName)
						}
						output = append(output, toAppend...)
					}
				}
			}
		}
		state.scopeRegisterDirty = make(map[int]bool, AssigneableRegisters)
		return append([]*asmCmd{cmd}, output...)
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
		return append([]*asmCmd{cmd}, output...)
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
					return append([]*asmCmd{cmd}, output...)
				}

				// Variable present, but in wrong register - check target register
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

				return append([]*asmCmd{cmd}, output...)
			}
		}

		// If we're here, the variable is currently not checked out
		conflictingName := getNameForRegister(cmd.scopeAnnotationRegister, state)
		if conflictingName != nil && state.scopeRegisterDirty[cmd.scopeAnnotationRegister] {
			// Target register not empty: Needs flushing, evict it first
			output = append(output, varToHeap(getAsmVar(*conflictingName, cmd.scope, state), toReg(cmd.scopeAnnotationRegister), state, cmd.scope)...)

			// Update state for consistency
			delete(state.scopeRegisterAssignment, *conflictingName)
		}

		// Finally load variable into correct register
		output = append(output, varFromHeap(getAsmVar(cmd.scopeAnnotationName, cmd.scope, state), toReg(cmd.scopeAnnotationRegister), state, cmd.scope)...)

		return append([]*asmCmd{cmd}, output...)
	}

	// Handle calc parameters first to avoid glitches with scoped variable assignment later on
	// Self-recursive resolving will take care of the rest
	processedCalc := false
	for _, p := range cmd.params {
		if p.asmParamType == asmParamTypeCalc {
			if processedCalc {
				// This would be very bad to allow, since a calc expression prepended to a regular asmCmd assumes full control over calc registers (especially F)
				// Thus, two calc-resolvings for one asmCmd would overwrite register F with whichever paramTypeCalc parameters is resolved later
				// This scenario should however never happen (in theory anyway)
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

	// Calc found - exit early, recursive resolving will save the day as always
	if processedCalc {
		// Special case of SETREG which doesn't accept registers, but could be used to set a "calc literal" to a register
		if cmd.ins == "SETREG" {
			cmd.ins = "MOV"
			cmd.params[0], cmd.params[1] = cmd.params[1], cmd.params[0]
		}

		return append(output, cmd)
	}

	postCmdAsm := make([]*asmCmd, 0)

	// Parameter translation (meta asm (variables/calc expressions)->real asm (registers/literals))
	cmdAssignedRegisters := make([]int, 0)
	for paramNum, p := range cmd.params {
		switch p.asmParamType {
		case asmParamTypeScopeVarCount:
			if cmd.ins == "SETREG" && paramNum == 1 && cmd.params[0].asmParamType == asmParamTypeRaw {
				// Special case for which we do not need any calcs
				p.asmParamType = asmParamTypeRaw
				p.value = fmt.Sprintf("0x%x", len(state.variableMap[p.value]))
			} else {
				// General case, let recursive resolving take care of it
				p.asmParamType = asmParamTypeCalc
				p.value = fmt.Sprintf("[%d]", len(state.variableMap[p.value]))
			}

		case asmParamTypeStringRead:
			// This is always a pointer to the start of the specfied string data
			p.asmParamType = asmParamTypeCalc
			p.value = strconv.Itoa(state.stringMap["global_"+p.value])

		case asmParamTypeCalc:
			log.Fatalln("ERROR: Calc parameter was not resolved early. This is most likely a compiler error.")

		// Address-type parameters
		case asmParamTypeGlobalAddr:
			// Easy mode
			p.asmParamType = asmParamTypeCalc
			p.value = strconv.Itoa(state.globalMemoryMap["global_"+p.value])

		case asmParamTypeStringAddr:
			// Not sure what this would do, let's just disallow it altogether
			log.Fatalln("ERROR: A 'string' global is already a pointer. Please first check out the string into a variable before creating a pointer-pointer.")

		case asmParamTypeVarAddr:
			// Alright, this is the tricky part
			// Recall that a "var" address is calculated from the varheap pointer minus it's order number
			output = append(output, []*asmCmd{
				&asmCmd{
					ins: "SETREG",
					params: []*asmParam{
						rawAsmParam("G"),
						rawAsmParam(fmt.Sprintf("0x%x", getAsmVar(p.value, cmd.scope, state).orderNumber)),
					},
					scope: cmd.scope,
				},
				&asmCmd{
					ins: "SUB",
					params: []*asmParam{
						rawAsmParam("H"),
						rawAsmParam("F"), // Output
						rawAsmParam("G"),
					},
					scope: cmd.scope,
				}}...)

			p.asmParamType = asmParamTypeRaw
			p.value = "F" // F is output of calculation above

		// Variable/Global access (aka. the big stuff)
		case asmParamTypeVarRead, asmParamTypeVarWrite, asmParamTypeGlobalRead, asmParamTypeGlobalWrite:

			asmVar := getAsmVar(p.value, cmd.scope, state)

			// Check if variable already checked out into register

			directlyAssigned, ok := state.scopeVariableDirectMarks[asmVar.name]
			if p.asmParamType == asmParamTypeGlobalWrite || p.asmParamType == asmParamTypeGlobalRead || (ok && directlyAssigned) {
				// If directly assigned, do not search for checked out var in registers, since it won't be up-to-date anyway
				cmd.comment += " (reg_alloc: skipping scope search, directly-assigned)"
			} else {
				found := false
				for varName, varReg := range state.scopeRegisterAssignment {
					if varName == asmVar.name {
						// Found
						p.value = toReg(varReg)

						cmd.comment += fmt.Sprintf(" (reg_alloc: var found checked out in %d)", varReg)

						// Mark dirty on write
						if p.asmParamType == asmParamTypeVarWrite || p.asmParamType == asmParamTypeGlobalWrite {
							// We know it's not a directly-assigned var or global, since otherwise we wouldn't even bother searching
							state.scopeRegisterDirty[varReg] = true
						}

						found = true
					}
				}

				if found {
					p.asmParamType = asmParamTypeRaw
					break
				}
			}

			// Assign register to variable
			// TODO (maybe): Analyze which register has been checked out the longest ago and use that? Better heuristic in general?
			reg := asmVar.orderNumber % AssigneableRegisters

			// First, check if we have a random free register available
			freeAssigned := false
			for regNum := 0; regNum < AssigneableRegisters; regNum++ {
				if !containsInt(cmdAssignedRegisters, regNum) {
					if dirty, ok := state.scopeRegisterDirty[regNum]; !ok || !dirty {
						reg = regNum
						freeAssigned = true
					}
				}
			}

			if !freeAssigned {
				// Otherwise, select candidate for eviction
				startReg := reg
				for containsInt(cmdAssignedRegisters, reg) {
					reg = (reg + 1) % AssigneableRegisters
					if reg == startReg {
						log.Fatalf("ERROR: Variable<>Register assignment failure; Internal error, too many variables attached to one meta-asm command. In scope: %s\n", cmd.scope)
					}
				}
			}

			p.value = toReg(reg)
			cmdAssignedRegisters = append(cmdAssignedRegisters, reg)

			// If marked dirty, evict to VarHeap before loading new value
			if state.scopeRegisterDirty[reg] {
				output = append(output, evictRegister(reg, cmd.scope, state)...)
			}

			// Check if anything was checked out into the assigned register beforehand, and if so, remove it from the assignment map
			toRemove := make([]string, 0)
			for otherVar, otherReg := range state.scopeRegisterAssignment {
				if otherReg == reg {
					toRemove = append(toRemove, otherVar)
					state.scopeRegisterDirty[reg] = false
					break
				}
			}

			for _, tr := range toRemove {
				delete(state.scopeRegisterAssignment, tr)
			}

			// Update state (right now, since next step calls evictRegister in some circumstances)
			state.scopeRegisterAssignment[asmVar.name] = reg

			// Set dirty on write
			if p.asmParamType == asmParamTypeVarWrite || p.asmParamType == asmParamTypeGlobalWrite {
				// Skip this and instead insert a write-back directly after if we are dealing with a directly-assigned variable or global
				directlyAssigned, ok := state.scopeVariableDirectMarks[asmVar.name]
				if p.asmParamType == asmParamTypeGlobalWrite || (ok && directlyAssigned) {
					state.scopeRegisterDirty[reg] = false
					postCmdAsm = append(evictRegister(reg, cmd.scope, state), postCmdAsm...)
					postCmdAsm[0].comment += " (reg_alloc: directly assigned, evicting back immediately)"
				} else {
					state.scopeRegisterDirty[reg] = true
				}
			}

			if state.scopeRegisterDirty[reg] {
				cmd.comment += fmt.Sprintf(" (reg_alloc: checked out as dirty)")
			} else {
				cmd.comment += fmt.Sprintf(" (reg_alloc: checked out as clean)")
			}

			// Load value (only on read, on write it will be overwritten anyway)
			if p.asmParamType == asmParamTypeVarRead || p.asmParamType == asmParamTypeGlobalRead {
				output = append(output, varFromHeap(asmVar, toReg(reg), state, cmd.scope)...)
			}

			// Set paramType to raw last to avoid errors above
			p.asmParamType = asmParamTypeRaw
		}
	}

	// Note to self: Any change above here but below for-loop should probably be reflected in early exits as well (especially calc early-out)
	return append(output, append([]*asmCmd{cmd}, postCmdAsm...)...)
}
