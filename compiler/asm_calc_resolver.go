package compiler

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/davecgh/go-spew/spew"

	yard "github.com/mgenware/go-shunting-yard"
)

const CalcTypeRegexLiteral = `^(?:0x)?\d+$`
const CalcTypeRegexFunctionCall = `^(?P<Ident>[a-zA-Z_][a-zA-Z0-9_]*)\d*\((?P<Params>(\s*[a-zA-Z0-9_]+\s*,?)*)\)$`
const CalcTypeRegexMath = `^(?:\=\=|\!\=|\<\=|\>\=|\<\<|\>\>|\+|\-|\<|\>|\*|\/|\%|\(|\)|\s|[a-zA-Z0-9_])*$`

var calcTypeRegexLiteralRegexp = regexp.MustCompile(CalcTypeRegexLiteral)
var calcTypeRegexFunctionCallRegexp = regexp.MustCompile(CalcTypeRegexFunctionCall)
var calcTypeRegexMathRegexp = regexp.MustCompile(CalcTypeRegexMath)

func resolveCalc(calc string, scope string) []*asmCmd {
	// Remove square brackets, they are just indicators that this is a calc value string
	calc = strings.Replace(calc, "[", "", -1)
	calc = strings.Replace(calc, "]", "", -1)
	calc = strings.Trim(calc, " \t")

	// Match type of expression using regex
	if calcTypeRegexLiteralRegexp.MatchString(calc) {
		var calcValue int
		if strings.Index(calc, "0x") == 0 {
			// Error ignored, format is validated at this point
			val, _ := strconv.ParseInt(calc[2:], 16, 16)
			calcValue = int(val)
		} else {
			val, _ := strconv.ParseInt(calc, 10, 16)
			calcValue = int(val)
		}

		return []*asmCmd{
			&asmCmd{
				ins: "SETREG",
				params: []*asmParam{
					&asmParam{
						asmParamType: asmParamTypeRaw,
						value:        "F", // F is calc out register
					},
					&asmParam{
						asmParamType: asmParamTypeRaw,
						value:        strconv.Itoa(calcValue),
					},
				},
			},
		}
	} else if calcTypeRegexFunctionCallRegexp.MatchString(calc) {
		fmt.Print(calc)
		fmt.Println(" <- function call (TODO: implement)")
	} else if calcTypeRegexMathRegexp.MatchString(calc) {

		// Math parsing
		scanned, err := yard.Scan(calc)

		if err != nil {
			log.Fatalln("ERROR (in calc/math/lexing): " + err.Error())
		}

		parsed, err := yard.Parse(scanned)

		if err != nil {
			log.Fatalln("ERROR (in calc/math/parsing): " + err.Error())
		}

		fmt.Println()
		fmt.Println("Math tokens:")
		for _, t := range parsed {
			spew.Dump(t)
		}
		fmt.Println()

	} else {
		log.Fatalln("ERROR: Unsupported calc string: " + calc)
	}

	return make([]*asmCmd, 0)
}
