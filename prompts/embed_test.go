package prompts

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaults_MatchSourceTemplates(t *testing.T) {
	soul, err := os.ReadFile("soul.md")
	require.NoError(t, err)
	instructions, err := os.ReadFile("instructions.md")
	require.NoError(t, err)

	assert.Equal(t, string(soul)+"\n\n"+string(instructions), Defaults())
}
