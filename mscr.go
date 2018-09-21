package main

import (
	"io/ioutil"
	"log"
	"os"

	"github.com/PiMaker/mscr/compiler"
)

func main() {
	//os.Args = []string{"", ".\\examples\\parsing_test.mscr", ".\\examples\\alphabet.ma"}

	if len(os.Args) != 3 {
		log.Fatalln("Command line usage: mscr <input.mscr> <output.ma>")
	}

	inputFile := os.Args[1]
	outputFile := os.Args[2]

	log.Println("Starting compilation of " + inputFile)

	tempFile := os.TempDir() + string(os.PathSeparator) + "preprocessed.mscr-tmp"

	compiler.Preprocess(inputFile, tempFile)
	ast := compiler.GenerateAST(tempFile)
	asm := []byte(ast.GenerateASM())

	os.Remove(tempFile) // Errors ignored

	ioutil.WriteFile(outputFile, asm, 0644)

	log.Printf("Compilation completed, %d bytes written\n", len(asm))
}
