package compiler

import "log"

func optimizeAsmAll(input []*asmCmd) []*asmCmd {
	if len(input) <= 1 {
		return input
	}

	retval := input

	log.Println("Performing asm optimizations...")

	// Optimization passes
	retval = optimizePushPop(retval)
	retval = optimizeMov(retval)
	retval = optimizeDoubleMov(retval) // Has to happen *after* optimizeMov to avoid removal of "copy"-movs

	return retval
}

/*
	Reduce constellations like
		PUSH A
		POP B
	to
		MOV A B

	(Note: "PUSH A, POP A" will be optimized to "MOV A A", which is then subsequently optimized away in optimizeMov)
*/
func optimizePushPop(input []*asmCmd) []*asmCmd {
	retval := make([]*asmCmd, 1)
	retval[0] = input[0]

	for i := 1; i < len(input); i++ {
		if input[i-1].ins == "PUSH" && input[i].ins == "POP" {
			// Optimizible constelation detected
			retval[len(retval)-1].ins = "MOV"
			retval[len(retval)-1].params = []*asmParam{
				// This is... not fully safe. But since the asmCmds here all generated directly by the compiler, we *should* be good...
				rawAsmParam(input[i-1].params[0].value),
				rawAsmParam(input[i].params[0].value),
			}
		} else {
			retval = append(retval, input[i])
		}
	}

	return retval
}

/*
	Remove expressions à la
		MOV A A
	or
		MOV F F
*/
func optimizeMov(input []*asmCmd) []*asmCmd {
	retval := make([]*asmCmd, 0)

	for _, cmd := range input {
		if !(cmd.ins == "MOV" && cmd.params[0].value == cmd.params[1].value) {
			retval = append(retval, cmd)
		}
	}

	return retval
}

/*
	Reduce double MOVs like
		MOV A B
		MOV B A
	to a single
		MOV A B
	which has the same effect on register content
*/
func optimizeDoubleMov(input []*asmCmd) []*asmCmd {
	retval := make([]*asmCmd, 1)
	retval[0] = input[0]

	for i := 1; i < len(input); i++ {
		if !(input[i-1].ins == "MOV" && input[i].ins == "MOV" &&
			input[i-1].params[0].value == input[i].params[1].value &&
			input[i-1].params[1].value == input[i].params[0].value) {
			retval = append(retval, input[i])
		}
	}

	return retval
}
