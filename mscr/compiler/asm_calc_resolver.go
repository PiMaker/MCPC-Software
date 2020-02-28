package compiler

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/davecgh/go-spew/spew"

	"github.com/PiMaker/MCPC-Software/mscr/yard"
)

// Careful here, we want to match base 10, 16, but not variables
// (e.g. 0xfAb = match, 0xno_u = no match, technically a variable [though it has a leading 0?])
const CalcTypeRegexLiteral = `^((0(x|X))[0-9a-fA-F]+|\d+)$`
const CalcTypeRegexMath = `^(?:\=\=|\!\=|\<\=|\>\=|\<\<|\>\>|\+|\-|\<|\>|\*|\/|\%|\(|\)|\s|,|\~|\||\&|\^|[a-zA-Z0-9_$\.])*$`
const CalcTypeRegexAsm = `^asm\s*\{.*?\}$`

var calcTypeRegexLiteralRegexp = regexp.MustCompile(CalcTypeRegexLiteral)
var calcTypeRegexMathRegexp = regexp.MustCompile(CalcTypeRegexMath)
var calcTypeRegexAsmRegexp = regexp.MustCompile(CalcTypeRegexAsm)

// Wrapper around resolveCalcInternal that additionally prints debug information if --verbose was passed
// (and performs some additional post-processing on generated asm)
func resolveCalc(calc string, scope string, state *asmTransformState) []*asmCmd {
	if state.verbose {
		log.Println("DEBUG OUTPUT: Calc expression \"" + calc + "\" resulted in following meta-asm:")
	}

	output := resolveCalcInternal(calc, scope, state)

	// Set scope of "parent" (calc instruction) on all generated "child" instructions
	for _, a := range output {
		a.scope = scope

		if state.verbose {
			fmt.Println("meta/calc " + a.String())
		}
	}

	if state.verbose {
		fmt.Println()
	}

	return output
}

func resolveCalcInternal(calc string, scope string, state *asmTransformState) []*asmCmd {
	// Remove square brackets, they are just indicators that this is a calc value string
	calc = strings.Replace(calc, "[", "", -1)
	calc = strings.Replace(calc, "]", "", -1)
	calc = strings.Trim(calc, " \t")

	// Match type of expression using regex
	if calcTypeRegexAsmRegexp.MatchString(calc) {

		// Assume developer knows what they are doing
		// Put asm verbosely and hope if fills up F
		return toRawAsm("_" + calc) // Note: underscore (_) is not used in calc (e.g. "asm", not "_asm"), to not confuse the parser

	} else if calcTypeRegexLiteralRegexp.MatchString(calc) {

		return setRegToLiteralFromString(calc, "F") // F is calc out register

	} else if calcTypeRegexMathRegexp.MatchString(calc) {

		// Math/Function parsing
		ensureShuntingYardParser()
		shunted := callShuntingYardParser(calc)
		output := make([]*asmCmd, 0)

		// Function call temp vars
		var funcFunct string
		funcStackOffset := 0
		var funcFunargLast int
		var lastVar string

		for i, token := range shunted {
			switch token.tokenType {
			case "FUNCT":
				funcFunct = token.value
			case "FUNARG":
				funcFunarg, _ := strconv.Atoi(token.value)
				// Offset pop count because called function will pop values from stack for us
				funcStackOffset -= funcFunarg
				funcFunargLast = funcFunarg

			case "SYS":
				switch token.value {
				case "INVOKE":
					// Check for $$ invocation mistakes
					if funcFunct == "$$" && (i < 3 || shunted[i-3].tokenType != "OPRND" || calcTypeRegexLiteralRegexp.MatchString(shunted[i-3].value)) {
						panic("ERROR: Tried calling special function $$ on anything else than a variable name (Note: $$ does not support nesting or addressing literals)")
					}

					// Call function and push return value to stack
					output = append(output, callCalcFunc(funcFunct, funcFunargLast, state, lastVar)...)

					// Special functions include a "POP", fix the stack counter for them by increasing the internal counter for what it was decreased earlier
					if funcFunct == "$" || funcFunct == "$$" {
						funcStackOffset += funcFunargLast // Will always be 1, since $ and $$ both require exactly one argument (or they fatalln)
					}
				}

			case "OPRND":
				// First, put operand in F
				if calcTypeRegexLiteralRegexp.MatchString(token.value) {
					output = append(output, setRegToLiteralFromString(token.value, "F")...)
				} else {
					// Assume variable or global
					cmd := &asmCmd{
						ins: "MOV",
						params: []*asmParam{
							&asmParam{
								asmParamType: asmParamTypeVarRead,
								value:        token.value,
							},
							rawAsmParam("F"),
						},
						comment: " CALC: var " + token.value,
					}

					lastVar = token.value

					// Take care of globals and string addresses
					cmd.fixGlobalAndStringParamTypes(state)

					output = append(output, cmd)
				}

				// Then, push F onto stack
				output = append(output, &asmCmd{
					ins: "PUSH",
					params: []*asmParam{
						rawAsmParam("F"),
					},
					comment: " CALC: push operand",
				})

			case "OPER":
				switch token.value {
				case "+", "*", "-", "&", "|", "^", "==", "<", ">", "<=", ">=", "!=", ">>", "<<":
					// Pop twice then calculate then push again
					output = append(output, &asmCmd{
						ins: "POP",
						params: []*asmParam{
							rawAsmParam("E"),
						},
					})
					output = append(output, &asmCmd{
						ins: "POP",
						params: []*asmParam{
							rawAsmParam("F"),
						},
					})

					aluIns := symbolToALUFuncName(token.value)
					output = append(output, &asmCmd{
						ins: aluIns,
						params: []*asmParam{
							rawAsmParam("F"), // Input 1
							rawAsmParam("F"), // Output
							rawAsmParam("E"), // Input 2
						},
						comment: " CALC: operator " + aluIns,
					})

					output = append(output, &asmCmd{
						ins: "PUSH",
						params: []*asmParam{
							rawAsmParam("F"),
						},
					})

				case ".-", ".~", "~":
					output = append(output, &asmCmd{
						ins: "POP",
						params: []*asmParam{
							rawAsmParam("F"),
						},
					})

					aluIns := "COM"
					if token.value == ".-" {
						aluIns = "NEG"
					}

					output = append(output, &asmCmd{
						ins: aluIns,
						params: []*asmParam{
							rawAsmParam("F"),
							rawAsmParam("F"),
						},
					})

					output = append(output, &asmCmd{
						ins: "PUSH",
						params: []*asmParam{
							rawAsmParam("F"),
						},
					})

				default:
					panic("ERROR: Unsupported tokenType returned from shunting yard parser. This calc feature is probably not implemented yet. (" + token.tokenType + " = " + token.value + ")")
				}
			}
		}

		output = append(output, &asmCmd{
			ins: "POP",
			params: []*asmParam{
				rawAsmParam("F"),
			},
		})

		// Validate result to preserve stack correctness
		stackValue := 0
		for _, c := range output {
			if c.ins == "PUSH" {
				stackValue++
			} else if c.ins == "POP" {
				stackValue--
			}
		}

		// Function calls modify the stack without push-es or pop-s
		stackValue += funcStackOffset

		if stackValue != 0 {
			log.Println("ERROR: In calc resolving, RPN attached hereafter:")
			spew.Dump(shunted)
			panic("ERROR: Calc-resoved instructions would produce invalid stack. This is either a compiler bug or an invalid calc-string (e.g. invalid operators or function calls). (Stack value: " + strconv.Itoa(stackValue) + "; should be 0)")
		}

		// Set scope of "parent" (calc instruction) on all generated "child" instructions
		for _, a := range output {
			a.scope = scope
		}

		// Shortcut: If last two instructions are "PUSH F", "POP F", leaving them out will still put result in F
		if len(output) > 1 && output[len(output)-2].ins == "PUSH" && len(output[len(output)-2].params) == 1 &&
			output[len(output)-2].params[0].asmParamType == asmParamTypeRaw && output[len(output)-2].params[0].value == "F" {

			return output[0 : len(output)-2]
		}

		return output
	}

	panic("ERROR: Unsupported calc string: " + calc)
}

func symbolToALUFuncName(oper string) string {
	switch oper {
	case "*":
		return "MUL"
	case "+":
		return "ADD"
	case "-":
		return "SUB"
	case "^":
		return "XOR"
	case "&":
		return "AND"
	case "|":
		return "OR"
	case "==":
		return "EQ"
	case "!=":
		return "NEQ"
	case ">":
		return "GT"
	case "<":
		return "LT"
	case "<=":
		return "LTOE"
	case ">=":
		return "GTOE"
	case "<<":
		return "SHFL"
	case ">>":
		return "SHFR"
	default:
		panic("ERROR: Unsupported operator in calc instruction: " + oper)
	}
}

func setRegToLiteralFromString(calc, reg string) []*asmCmd {
	var calcValue uint64
	if strings.Index(calc, "0x") == 0 || strings.Index(calc, "0X") == 0 {
		// Error ignored, format is validated at this point
		calcValue, _ = strconv.ParseUint(calc[2:], 16, 16)
	} else {
		calcValue, _ = strconv.ParseUint(calc, 10, 16)
	}

	// Shortcuts for 1, 0, -1 registers
	if calcValue == 1 {
		return []*asmCmd{
			&asmCmd{
				ins: "MOV",
				params: []*asmParam{
					rawAsmParam("1"),
					rawAsmParam(reg),
				},
				comment: " CALC: literal " + calc + " (from const reg)",
			},
		}
	} else if calcValue == 0xFFFF {
		return []*asmCmd{
			&asmCmd{
				ins: "MOV",
				params: []*asmParam{
					rawAsmParam("-1"),
					rawAsmParam(reg),
				},
				comment: " CALC: literal " + calc + " (from const reg)",
			},
		}
	} else if calcValue == 0 {
		return []*asmCmd{
			&asmCmd{
				ins: "MOV",
				params: []*asmParam{
					rawAsmParam("0"),
					rawAsmParam(reg),
				},
				comment: " CALC: literal " + calc + " (from const reg)",
			},
		}
	}

	return []*asmCmd{
		&asmCmd{
			ins: "SETREG",
			params: []*asmParam{
				rawAsmParam(reg),
				rawAsmParam("0x" + strconv.FormatUint(calcValue, 16)),
			},
			comment: " CALC: literal " + calc,
		},
	}
}

func callCalcFunc(funcName string, paramCount int, state *asmTransformState, lastVarName string) []*asmCmd {
	retval := make([]*asmCmd, 0)

	if funcName == "$" {

		if paramCount != 1 {
			panic("ERROR: Special function $ requires exactly 1 argument, " + strconv.Itoa(paramCount) + " given")
		}

		// Special function $ -> Dereference (get value behind address)

		// Retrieve address value
		retval = append(retval, &asmCmd{
			ins: "POP",
			params: []*asmParam{
				rawAsmParam("F"),
			},
		})

		// Perform memory access
		retval = append(retval, &asmCmd{
			ins: "LOAD",
			params: []*asmParam{
				rawAsmParam("F"),
				rawAsmParam("F"),
			},
		})

		// Push result back
		retval = append(retval, &asmCmd{
			ins: "PUSH",
			params: []*asmParam{
				rawAsmParam("F"),
			},
		})

		retval[1].fixGlobalAndStringParamTypes(state)

	} else if funcName == "$$" {

		if paramCount != 1 {
			panic("ERROR: Special function $$ requires exactly 1 argument, " + strconv.Itoa(paramCount) + " given")
		}

		// Special function $$ -> Reference (create pointer)

		// This POP is technically useless, but needed to keep the stack sane. It will automatically be optimized out later.
		// (Sadly, the variable access getting us this value probably won't, but hey, I'm trying.)
		retval = append(retval, &asmCmd{
			ins: "POP",
			params: []*asmParam{
				rawAsmParam("F"),
			},
		})

		// Mark value as directly-addressed since we never know when someone is going to dereference this pointer
		// Note: This only does something for variables, globals are always directly-addressed
		retval = append(retval, &asmCmd{
			ins:                 "__SET_DIRECT",
			scopeAnnotationName: lastVarName,
		})

		retval = append(retval, &asmCmd{
			ins: "MOV",
			params: []*asmParam{
				&asmParam{
					asmParamType: asmParamTypeVarAddr,
					value:        lastVarName,
				},
				rawAsmParam("F"),
			},
		})

		// Push result back
		retval = append(retval, &asmCmd{
			ins: "PUSH",
			params: []*asmParam{
				rawAsmParam("F"),
			},
		})

		retval[2].fixGlobalAndStringParamTypes(state)

	} else {

		// Regular function

		// Scope handling should still work in calc context, recursive resolving is really quite something huh?
		retval = append(retval, &asmCmd{
			ins: "__FLUSHSCOPE",
		})

		retval = append(retval, &asmCmd{
			ins: "__CLEARSCOPE",
		})

		fLabel := getFuncLabelSpecific(funcName, paramCount)
		function := ""
		for _, f := range state.functionTable {
			if f.label == fLabel {
				function = f.label

				if f.returnType == nil {
					panic(fmt.Sprintf("ERROR: Tried calling a void function in a calc context: Function '%s' with %d parameters\n", funcName, paramCount))
				}

				break
			}
		}

		if function == "" {
			log.Printf("WARNING: Cannot find function to call (calc): Function '%s' with %d parameters (Assuming extern function)\n", funcName, paramCount)
			function = fLabel
		}

		retval = append(retval, &asmCmd{
			ins: "CALL",
			params: []*asmParam{
				rawAsmParam("." + function),
			},
		})

		// Push returned value to stack for further calculations
		retval = append(retval, &asmCmd{
			ins: "PUSH",
			params: []*asmParam{
				rawAsmParam("A"),
			},
		})

		retval = append(retval, &asmCmd{
			ins: "__CLEARSCOPE",
		})

	}

	return retval
}

var parserExtracted = false
var dijkstraPath = ""

func ensureShuntingYardParser() {
	if parserExtracted && dijkstraPath != "" {
		return
	}

	// Extract parser from go-bindata
	dijkstraPath = os.TempDir() + string(os.PathSeparator)
	err := yard.RestoreAssets(dijkstraPath, "dijkstra-shunting-yard")

	if err != nil {
		panic("ERROR: Could not extract dijkstra parser: " + err.Error())
	}

	dijkstraPath += "dijkstra-shunting-yard" + string(os.PathSeparator)
	parserExtracted = true
}

func callShuntingYardParser(calc string) []*YardToken {
	cmd := exec.Command(dijkstraPath + "shunt.sh")

	var out bytes.Buffer
	in := bytes.NewBufferString(calc + " ") // Space is needed, trust me

	cmd.Stdin = in
	cmd.Stdout = &out

	err := cmd.Run()

	output := string(out.Bytes())

	if err != nil {
		log.Println("Debug log of shunt.sh:")
		fmt.Println(output)

		panic("ERROR: While executing shunting yard parser: " + err.Error())
	}

	// Check for error output
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, shuntSplit+"error") {
			log.Println("ERROR: Shunting yard parser encountered an error on a calc expression:")
			log.Println("Calc: " + calc)
			panic("Error: " + line)
		}
	}

	return parseIntoYardTokens(string(out.Bytes()))
}
