package compiler

import (
	"fmt"
	"log"
	"reflect"
	"regexp"
	"strings"
	"unicode"

	"github.com/PiMaker/MCPC-Software/constants"
	"github.com/logrusorgru/aurora"
)

func (ast *AST) GenerateASM(bootloader, verbose, optimizeDisable bool) string {

	if bootloader {
		log.Println("! Using bootloader mode !")
	}

	// DEBUG
	if verbose {
		log.Println("DEBUG OUTPUT (AST):")
		fmt.Println(aurora.Cyan("*AST"))
		printBody := false
		walkInterface(ast, func(val reflect.Value, name string, depth int) {
			if name != "Pos" && name != "Filename" && name != "Offset" && name != "Line" && name != "Column" {
				if name == "Body" {
					if !printBody {
						return
					}

					printBody = false
				}

				for i := 0; i < depth+1; i++ {
					fmt.Print("  ")
				}
				fmt.Print(aurora.Cyan(name).String())

				for val.Kind() == reflect.Ptr {
					val = val.Elem()
				}

				if val.Kind() == reflect.Struct {
					fmt.Println()

					// Func entry detection
					if val.Type().Name() == "Function" {
						printBody = true
					}

				} else if val.Kind() == reflect.Int {
					fmt.Print(": ")
					fmt.Println(val.Int())
				} else if val.Kind() == reflect.Bool {
					fmt.Print(": ")
					fmt.Println(val.Bool())
				} else {
					fmt.Print(": ")
					fmt.Println(val.String())
				}
			}
		}, nil, 0)

		fmt.Println()
	}

	log.Println("Validating source...")

	asm := make([]*asmCmd, 0)

	// Redefinition detection tables
	var globalTable []*Global
	var functionTable []asmFunc

	// Add default types
	typeMap := map[string]*asmType{
		"word": &asmType{
			name:    "word",
			size:    1,
			builtin: true,
			members: make([]asmTypeMember, 0),
		},
	}

	// Fill tables
	walkInterface(ast, func(val reflect.Value, name string, depth int) {

		if !(val.Kind() == reflect.Struct || (val.Kind() == reflect.Ptr && val.Elem().Kind() == reflect.Struct)) {
			// Early out if value instead of node
			return
		}

		nodeInterface := val.Interface()

		switch node := nodeInterface.(type) {

		case *Global:
			for _, g := range globalTable {
				if g.Name == node.Name {
					panic(fmt.Sprintf("ERROR: Redefinition of global '%s' at %s", node.Name, node.Pos.String()))
				}
			}
			globalTable = append(globalTable, node)

		case *Function:
			functionLabel := getFuncLabel(*node)
			for _, f := range functionTable {
				if f.name == functionLabel {
					panic(fmt.Sprintf("ERROR: Redefinition of function '%s' at %s", node.Name, node.Pos.String()))
				}
			}

			var returnType *asmType
			if node.Type != "void" {
				ret, ok := typeMap[node.Type]
				if !ok {
					panic(fmt.Sprintf("ERROR: Use of undefined type '%s' in function signature (return type of function '%s')", node.Type, node.Name))
				}

				if ret.size != 1 {
					panic(fmt.Sprintf("ERROR: Return types with size != 1 are prohibited (type '%s' in function '%s')", node.Type, node.Name))
				}

				returnType = ret
			}

			f := asmFunc{
				name:       node.Name,
				label:      functionLabel,
				params:     make([]asmTypeMember, 0),
				returnType: returnType,
			}

			for _, p := range node.Parameters {
				asmType, ok := typeMap[p.Type]
				if !ok {
					panic(fmt.Sprintf("ERROR: Use of undefined type '%s' in function parameter '%s' (function '%s')", p.Type, p.Name, node.Name))
				}

				if asmType.size != 1 {
					panic(fmt.Sprintf("ERROR: Parameter types with size != 1 are prohibited (type '%s' in parameter '%s', function '%s')", p.Type, p.Name, node.Name))
				}

				f.params = append(f.params, asmTypeMember{
					name:    p.Name,
					asmType: asmType,
				})
			}

			functionTable = append(functionTable, f)

		// Struct definition
		case *Struct:
			for _, t := range typeMap {
				if t.name == node.Name {
					panic(fmt.Sprintf("ERROR: Redefinition of struct '%s' at %s", node.Name, node.Pos.String()))
				}
			}

			newType := &asmType{
				name:    node.Name,
				size:    0,
				builtin: false,
				members: make([]asmTypeMember, 0),
			}

			if node.Members != nil {
				for _, member := range node.Members {
					memberType, ok := typeMap[member.Type]
					if !ok {
						panic(fmt.Sprintf("ERROR: Use of undefined type '%s' for struct member %s.%s", member.Type, node.Name, member.Name))
					}
					newType.members = append(newType.members, asmTypeMember{
						name:    member.Name,
						asmType: memberType,
					})
					newType.size += memberType.size
				}
			}

			typeMap[node.Name] = newType
		}

	}, nil, 0)

	// Check for entry point existance
	containsMain := false
	for _, f := range functionTable {
		if f.label == "mscr_function_main_params_2" {
			if f.params[0].asmType.name != "word" || f.params[1].asmType.name != "word" || f.returnType == nil || f.returnType.name != "word" {
				panic("ERROR: Function main must have type signature 'func word main (word argc, word argp)'")
			}

			containsMain = true
			break
		}
	}
	if !containsMain {
		panic("ERROR: Entry point not found. Please declare a function 'func word main (word argc, word argp)'")
	}

	transformState := &asmTransformState{
		functionTable: functionTable,
		typeMap:       typeMap,

		currentFunction: "",

		globalMemoryMap: make(map[string]int, 0),
		stringMap:       make(map[string]int, 0),
		maxDataAddr:     3, // Start of global area in .mscr_data block

		variableMap: make(map[string][]asmVar, 0),

		scopeRegisterDirty: make(map[int]bool, AssigneableRegisters),

		specificInitializationAsm: make([]*asmCmd, 0),
		binData:                   make([]int16, 0),

		verbose: verbose,
	}

	// Generate Meta-ASM
	log.Println("Generating Meta-ASM...")

	walkInterface(ast, func(val reflect.Value, name string, depth int) {

		if !(val.Kind() == reflect.Struct || (val.Kind() == reflect.Ptr && val.Elem().Kind() == reflect.Struct)) {
			// Early out if value instead of node
			return
		}

		nodeInterface := val.Interface()
		newAsm := asmForNodePre(nodeInterface, transformState)

		if len(newAsm) == 0 {
			return
		}

		for i := range newAsm {
			newAsm[i].comment = fmt.Sprintf("%s [%s (in func: %s)]", newAsm[i].comment, name, transformState.currentFunction)
		}

		asm = append(asm, newAsm...)

	}, func(val reflect.Value, name string, depth int) {

		if !(val.Kind() == reflect.Struct || (val.Kind() == reflect.Ptr && val.Elem().Kind() == reflect.Struct)) {
			// Early out if value instead of node
			return
		}

		nodeInterface := val.Interface()
		newAsm := asmForNodePost(nodeInterface, transformState)

		if len(newAsm) == 0 {
			return
		}

		for i := range newAsm {
			newAsm[i].comment = fmt.Sprintf("%s [%s (in func: %s)]", newAsm[i].comment, name, transformState.currentFunction)
		}

		// Formatting
		newAsm[len(newAsm)-1].comment += "\n"

		asm = append(asm, newAsm...)

	}, 0)

	// Prepend bootloader init call to userland init if necessary
	if bootloader {
		transformState.specificInitializationAsm = append([]*asmCmd{
			&asmCmd{
				ins: "CALL .mscr_init_bootloader",
			},
		}, transformState.specificInitializationAsm...)
	}

	// Prepare specific init asm
	transformState.specificInitializationAsm = append([]*asmCmd{
		&asmCmd{
			ins: ".mscr_init_userland __LABEL_SET",
		},
	}, append(transformState.specificInitializationAsm, &asmCmd{
		ins:     "RET",
		comment: "Userland init end\n",
	})...)
	asm = append(transformState.specificInitializationAsm, asm...)

	// Insert __CLEARSCOPE to beginning of asm to initialize scoping correctly
	asm = append([]*asmCmd{
		&asmCmd{
			ins: "__CLEARSCOPE",
		},
	}, asm...)

	// Fix global and string references
	// Necessary, because identifiers are by default auto-assigned to var param types
	for _, a := range asm {
		a.fixGlobalAndStringParamTypes(transformState)

		// For debug printing
		a.originalAsmCmdString = a.String()
	}

	// Generate ASM
	log.Println("Resolving Meta-ASM...")

	// Resolve meta-asm
	initAsm := make([]*asmCmd, 0)
	asm = resolveMetaAsm(asm, initAsm, transformState)

	// Append initAsm generated by resolving
	asm = append(initAsm, asm...)

	if !isResolved(asm) {
		panic("ERROR: Meta-ASM has not been fully resolved. This is a compiler bug, sorry.")
	}

	// Optimize generated asm
	if optimizeDisable {
		log.Println("Optimization disabled.")
	} else {
		asm = optimizeAsmAll(asm)
	}

	// DEBUG
	if verbose {
		log.Println("DEBUG OUTPUT (ASM):")
		var prevOrigAsmCmd string
		for _, a := range asm {
			if prevOrigAsmCmd != a.originalAsmCmdString && a.originalAsmCmdString != "" {
				toPrint := strings.TrimSpace(strings.Replace(a.originalAsmCmdString, "\n", "", -1))
				fmt.Println("\nmeta " + toPrint)
				prevOrigAsmCmd = a.originalAsmCmdString
			}

			fmt.Println("out  " + strings.TrimSpace(a.String()))
		}
	}

	// Print asm to string and check for warnings in compiled code
	log.Println("Generating output ASM...")
	outputAsm := ""

	regexpLabelSetOrMetaCmd := regexp.MustCompile(`^(?:\..+\s+)?__.*$`)

	prevIns := &asmCmd{
		ins: "__INTENTIONALLY_INVALID",
	}
	for i, a := range asm {
		outputAsm += a.asmString() + "\n"

		// Check for no return
		if a.ins == "FAULT" && a.params[0].value == FAULT_NO_RETURN {
			lastActualCmd := prevIns
			prevInsIndex := i - 1

			for regexpLabelSetOrMetaCmd.MatchString(lastActualCmd.ins) {
				prevInsIndex--
				if i < 0 {
					panic("ERROR: No valid output asm before FAULT_NO_RETURN, this program would not be executable")
				}

				lastActualCmd = asm[prevInsIndex]
			}

			if lastActualCmd.ins != "RET" {
				fmt.Printf("WARNING: Non-void function without trailing (default) return (%s @ %s)\n", strings.TrimRight(prevIns.asmString(), "\n"), prevIns.scope)
			}
		}

		prevIns = a
	}

	bootloaderInitialization := ""
	if bootloader {
		bootloaderInitialization = bootloaderInitAsm
	}

	// Create data section
	dataAsm := ".mscr_data __LABEL_SET\n"
	for _, d := range transformState.binData {
		dataAsm += fmt.Sprintf("0x%x\n", uint(d))
	}

	// Combine everything together
	return "; Generated using MSCR compiler version " + constants.MCPCVersion + "\n\nJMP .mscr_init_main\n\n" +
		dataAsm +
		initializationAsm +
		bootloaderInitialization +
		outputAsm +
		".mscr_code_end HALT" // Trailer (0x0, but includes label for Assembler)
}

func resolveMetaAsm(asm []*asmCmd, initAsm []*asmCmd, transformState *asmTransformState) []*asmCmd {
	var prevCmd *asmCmd
	prevCmdCounter := 0
	for i := 0; i < len(asm); i++ {
		if asm[i] == prevCmd {
			if isResolved([]*asmCmd{asm[i]}) {
				prevCmdCounter = 0
				continue
			}

			if prevCmdCounter > 100 {
				panic("ERROR: Recursive resolving detected (> 100 steps). This is a compiler bug, sorry. Instruction: " + prevCmd.String())
			}
		} else {
			prevCmdCounter = 0
		}

		prevCmd = asm[i]
		prevCmdCounter++

		resolved := asm[i].resolve(initAsm, transformState)

		for ir := range resolved {
			resolved[ir].originalAsmCmdString = asm[i].originalAsmCmdString
		}

		if len(resolved) == 0 {
			// Cut out value if nothing has been returned
			asm = append(asm[0:i], asm[(i+1):len(asm)]...)
			i--
			continue
		} else if len(resolved) == 1 {
			// Replace value if exactly one item has been returned
			asm[i] = resolved[0]
			i--
		} else {
			// Replace value with returned slice
			asm = append(asm[0:i], append(resolved, asm[(i+1):len(asm)]...)...)
			i--
		}
	}

	return asm
}

func walkInterface(x interface{}, pre func(reflect.Value, string, int), post func(reflect.Value, string, int), level int) {
	typ := reflect.TypeOf(x)

	for typ.Kind() == reflect.Ptr {
		x = reflect.ValueOf(x).Elem().Interface()
		typ = reflect.TypeOf(x)
	}

	if typ.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < typ.NumField(); i++ {
		switch typ.Field(i).Type.Kind() {
		case reflect.Slice:
			s := reflect.ValueOf(x).Field(i)
			styp := reflect.TypeOf(x).Field(i)
			if s.Type().Kind() == reflect.Ptr && s.IsNil() {
				continue
			}

			for j := 0; j < s.Len(); j++ {
				s2 := s.Index(j)

				for s2.Kind() == reflect.Ptr {
					s2 = s2.Elem()
				}

				if pre != nil {
					pre(tryAddr(s2), styp.Name, level)
				}
				walkInterface(s2.Interface(), pre, post, level+1)
				if post != nil {
					post(tryAddr(s2), styp.Name, level)
				}
			}

		default:
			s := reflect.ValueOf(x).Field(i)
			styp := reflect.TypeOf(x).Field(i)
			if s.Type().Kind() == reflect.Ptr && s.IsNil() {
				continue
			}

			for s.Kind() == reflect.Ptr {
				s = s.Elem()
			}

			if pre != nil {
				pre(tryAddr(s), styp.Name, level)
			}

			// Check exported status
			fletter := []rune(styp.Name)[0]
			if unicode.IsLetter(fletter) && unicode.IsUpper(fletter) {
				walkInterface(s.Interface(), pre, post, level+1)
			}

			if post != nil {
				post(tryAddr(s), styp.Name, level)
			}
		}
	}
}

func tryAddr(val reflect.Value) reflect.Value {
	if val.CanAddr() {
		return val.Addr()
	}

	return val
}

const initializationAsm = `
; MSCR initialization routine
.mscr_init_main __LABEL_SET
SET SP ; Stack
.mscr_data_end __LABEL_SET ; _data_end label has to be one word after code start, because reading in bootloaderInitAsm is technically off by one for performance reasons
0x7FFF
SET H ; VarHeap
.mscr_code_end

CALL .mscr_init_userland ; Call program specific initialization

PUSH 0 ; argp
PUSH 0 ; argc
CALL .mscr_function_main_params_2 ; Call userland main

; After main, copy exit code to H to show on hex-display (but keep in A for autotest!)
MOV A H

HALT ; After execution, halt

`

const bootloaderInitAsm = `
; MSCR bootloader static value loader
.mscr_init_bootloader SET A
.mscr_data_end ; Data block end address = Code block start address
SETREG B 0x0003 ; Data start
SETREG C 0xD003 ; Start of readonly CFG region for bootloader ROM + offset for data start

.mscr_init_bootloader_loop_start __LABEL_SET
LOAD D C ; Read from ROM to regD
STOR D B ; Write to RAM
INC C ; Increment read address
INC B ; Increment write address
EQ A D B ; Check if we reached end of data and jump accordingly
JMPNZ .mscr_init_bootloader_return D
JMP .mscr_init_bootloader_loop_start

.mscr_init_bootloader_return RET ; Return out


`
