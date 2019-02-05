package compiler

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	gppbin "github.com/PiMaker/MCPC-Software/mscr/gppbin"

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
)

const LexerRegex = `(?s)(\s+)|` +
	`(?P<Int>(?:(?:0(x|X))[0-9a-fA-F]+|\d+))|` +
	`(?P<String>"(?:[^"\\]|\\.)*")|` +
	`(?P<Eval>\[.*?\])|` +
	`(?P<ASM>_asm\s*\{.*?\})|` +
	`(?P<Ident>[a-zA-Z_$][a-zA-Z0-9_$]*)|` +
	`(?P<AssignmentOperator>\+\=|\-\=|\*\=|\/\=|\%\=|\=)|` +
	`(?P<Operator>\=\=|\!\=|\<\=|\>\=|\<\<|\>\>|\+|\-|\<|\>|\*|\/|\%)|` +
	`(?P<RawToken>\S)`

var regexpAutotestHeader = regexp.MustCompile(`(?m)^;autotest\s+(.*?)$`)

func Preprocess(inputFile, outputFile string) {
	cmd := os.TempDir() + string(os.PathSeparator) + "gpp"

	if _, err := exec.LookPath("gpp"); err == nil {
		log.Println("gpp found in path")
		cmd = "gpp"
	} else {
		log.Println("gpp not in path, using bundled version")
		err = gppbin.RestoreAsset(os.TempDir()+string(os.PathSeparator), "gpp")
		if err != nil {
			panic("ERROR: Could not extract bundled gpp: " + err.Error())
		}
	}

	// Mostly for debugging at this point
	if _, err := os.Stat("./gpp"); err == nil {
		cmd = "./gpp"
	}
	if _, err := os.Stat(".\\gpp.exe"); err == nil {
		cmd = ".\\gpp.exe"
	}

	log.Printf("Executing GPP: %s -o %s -C %s\n", cmd, outputFile, inputFile)
	stdout, err := exec.Command(cmd, "-o", outputFile, "-C", inputFile).CombinedOutput()

	fmt.Print(string(stdout))

	if err != nil {
		panic(err.Error())
	}
}

func GenerateAST(inputFile string) *AST {

	log.Println("Parsing into AST...")

	ast := &AST{}
	lexer := lexer.Must(lexer.Regexp(LexerRegex))
	parser := participle.MustBuild(
		ast,
		participle.Lexer(lexer),
		participle.Unquote("String"),
		participle.UseLookahead(5))
	fileContentsRaw, err := ioutil.ReadFile(inputFile)

	if err != nil {
		panic(err.Error())
	}

	astCommentHeader := make([]string, 0)

	// Check for autotest header
	autotestMatch := regexpAutotestHeader.FindAllStringSubmatch(string(fileContentsRaw), 1)
	if autotestMatch != nil && len(autotestMatch) > 0 && len(autotestMatch[0]) > 1 {
		astCommentHeader = []string{";autotest " + autotestMatch[0][1]}
		log.Println("Autotest header found: " + astCommentHeader[0])

		fileContentsRaw = fileContentsRaw[strings.Index(string(fileContentsRaw), "\n"):]
	}

	// Strip comments
	fileContents := stripComments(string(fileContentsRaw))

	// Handle 'character' type as numbers directly
	fileContents = handleCharacters(fileContents)

	// Automatically enclose possible calc expressions in square brackets
	// "Bracketless M"
	fileContents = autoCalcBracket(fileContents)

	//fmt.Println(fileContents)

	err = parser.ParseString(fileContents, ast)

	if err != nil {
		panic(err.Error())
	}

	if ast == nil || ast.TopExpressions == nil || len(ast.TopExpressions) == 0 {
		panic("Empty AST parsed. Check your syntax!")
	}

	ast.CommentHeaders = astCommentHeader

	return ast
}

// This is actually a big clusterfuck, but it *seems* to be working well enough for now
// TODO: Yeet this function into oblivion
func autoCalcBracket(input string) string {
	// Note: Function call parameters are converted to a single big calc, including the comma between multiple parameters (if there are any)
	regex := `(?s)return\s+([^;]*?);|(?:\+\=|\-\=|\*\=|\/\=|\%\=|\=)\s*([^;]+);|if\s+([^{]*){|while\s+([^{]*){|(?:[a-zA-Z_$][a-zA-Z0-9_$]*)\s*\((.*?)\)\s*;|func\s+(?:var|void)\s+(?:[a-zA-Z_$][a-zA-Z0-9_$]*)|global.*?;`
	replacer := regexp.MustCompile(regex)
	regexReplaced := replacer.ReplaceAllStringFunc(input, func(s string) string {
		// Ignore patterns starting with '"', "global" or "func" (this is our makeshift replacement for lookbehinds)
		if strings.IndexRune(s, '"') == -1 && strings.Index(s, "global") != 0 && strings.Index(s, "func") != 0 && strings.Index(s, "_reg_assign") != 0 {
			s = strings.Replace(s, "[", "", -1)
			s = strings.Replace(s, "]", "", -1)
			groupText, i := firstNonEmpty(replacer.FindStringSubmatch(s)[1:])
			if i == -1 {
				return s
			}
			// 2*(i+1) because weird golang regexp group index handling idk look at the docs kthxbye
			groupIndex := replacer.FindStringSubmatchIndex(s)[2*(i+1)]
			withBrackets := s[:groupIndex] + "[" + groupText + "]" + s[groupIndex+len(groupText):]

			// Sorry to everyone who is reading this
			// BTW 4 is actually 5, but we get the index from the slice [1:] so it's minus one
			// Told you
			if i == 4 {
				//withBrackets = strings.Replace(withBrackets, ",", "],[", -1)

				// Big chungus loop below replaces string replace up top to handle expressions like:
				// funca(param1_func(param1, param2), param2)
				// correctly as
				// funca([param1_func(param1, param2)],[param2])
				bracketDepth := 0
				for i := 0; i < len(withBrackets); i++ {
					if withBrackets[i] == '(' {
						bracketDepth++
					}

					if withBrackets[i] == ')' {
						bracketDepth--
					}

					if withBrackets[i] == ',' && bracketDepth < 2 {
						withBrackets = withBrackets[:i] + "],[" + withBrackets[i+1:]
						i += 2
					}
				}
			}

			return withBrackets
		}

		return s
	})

	// Fix standalone function calls
	return regexReplaced
}

func firstNonEmpty(arr []string) (string, int) {
	for i, s := range arr {
		if len(s) > 0 {
			return s, i
		}
	}

	return "", -1
}

func stripComments(input string) string {
	// Adapted from: https://stackoverflow.com/a/241506
	regex := `(?s)(?m)//.*?$|/\*.*?\*/|\'(?:\\.|[^\\\'])*\'|"(?:\\.|[^\\"])*"`
	replacer := regexp.MustCompile(regex)
	return replacer.ReplaceAllStringFunc(input, func(s string) string {
		if strings.IndexRune(s, '"') == 0 || strings.IndexRune(s, '\'') == 0 {
			return s
		}

		return " "
	})
}

func handleCharacters(input string) string {
	regex := `\'\\?.\'|\".*?\"`
	replacer := regexp.MustCompile(regex)
	return replacer.ReplaceAllStringFunc(input, func(s string) string {
		if (len(s) != 3 && len(s) != 4) || strings.IndexRune(s, '"') == 0 {
			return s
		}

		if len(s) == 4 {
			unquoted, err := strconv.Unquote(s)
			if err != nil {
				panic("ERROR: Unknown escape sequence in char: " + err.Error())
			}
			return strconv.Itoa(int(unquoted[0]))
		}

		return strconv.Itoa(int(s[1]))
	})
}

// Taken from: https://opensource.com/article/18/6/copying-files-go
func copy(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}
