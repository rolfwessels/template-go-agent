package discord

import _ "embed"

//go:embed instructions.md
var agentInstructions string

func Instructions() string {
	return agentInstructions
}
