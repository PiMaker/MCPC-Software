package compiler

import (
	"fmt"
	"github.com/logrusorgru/aurora"
	"log"
	"reflect"
	"strings"
	"unicode"
)

func (ast *AST) GenerateASM(bootloader, verbose bool) string {

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
	}

	log.Println("Validating source...")

	asm := make([]*asmCmd, 0)

	// Redefinition detection tables
	var globalTable []*Global
	var functionTableVar []string
	var functionTableVoid []string

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
					log.Fatalf("Redefinition of global '%s' at %s\n", node.Name, node.Pos.String())
				}
			}
			globalTable = append(globalTable, node)

		case *Function:
			functionLabel := getFuncLabel(*node)
			for _, f := range append(functionTableVoid, functionTableVar...) {
				if f == functionLabel {
					log.Fatalf("Redefinition of function '%s' at %s\n", node.Name, node.Pos.String())
				}
			}

			if node.Type == "var" {
				functionTableVar = append(functionTableVar, functionLabel)
			} else if node.Type == "void" {
				functionTableVoid = append(functionTableVoid, functionLabel)
			}
		}

	}, nil, 0)

	// Check for entry point existance
	containsMain := false
	for _, f := range functionTableVar {
		if f == "mscr_function_main_params_2" {
			containsMain = true
			break
		}
	}
	if !containsMain {
		log.Fatalln("ERROR: Entry point not found. Please declare a function 'func var main (argc, argp)'")
	}

	transformState := &asmTransformState{
		functionTableVar:  functionTableVar,
		functionTableVoid: functionTableVoid,

		currentFunction: "",

		globalMemoryMap: make(map[string]int, 0),
		stringMap:       make(map[string]int, 0),
		maxDataAddr:     4, // Start of global area in .mscr_data block

		variableMap: make(map[string][]asmVar, 0),

		scopeRegisterDirty: make(map[int]bool, AssigneableRegisters),

		specificInitializationAsm: make([]*asmCmd, 0),
		binData:                   make([]int16, 0),
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
		log.Fatalln("ERROR: Meta-ASM has not been fully resolved. This is a compiler bug, sorry.")
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
	prevIns := &asmCmd{
		ins: "",
	}
	for _, a := range asm {
		outputAsm += a.asmString() + "\n"

		// Check for no return
		if a.ins == "FAULT" && a.params[0].value == FAULT_NO_RETURN && prevIns.ins != "RET" {
			fmt.Printf("WARNING: Non-void function without trailing (default) return (%s @ %s)\n", strings.TrimRight(prevIns.asmString(), "\n"), prevIns.scope)
		}

		prevIns = a
	}

	bootloaderInitialization := ""
	if bootloader {
		bootloaderInitialization = fmt.Sprintf(bootloaderInitAsm, 4+len(transformState.binData))
	}

	// Create data section
	dataAsm := "0x4000 ; HSP\n\n.mscr_data __LABEL_SET\n"
	for _, d := range transformState.binData {
		dataAsm += fmt.Sprintf("0x%x\n", d)
	}

	// Combine everything together
	return "; Generated using MSCR compiler version " + CompilerVersion + "\n\nJMP .mscr_init_main\n\n" +
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
				log.Fatalln("ERROR: Recursive resolving detected (> 100 steps). This is a compiler bug, sorry. Instruction: " + prevCmd.String())
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
.mscr_init_bootloader SETREG A 0x%x ; Data block end address
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
