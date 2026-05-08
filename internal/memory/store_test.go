package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileStore_SaveCreatesDateFile(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := NewFileStore(dir)
	e := Entry{UserID: "alice", Content: "Alice likes Go"}
	date := time.Now().UTC().Format("2006-01-02")

	// act
	err := store.Save(context.Background(), e)

	// assert
	require.NoError(t, err)
	path := filepath.Join(dir, "alice", fmt.Sprintf("%s.md", date))
	_, statErr := os.Stat(path)
	assert.NoError(t, statErr)
}

func TestFileStore_SaveAppendsFacts(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := NewFileStore(dir)
	ctx := context.Background()
	date := time.Now().UTC().Format("2006-01-02")

	// act
	require.NoError(t, store.Save(ctx, Entry{UserID: "alice", Content: "fact one"}))
	require.NoError(t, store.Save(ctx, Entry{UserID: "alice", Content: "fact two"}))

	// assert
	data, err := os.ReadFile(filepath.Join(dir, "alice", fmt.Sprintf("%s.md", date)))
	require.NoError(t, err)
	body := string(data)
	assert.Contains(t, body, "- fact one")
	assert.Contains(t, body, "- fact two")
}

func TestFileStore_DifferentSessionsSameDay_SingleFile(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := NewFileStore(dir)
	ctx := context.Background()

	// act — two saves representing facts from different sessions
	require.NoError(t, store.Save(ctx, Entry{UserID: "alice", Content: "fact from session one"}))
	require.NoError(t, store.Save(ctx, Entry{UserID: "alice", Content: "fact from session two"}))

	// assert — only one file exists (not one per session)
	entries, err := os.ReadDir(filepath.Join(dir, "alice"))
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestFileStore_AllReturnsStoredFacts(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := NewFileStore(dir)
	ctx := context.Background()
	_ = store.Save(ctx, Entry{UserID: "alice", Content: "fact one"})
	_ = store.Save(ctx, Entry{UserID: "alice", Content: "fact two"})

	// act
	entries, err := store.All(ctx, "alice")

	// assert
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0].Content, "fact one")
	assert.Contains(t, entries[0].Content, "fact two")
}

func TestFileStore_AllReturnsEmptyForUnknownUser(t *testing.T) {
	// arrange
	store := NewFileStore(t.TempDir())

	// act
	entries, err := store.All(context.Background(), "unknown")

	// assert
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestFileStore_AllAsContextFormatsEntries(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := NewFileStore(dir)
	ctx := context.Background()
	_ = store.Save(ctx, Entry{UserID: "bob", Content: "Bob prefers dark mode"})

	// act
	got, err := store.AllAsContext(ctx, "bob")

	// assert
	require.NoError(t, err)
	assert.Contains(t, got, "Bob prefers dark mode")
}

func TestFileStore_AllAsContextEmptyForNoEntries(t *testing.T) {
	// arrange
	store := NewFileStore(t.TempDir())

	// act
	got, err := store.AllAsContext(context.Background(), "nobody")

	// assert
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestFileStore_SaveDateHeaderInFile(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := NewFileStore(dir)
	date := time.Now().UTC().Format("2006-01-02")

	// act
	require.NoError(t, store.Save(context.Background(), Entry{UserID: "carol", Content: "a fact"}))

	// assert — file starts with date header
	entries, _ := os.ReadDir(filepath.Join(dir, "carol"))
	require.Len(t, entries, 1)
	data, _ := os.ReadFile(filepath.Join(dir, "carol", entries[0].Name()))
	assert.True(t, strings.HasPrefix(string(data), fmt.Sprintf("# %s", date)))
}
