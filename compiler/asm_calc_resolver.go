package compiler

import (
	"bytes"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/PiMaker/mscr/yard"
)

const CalcTypeRegexLiteral = `^(?:0x)?\d+$`
const CalcTypeRegexMath = `^(?:\=\=|\!\=|\<\=|\>\=|\<\<|\>\>|\+|\-|\<|\>|\*|\/|\%|\(|\)|\s|[a-zA-Z0-9_])*$`

var calcTypeRegexLiteralRegexp = regexp.MustCompile(CalcTypeRegexLiteral)
var calcTypeRegexMathRegexp = regexp.MustCompile(CalcTypeRegexMath)

func resolveCalc(calc string, scope string, state *asmTransformState) []*asmCmd {
	// Remove square brackets, they are just indicators that this is a calc value string
	calc = strings.Replace(calc, "[", "", -1)
	calc = strings.Replace(calc, "]", "", -1)
	calc = strings.Trim(calc, " \t")

	// Match type of expression using regex
	if calcTypeRegexLiteralRegexp.MatchString(calc) {

		return setRegToLiteralFromString(calc, "F") // F is calc out register

	} else if calcTypeRegexMathRegexp.MatchString(calc) {

		// Math/Function parsing
		ensureShuntingYardParser()
		shunted := callShuntingYardParser(calc)
		output := make([]*asmCmd, 0)

		// Function call temp vars
		var funcFunct string
		var funcFunarg int

		for _, token := range shunted {
			switch token.tokenType {
			case "FUNCT":
				funcFunct = token.value
			case "FUNARG":
				funcFunarg, _ = strconv.Atoi(token.value)

			case "SYS":
				switch token.value {
				case "INVOKE":
					// Call function and push return value to stack
					log.Println("Would call function: " + funcFunct + " with param count: " + strconv.Itoa(funcFunarg))
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
							&asmParam{
								asmParamType: asmParamTypeRaw,
								value:        "F",
							},
						},
						comment: " CALC: var " + token.value,
						scope:   scope,
					}

					// Take care of globals and string addresses
					cmd.fixGlobalAndStringParamTypes(state)

					output = append(output, cmd)
				}

				// Then, push F onto stack
				output = append(output, &asmCmd{
					ins: "PUSH",
					params: []*asmParam{
						&asmParam{
							asmParamType: asmParamTypeRaw,
							value:        "F",
						},
					},
					comment: " CALC: push operand",
					scope:   scope,
				})

			case "OPER":
				switch token.value {
				case "+", "*", "-", "&", "|", "^":
					// Pop twice then calculate then push again
					output = append(output, &asmCmd{
						ins: "POP",
						params: []*asmParam{
							&asmParam{
								asmParamType: asmParamTypeRaw,
								value:        "E",
							},
						},
					})
					output = append(output, &asmCmd{
						ins: "POP",
						params: []*asmParam{
							&asmParam{
								asmParamType: asmParamTypeRaw,
								value:        "F",
							},
						},
					})

					output = append(output, &asmCmd{
						ins: symbolToALUFuncName(token.value),
						params: []*asmParam{
							&asmParam{
								asmParamType: asmParamTypeRaw,
								value:        "F", // Input 1
							},
							&asmParam{
								asmParamType: asmParamTypeRaw,
								value:        "F", // Output
							},
							&asmParam{
								asmParamType: asmParamTypeRaw,
								value:        "E", // Input 2
							},
						},
					})

					output = append(output, &asmCmd{
						ins: "PUSH",
						params: []*asmParam{
							&asmParam{
								asmParamType: asmParamTypeRaw,
								value:        "F",
							},
						},
					})

				default:
					log.Fatalln("ERROR: Unsupported tokenType returned from shunting yard parser. This calc feature is probably not implemented yet. (" + token.tokenType + " = " + token.value + ")")
				}
			}
		}

		output = append(output, &asmCmd{
			ins: "POP",
			params: []*asmParam{
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        "F",
				},
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

		if stackValue != 0 {
			spew.Dump(shunted)
			log.Fatalln("ERROR: Calc instruction produced invalid stack. This is *probably* a compiler bug. (Stack value: " + strconv.Itoa(stackValue) + ")")
		}

		// Shortcut: If last two instructions are "PUSH F", "POP F", leaving them out will still put result in F
		if len(output) > 1 && output[len(output)-2].ins == "PUSH" && len(output[len(output)-2].params) == 1 &&
			output[len(output)-2].params[0].asmParamType == asmParamTypeRaw && output[len(output)-2].params[0].value == "F" {

			return output[0 : len(output)-2]
		}

		return output

	} else {
		log.Fatalln("ERROR: Unsupported calc string: " + calc)
		return nil
	}
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
	default:
		log.Fatalln("ERROR: Unsupported operator in calc instruction: " + oper)
		return ""
	}
}

func setRegToLiteralFromString(calc, reg string) []*asmCmd {
	var calcValue int64
	if strings.Index(calc, "0x") == 0 {
		// Error ignored, format is validated at this point
		calcValue, _ = strconv.ParseInt(calc[2:], 16, 16)
	} else {
		calcValue, _ = strconv.ParseInt(calc, 10, 16)
	}

	return []*asmCmd{
		&asmCmd{
			ins: "SETREG",
			params: []*asmParam{
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        reg,
				},
				&asmParam{
					asmParamType: asmParamTypeRaw,
					value:        "0x" + strconv.FormatInt(calcValue, 16),
				},
			},
			comment: " CALC: literal " + calc,
		},
	}
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
		log.Fatalln("ERROR: Could not extract dijkstra parser: " + err.Error())
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

	if err != nil {
		log.Println("DEBUG LOG OF SHUNTING YARD PARSER ATTACHED:")
		fmt.Println(string(out.Bytes()))

		log.Fatalln("ERROR: While executing shunting yard parser: " + err.Error())
	}

	return parseIntoYardTokens(string(out.Bytes()))
}
