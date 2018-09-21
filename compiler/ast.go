package compiler

import (
	"fmt"
	"log"
	"reflect"
	"strconv"
	"unicode"
)

func (ast *AST) GenerateASM() string {

	log.Println("Generating ASM...")

	/*fmt.Println("AST:")
	walkInterface(ast, func(val reflect.Value, name string, depth int) {
		for i := 0; i < depth+1; i++ {
			fmt.Print("  ")
		}
		fmt.Print(name)

		if val.Kind() == reflect.Struct {
			fmt.Println()
		} else if val.Kind() == reflect.Int {
			fmt.Print(" = ")
			fmt.Println(val.Int())
		} else if val.Kind() == reflect.Bool {
			fmt.Print(" = ")
			fmt.Println(val.Bool())
		} else {
			fmt.Print(" = ")
			fmt.Println(val.String())
		}
	}, nil, 0)*/

	asm := ""

	// Compilation tables
	var globalTable []*Global // TODO: Make map
	var functionTable []*Function

	// Fill tables
	walkInterface(ast, func(val reflect.Value, name string, depth int) {

		if val.Kind() != reflect.Struct {
			// Early out if value instead of node
			return
		}

		nodeInterface := val.Interface()

		switch node := nodeInterface.(type) {

		case Global:
			for _, g := range globalTable {
				if g.Name == node.Name {
					log.Fatalf("Redefinition of global '%s' at %s\n", node.Name, node.Pos.String())
				}
			}
			globalTable = append(globalTable, &node)

		case Function:
			for _, f := range functionTable {
				if f.Name == node.Name {
					log.Fatalf("Redefinition of function '%s' at %s\n", node.Name, node.Pos.String())
				}
			}
			nodeP := &node
			nodeP.functionLabel = "mscr_function_" + node.Name + "_params_" + strconv.Itoa(len(node.Parameters)) + "_"
			functionTable = append(functionTable, nodeP)
		}

	}, nil, 0)

	// Output ASM
	walkInterface(ast, func(val reflect.Value, name string, depth int) {

		if val.Kind() != reflect.Struct {
			// Early out if value instead of node
			return
		}

		nodeInterface := val.Interface()

		newAsm := ""

		switch node := nodeInterface.(type) {

		case Function:
			newAsm = fmt.Sprintf(".%s MOV A A\n", functionTable[0].functionLabel)
			node.Inline = false

		default:
			log.Println("Instruction currently unsupported: " + val.Type().String())
		}

		if newAsm != "" {
			asm = fmt.Sprintf("%s\n; %s\n%s\n", asm, name, newAsm)
		}

	}, nil, 0)

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
					pre(s2, styp.Name, level)
				}
				walkInterface(s2.Interface(), pre, post, level+1)
				if post != nil {
					post(s2, styp.Name, level)
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
				pre(s, styp.Name, level)
			}

			// Check exported status
			fletter := []rune(styp.Name)[0]
			if unicode.IsLetter(fletter) && unicode.IsUpper(fletter) {
				walkInterface(s.Interface(), pre, post, level+1)
			}

			if post != nil {
				post(s, styp.Name, level)
			}
		}
	}
}
