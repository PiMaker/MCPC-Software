package compiler

import (
	"github.com/alecthomas/participle/lexer"
)

type AST struct {
	TopExpressions []*TopExpression `{ @@ }`

	CommentHeaders []string
}

type TopExpression struct {
	Pos lexer.Position

	Function *Function `@@`
	Struct   *Struct   `| @@`
	Global   *Global   `| @@ ";"`
	View     *View     `| @@ ";"`
}

type Expression struct {
	Pos lexer.Position

	Assignment   *Assignment   `(( @@`
	FunctionCall *FunctionCall `| @@`
	Return       *RuntimeValue `| "return" @@`
	Variable     *Variable     `| @@) ";")`

	WhileLoop *WhileLoop `| @@`
	//	ForLoop     *ForLoop     `| @@`
	IfCondition *Conditional `| @@`
	Asm         *string      `| @ASM`
}

type Struct struct {
	Pos lexer.Position

	Name    string          `"struct" @Ident`
	Members []*StructMember `"{" { @@ } "}"`
}

type StructMember struct {
	Pos lexer.Position

	Type string `@Ident`
	Name string `@Ident ";"`
}

type Function struct {
	Pos lexer.Position

	Inline     bool                 `"func" [@"inline"]`
	Type       string               `@Ident`
	Name       string               `@Ident`
	Parameters []*FunctionParameter `"(" { @@ [","] } ")"`
	Body       []*Expression        `"{" { @@ } "}"`
}

type FunctionParameter struct {
	Pos lexer.Position

	Type string `@Ident`
	Name string `@Ident`
}

// type ForLoop struct {
// 	Pos lexer.Position

// 	IsVar        bool          `"for" @"var"`
// 	IteratorName string        `@Ident`
// 	From         int           `"from" @Int`
// 	To           int           `"to" @Int`
// 	Body         []*Expression `"{" { @@ } "}"`
// }

type WhileLoop struct {
	Pos lexer.Position

	Condition string        `"while" @Eval`
	Body      []*Expression `"{" { @@ } "}"`
}

type Conditional struct {
	Pos lexer.Position

	Condition string        `"if" @Eval`
	BodyIf    []*Expression `"{" { @@ } "}"`
	BodyElse  []*Expression `["else" "{" { @@ } "}"]`
}

type Variable struct {
	Pos lexer.Position

	Type  string        `@Ident`
	Name  string        `@Ident`
	Value *RuntimeValue `["=" @@]`
}

type Assignment struct {
	Pos lexer.Position

	Name     string        `@(IdentWithDot|Ident)`
	Operator string        `@AssignmentOperator`
	Value    *RuntimeValue `@@`
}

type FunctionCall struct {
	Pos lexer.Position

	FunctionName string          `@Ident`
	Parameters   []*RuntimeValue `"(" { @@ [","] } ")"`
}

type Global struct {
	Pos lexer.Position

	Type  string `"global" @Ident`
	Name  string `@Ident`
	Value *Value `["=" @@]`
}

type View struct {
	Pos lexer.Position

	Name    string `"view" @Ident`
	Address int    `"@"@Int`
}

type Value struct {
	Pos lexer.Position

	Text   *string `  @String`
	Number *int    `| @Int`
}

// RuntimeValue represents a programmatic value that will be resolved to a single results at runtime.
// It's implemented as an expression parser for supported mathematical expressions and function calls.
// A RuntimeValue can be seen as a "sub-AST" that encompasses all necessary information to be transformed into meta ASM.

type RuntimeValue struct {
	Pos lexer.Position

	Left  RVValue  `@@`
	Right *RVOpExp `[ @@ ]`

	/*FunctionCall *RVFunctionCall `  @@`
	Eval         *string         `| @Eval`
	Number       *int            `| @Int`
	Variable     *string         `| @(IdentWithDot|Ident)`*/
}

type RVFunctionCall struct {
	Pos lexer.Position

	FunctionName string          `@Ident`
	Parameters   []*RuntimeValue `"(" { @@ [","] } ")"`
}

type RVValue struct {
	Pos lexer.Position

	Number        *int            `@Int`
	Variable      *string         `| @(IdentWithDot|Ident)`
	FunctionCall  *RVFunctionCall `| @@`
	SubExpression *RuntimeValue   `| "(" @@ ")"`
}

type RVOpExp struct {
	Pos lexer.Position

	Operator *string       `@Operator`
	Right    *RuntimeValue `@@`
}
