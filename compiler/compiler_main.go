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

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
)

const CompilerVersion = "0.1.3"

const LexerRegex = `(?s)(\s+)|` +
	`(?P<Int>(?:0x)?\d+)|` +
	`(?P<String>"(?:[^"\\]|\\.)*")|` +
	`(?P<Eval>\[.*?\])|` +
	`(?P<ASM>_asm\s*\{.*?\})|` +
	`(?P<Ident>[a-zA-Z_][a-zA-Z0-9_]*)|` +
	`(?P<AssignmentOperator>\+\=|\-\=|\*\=|\/\=|\%\=|\=)|` +
	`(?P<Operator>\=\=|\!\=|\<\=|\>\=|\<\<|\>\>|\+|\-|\<|\>|\*|\/|\%)|` +
	`(?P<RawToken>\S)`

func Preprocess(inputFile, outputFile string) {
	/*_, err := copy(inputFile, outputFile)
	if err != nil {
		log.Fatalln(err)
	}*/

	cmd := "gpp"

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
		log.Fatalln(err.Error())
	}
}

func GenerateAST(inputFile string) *AST {

	log.Println("Parsing into AST...")

	ast := &AST{}
	lexer := lexer.Must(lexer.Regexp(LexerRegex))
	parser := participle.MustBuild(
		ast,
		participle.Lexer(lexer),
		participle.Unquote(lexer, "String"),
		participle.UseLookahead())
	fileContentsRaw, err := ioutil.ReadFile(inputFile)

	if err != nil {
		log.Fatalln(err.Error())
	}

	// Strip comments
	fileContents := stripComments(string(fileContentsRaw))

	// Handle character type
	fileContents = handleCharacters(fileContents)

	//fmt.Println(fileContents)

	err = parser.ParseString(fileContents, ast)

	if err != nil {
		log.Fatalln(err.Error())
	}

	return ast
}

func stripComments(input string) string {
	// Adapted from: https://stackoverflow.com/a/241506
	regex := `(?s)(?m)//.*?$|/\*.*?\*/|\'(?:\\.|[^\\\'])*\'|"(?:\\.|[^\\"])*"`
	replacer := regexp.MustCompile(regex)
	return replacer.ReplaceAllStringFunc(input, func(s string) string {
		if strings.IndexRune(s, '"') == 0 || strings.IndexRune(s, '\'') == 0 {
			return s
		} else {
			return " "
		}
	})
}

func handleCharacters(input string) string {
	regex := `\'.\'|\".*?\"`
	replacer := regexp.MustCompile(regex)
	return replacer.ReplaceAllStringFunc(input, func(s string) string {
		if len(s) != 3 || strings.IndexRune(s, '"') == 0 {
			return s
		} else {
			return strconv.Itoa(int(s[1]))
		}
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
