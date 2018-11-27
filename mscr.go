package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/PiMaker/mscr/compiler"
)

func main() {
	inputFile, outputFile, bootloader, verbose, version := processArgs()

	if version {
		fmt.Println("M-Script compiler v" + compiler.CompilerVersion)
		return
	}

	if inputFile == "" || outputFile == "" {
		log.Fatalln("Command line usage: mscr <input.mscr> <output.ma> [--bootloader] [--verbose]")
	}

	log.Println("Starting compilation of " + inputFile)

	tempFile := os.TempDir() + string(os.PathSeparator) + "preprocessed.mscr-tmp"

	compiler.Preprocess(inputFile, tempFile)
	ast := compiler.GenerateAST(tempFile)
	asm := []byte(ast.GenerateASM(bootloader, verbose))

	for _, ch := range ast.CommentHeaders {
		asm = append([]byte(ch+"\r\n"), asm...)
	}

	os.Remove(tempFile) // Errors ignored

	ioutil.WriteFile(outputFile, asm, 0644)

	log.Printf("Compilation completed, %d bytes written\n", len(asm))
}

func processArgs() (inputFile, outputFile string, bootloader, verbose, version bool) {
	verbose = false
	bootloader = false
	version = false
	inputFile = ""
	outputFile = ""

	for i := 1; i < len(os.Args); i++ {
		if strings.HasPrefix(os.Args[i], "--") {
			if os.Args[i] == "--verbose" {
				verbose = true
			} else if os.Args[i] == "--bootloader" {
				bootloader = true
			} else if os.Args[i] == "--version" {
				version = true
			} else {
				log.Println("WARN: Ignoring unknown command line flag: " + os.Args[i])
			}
		} else {
			if inputFile == "" {
				inputFile = os.Args[i]
			} else if outputFile == "" {
				outputFile = os.Args[i]
			} else {
				log.Println("WARN: Ignoring unknown command line argument: " + os.Args[i])
			}
		}
	}

	return
}
