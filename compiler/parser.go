package compiler

import (
	"github.com/alecthomas/participle/lexer"
)

type AST struct {
	TopExpressions []*TopExpression `{ @@ }`
}

type TopExpression struct {
	Pos lexer.Position

	Function *Function `@@`
	Global   *Global   `| @@ ";"`
}

type Expression struct {
	Pos lexer.Position

	Assignment   *Assignment   `(( @@`
	FunctionCall *FunctionCall `| @@`
	Variable     *Variable     `| @@`
	Return       *RuntimeValue `| "return" @@) ";")`

	WhileLoop   *WhileLoop   `| @@`
	ForLoop     *ForLoop     `| @@`
	IfCondition *Conditional `| @@`
	Asm         *string      `| @ASM`
}

type Function struct {
	Pos lexer.Position

	Inline     bool          `"func" [@"inline"]`
	Type       string        `@("var"|"void")`
	Name       string        `@Ident`
	Parameters []string      `"(" { @Ident [","] } ")"`
	Body       []*Expression `"{" { @@ } "}"`
}

type ForLoop struct {
	Pos lexer.Position

	IsVar        bool          `"for" @"var"`
	IteratorName string        `@Ident`
	From         int           `"from" @Int`
	To           int           `"to" @Int`
	Body         []*Expression `"{" { @@ } "}"`
}

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

	Name  string        `"var" @Ident`
	Value *RuntimeValue `["=" @@]`
}

type Assignment struct {
	Pos lexer.Position

	Name     string        `@Ident`
	Operator string        `@AssignmentOperator`
	Value    *RuntimeValue `@@`
}

type FunctionCall struct {
	Pos lexer.Position

	FunctionName string          `@Ident`
	Parameters   []*RuntimeValue `"(" { @@ [","] } ")"`
}

type RVFunctionCall struct {
	Pos lexer.Position

	FunctionName string          `@Ident`
	Parameters   []*RuntimeValue `"(" { @@ [","] } ")"`
}

type RuntimeValue struct {
	Pos lexer.Position

	FunctionCall *RVFunctionCall `  @@`
	Eval         *string         `| @Eval`
	Number       *int            `| @Int`
	Ident        *string         `| @Ident`
}

type Global struct {
	Pos lexer.Position

	Name  string `"global" @Ident`
	Value *Value `["=" @@]`
}

type Value struct {
	Pos lexer.Position

	Text   *string `  @String`
	Number *int    `| @Int`
}
