package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeMemoryFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
}

func TestReadMemoryFileTool_ReturnsFileContents(t *testing.T) {
	// arrange
	memDir := t.TempDir()
	writeMemoryFile(t, memDir, "general.md", "# general\n\n- user likes Go\n")
	tl := newReadMemoryFileTool(memDir)

	// act
	result, err := tl.InvokableRun(context.Background(), `{"filename":"general.md"}`)

	// assert
	require.NoError(t, err)
	assert.Contains(t, result, "user likes Go")
}

func TestReadMemoryFileTool_ReturnsErrorForMissingFile(t *testing.T) {
	// arrange
	tl := newReadMemoryFileTool(t.TempDir())

	// act
	result, err := tl.InvokableRun(context.Background(), `{"filename":"daily/2099-01-01.md"}`)

	// assert
	require.NoError(t, err)
	assert.Contains(t, result, "error:")
}

func TestReadMemoryFileTool_ReturnsErrorForPathTraversal(t *testing.T) {
	// arrange
	tl := newReadMemoryFileTool(t.TempDir())

	// act
	result, err := tl.InvokableRun(context.Background(), `{"filename":"../secret.txt"}`)

	// assert
	require.NoError(t, err)
	assert.Contains(t, result, "error: invalid filename")
}

func TestSearchMemoryTool_ReturnsMatchingLinesGroupedByFile(t *testing.T) {
	// arrange
	memDir := t.TempDir()
	writeMemoryFile(t, memDir, "general.md", "# general\n\n- user prefers dark mode\n- user uses Go\n")
	writeMemoryFile(t, memDir, "daily/2026-05-09.md", "# 2026-05-09\n\n- meeting about dark theme\n")
	tl := newSearchMemoryTool(memDir)

	// act
	result, err := tl.InvokableRun(context.Background(), `{"queries":["dark"]}`)

	// assert
	require.NoError(t, err)
	var got map[string][]string
	require.NoError(t, json.Unmarshal([]byte(result), &got))
	assert.Contains(t, got["general.md"][0], "dark mode")
	assert.Contains(t, got["daily/2026-05-09.md"][0], "dark theme")
}

func TestSearchMemoryTool_EmptyResultWhenNoMatch(t *testing.T) {
	// arrange
	memDir := t.TempDir()
	writeMemoryFile(t, memDir, "general.md", "# general\n\n- user likes Go\n")
	tl := newSearchMemoryTool(memDir)

	// act
	result, err := tl.InvokableRun(context.Background(), `{"queries":["python"]}`)

	// assert
	require.NoError(t, err)
	var got map[string][]string
	require.NoError(t, json.Unmarshal([]byte(result), &got))
	assert.Empty(t, got)
}

func TestSearchMemoryTool_MultipleQueriesMatchSameLine(t *testing.T) {
	// arrange
	memDir := t.TempDir()
	writeMemoryFile(t, memDir, "general.md", "# general\n\n- user prefers dark Go mode\n")
	tl := newSearchMemoryTool(memDir)

	// act — both queries would match the same line; it should appear once
	result, err := tl.InvokableRun(context.Background(), `{"queries":["dark","Go"]}`)

	// assert
	require.NoError(t, err)
	var got map[string][]string
	require.NoError(t, json.Unmarshal([]byte(result), &got))
	assert.Len(t, got["general.md"], 1)
}
