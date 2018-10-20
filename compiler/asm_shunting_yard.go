package compiler

import (
	"log"
	"strings"
)

type YardToken struct {
	value     string
	tokenType string
}

const shuntSplit = "·"

func parseIntoYardTokens(rawOutput string) []*YardToken {
	lines := strings.Split(rawOutput, "\n")
	retval := make([]*YardToken, 0)

	for _, line := range lines {
		split := strings.Split(line, shuntSplit)
		if len(split) == 2 {
			retval = append(retval, &YardToken{
				value:     split[1],
				tokenType: split[0],
			})
		} else if len(split) > 2 {
			log.Println("WARN: Encountered unexpected output from shunting yard parser: " + line)
		}
	}

	return retval
}
