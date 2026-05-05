package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadPrompts_MissingSoul(t *testing.T) {
	dir := t.TempDir()

	_, err := loadPrompts(filepath.Join(dir, "soul.md"), filepath.Join(dir, "instructions.md"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "soul prompt")
}

func TestLoadPrompts_MissingInstructions(t *testing.T) {
	dir := t.TempDir()
	soulPath := filepath.Join(dir, "soul.md")
	require.NoError(t, os.WriteFile(soulPath, []byte("# Soul"), 0600))

	_, err := loadPrompts(soulPath, filepath.Join(dir, "instructions.md"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "instructions prompt")
}

func TestLoadPrompts_ComposesBothFiles(t *testing.T) {
	dir := t.TempDir()
	soulPath := filepath.Join(dir, "soul.md")
	instrPath := filepath.Join(dir, "instructions.md")
	require.NoError(t, os.WriteFile(soulPath, []byte("# Soul"), 0600))
	require.NoError(t, os.WriteFile(instrPath, []byte("# Instructions"), 0600))

	prompt, err := loadPrompts(soulPath, instrPath)

	require.NoError(t, err)
	assert.Contains(t, prompt, "# Soul")
	assert.Contains(t, prompt, "# Instructions")
}
