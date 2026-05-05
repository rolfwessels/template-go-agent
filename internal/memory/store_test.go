package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileStore_SaveCreatesMarkdownFile(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := NewFileStore(dir)
	e := Entry{ID: "entry-1", UserID: "alice", SessionID: "sess-1", Content: "Alice likes Go"}

	// act
	err := store.Save(context.Background(), e)

	// assert
	require.NoError(t, err)
	path := filepath.Join(dir, "alice", "entry-1.md")
	_, statErr := os.Stat(path)
	assert.NoError(t, statErr)
}

func TestFileStore_AllReturnsStoredEntries(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := NewFileStore(dir)
	ctx := context.Background()
	_ = store.Save(ctx, Entry{ID: "e1", UserID: "alice", SessionID: "s1", Content: "fact one"})
	_ = store.Save(ctx, Entry{ID: "e2", UserID: "alice", SessionID: "s1", Content: "fact two"})

	// act
	entries, err := store.All(ctx, "alice")

	// assert
	require.NoError(t, err)
	require.Len(t, entries, 2)
	var contents []string
	for _, e := range entries {
		contents = append(contents, e.Content)
	}
	assert.ElementsMatch(t, []string{"fact one", "fact two"}, contents)
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
	_ = store.Save(ctx, Entry{ID: "e1", UserID: "bob", SessionID: "s1", Content: "Bob prefers dark mode"})

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
