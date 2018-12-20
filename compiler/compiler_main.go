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

	gppbin "github.com/PiMaker/mscr/gppbin"

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
)

const CompilerVersion = "0.2.1"

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
	/*_, err := copy(inputFile, outputFile)
	if err != nil {
		log.Fatalln(err)
	}*/

	cmd := os.TempDir() + string(os.PathSeparator) + "gpp"

	if _, err := exec.LookPath("gpp"); err == nil {
		log.Println("gpp found in path")
		cmd = "gpp"
	} else {
		log.Println("gpp not in path, using bundled version")
		err = gppbin.RestoreAsset(os.TempDir()+string(os.PathSeparator), "gpp")
		if err != nil {
			log.Fatalln("ERROR: Could not extract bundled gpp: " + err.Error())
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
		participle.Unquote("String"),
		participle.UseLookahead(5))
	fileContentsRaw, err := ioutil.ReadFile(inputFile)

	if err != nil {
		log.Fatalln(err.Error())
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

	err = parser.ParseString(fileContents, ast)

	if err != nil {
		log.Fatalln(err.Error())
	}

	if ast == nil || ast.TopExpressions == nil || len(ast.TopExpressions) == 0 {
		log.Fatalln("Empty AST parsed. Check your syntax!")
	}

	ast.CommentHeaders = astCommentHeader

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
