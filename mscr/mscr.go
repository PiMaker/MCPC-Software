package mscr

import (
	"io/ioutil"
	"log"
	"os"

	"github.com/PiMaker/MCPC-Software/mscr/compiler"
)

func CompileMSCR(inputFile, outputFile string, bootloader, verbose, optimizeDisable bool) {

	if inputFile == "" || outputFile == "" {
		panic("You need to specify an input and output file combination for MSCR.")
	}

	log.Println("Starting compilation of " + inputFile)

	tempFile := os.TempDir() + string(os.PathSeparator) + "preprocessed.mscr-tmp"

	compiler.Preprocess(inputFile, tempFile)
	ast := compiler.GenerateAST(tempFile)
	asm := []byte(ast.GenerateASM(bootloader, verbose, optimizeDisable))

	for _, ch := range ast.CommentHeaders {
		asm = append([]byte(ch+"\r\n"), asm...)
	}

	os.Remove(tempFile) // Errors ignored

	ioutil.WriteFile(outputFile, asm, 0644)

	log.Printf("Compilation completed, %d bytes written\n", len(asm))
}
