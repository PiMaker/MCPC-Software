package compiler

import (
	"fmt"
	"log"
	"reflect"
	"regexp"

	"github.com/alecthomas/participle/lexer"
)

var regexpAsmExtract = regexp.MustCompile(`(?s)asm\s*\{(.*?)\}`)

type asmTransformState struct {
	currentFunction string
	functionTable   []string

	globalMemoryMap map[string]int
	maxGlobalAddr   int
}

func asmForNodePre(nodeInterface interface{}, state *asmTransformState) string {
	newAsm := ""

	switch astNode := nodeInterface.(type) {

	case Function:
		functionLabel := fmt.Sprintf("mscr_function_%s_%s_params_%d", astNode.Type, astNode.Name, len(astNode.Parameters))
		newAsm = fmt.Sprintf(".%s __LABEL_SET\n", functionLabel)
		state.currentFunction = astNode.Name

	case Global:
		state.globalMemoryMap[astNode.Name] = state.maxGlobalAddr
		state.maxGlobalAddr++

	case Expression:
		if astNode.Asm != nil {
			newAsm = regexpAsmExtract.FindAllStringSubmatch(*astNode.Asm, -1)[0][1]
		}

	case TopExpression, RuntimeValue, lexer.Position:
		break

	default:
		log.Println("Instruction currently unsupported: " + reflect.TypeOf(astNode).String())
	}

	return newAsm
}

func asmForNodePost(nodeInterface interface{}, state *asmTransformState) {
	switch nodeInterface.(type) {
	case Function:
		state.currentFunction = ""
	}
}
