package compiler

const AssigneableRegisters = 4

// Parameter types for meta-assembly
// An asmCmd with only asmParamTypeRaw-type parameters is considered "fully resolved"
const asmParamTypeRaw = 0
const asmParamTypeVarRead = 1
const asmParamTypeVarWrite = 2
const asmParamTypeCalc = 4
const asmParamTypeGlobalWrite = 8
const asmParamTypeGlobalRead = 16
const asmParamTypeScopeVarCount = 32
const asmParamTypeStringRead = 64
const asmParamTypeVarAddr = 128
const asmParamTypeStringAddr = 256
const asmParamTypeGlobalAddr = 512

type asmCmd struct {
	ins    string
	params []*asmParam

	// Encompassing function name
	scope string

	// For meta-assembly-only commands; these will never be directly represented in output asm
	scopeAnnotationName     string
	scopeAnnotationRegister int

	// For output formatting
	comment     string
	printIndent int

	// For verbose printing
	originalAsmCmdString string
}

type asmParam struct {
	asmParamType int
	value        string

	// For resolving globals and strings
	addrCache int

	// For calc expressions
	subAST *RuntimeValue
}

type asmTransformState struct {
	currentFunction           string
	currentScopeVariableCount int

	functionTable []asmFunc

	globalMemoryMap map[string]int
	maxDataAddr     int

	typeMap     map[string]*asmType
	variableMap map[string][]asmVar
	stringMap   map[string]int

	specificInitializationAsm []*asmCmd
	binData                   []int16

	scopeRegisterAssignment  map[string]int
	scopeRegisterDirty       map[int]bool
	scopeVariableDirectMarks map[string]bool

	printIndent int
	verbose     bool
}

type asmVar struct {
	name        string
	orderNumber int
	isGlobal    bool
	asmType     *asmType
}

type asmType struct {
	name    string
	size    int // in words
	builtin bool

	members []asmTypeMember
}

type asmTypeMember struct {
	name    string
	asmType *asmType
}

type asmFunc struct {
	name       string
	label      string
	params     []asmTypeMember
	returnType *asmType
}
