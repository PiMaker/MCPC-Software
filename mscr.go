package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/PiMaker/mscr/compiler"
)

const LicenseNotice string = `
Copyright (C) 2019  Stefan Reiter (pimaker.at)
This program comes with ABSOLUTELY NO WARRANTY.
This is free software, and you are welcome to redistribute it
under certain conditions.
See https://github.com/PiMaker/mscr/blob/master/LICENSE for more.`

func main() {
	inputFile, outputFile, bootloader, verbose, version, optimizeDisable := processArgs()

	if version {
		fmt.Println("M-Script Compiler - Version " + compiler.CompilerVersion)
		fmt.Println(LicenseNotice)
		return
	}

	if inputFile == "" || outputFile == "" {
		log.Fatalln("Command line usage: mscr <input.mscr> <output.ma> [--bootloader] [--verbose] [--optimizedisable] [--version] [--help]")
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

func processArgs() (inputFile, outputFile string, bootloader, verbose, version, optimizeDisable bool) {
	verbose = false
	bootloader = false
	version = false
	optimizeDisable = false
	inputFile = ""
	outputFile = ""

	for i := 1; i < len(os.Args); i++ {
		arg := strings.ToLower(os.Args[i])
		if strings.HasPrefix(arg, "--") {
			if arg == "--verbose" {
				verbose = true
			} else if arg == "--bootloader" {
				bootloader = true
			} else if arg == "--version" {
				version = true
			} else if arg == "--optimizedisable" {
				optimizeDisable = true
			} else if arg == "--help" {
				// ignored
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
