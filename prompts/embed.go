// Package prompts contains the agent's embedded default prompts.
package prompts

import _ "embed"

//go:embed soul.md
var soul string

//go:embed instructions.md
var instructions string

// Defaults returns the persona and instructions embedded at build time.
func Defaults() string {
	return soul + "\n\n" + instructions
}
