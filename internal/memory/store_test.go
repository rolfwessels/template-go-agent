package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileStore_SaveGeneral_CreatesGeneralFile(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := NewFileStore(dir)

	// act
	err := store.Save(context.Background(), "alice", Fact{Content: "Alice likes Go", Kind: KindGeneral})

	// assert
	require.NoError(t, err)
	path := filepath.Join(dir, "user", "alice", "memory", "general.md")
	_, statErr := os.Stat(path)
	assert.NoError(t, statErr)
}

func TestFileStore_SaveDaily_CreatesDailyFile(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := NewFileStore(dir)
	date := time.Now().UTC().Format("2006-01-02")

	// act
	err := store.Save(context.Background(), "alice", Fact{Content: "meeting today", Kind: KindDaily})

	// assert
	require.NoError(t, err)
	path := filepath.Join(dir, "user", "alice", "memory", "daily", date+".md")
	_, statErr := os.Stat(path)
	assert.NoError(t, statErr)
}

func TestFileStore_SaveGeneral_AppendsFacts(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := NewFileStore(dir)
	ctx := context.Background()

	// act
	require.NoError(t, store.Save(ctx, "alice", Fact{Content: "fact one", Kind: KindGeneral}))
	require.NoError(t, store.Save(ctx, "alice", Fact{Content: "fact two", Kind: KindGeneral}))

	// assert
	data, err := os.ReadFile(filepath.Join(dir, "user", "alice", "memory", "general.md"))
	require.NoError(t, err)
	body := string(data)
	assert.Contains(t, body, "- fact one")
	assert.Contains(t, body, "- fact two")
}

func TestFileStore_UpdateIndex_UsesMarkdownLinks(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := NewFileStore(dir)
	ctx := context.Background()
	date := time.Now().UTC().Format("2006-01-02")
	require.NoError(t, store.Save(ctx, "alice", Fact{Content: "pref", Kind: KindGeneral}))
	require.NoError(t, store.Save(ctx, "alice", Fact{Content: "event", Kind: KindDaily}))

	// act
	err := store.UpdateIndex("alice", map[string]string{
		"daily/" + date + ".md": "Some event happened today.",
	})

	// assert — Markdown link syntax with provided summary
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(dir, "user", "alice", "memory", "MEMORY.md"))
	require.NoError(t, err)
	body := string(data)
	assert.Contains(t, body, "[general.md](general.md)")
	assert.Contains(t, body, "[daily/"+date+".md](daily/"+date+".md)")
	assert.Contains(t, body, "Some event happened today.")
}

func TestFileStore_UpdateIndex_DefaultDescriptionWhenNoSummary(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := NewFileStore(dir)
	ctx := context.Background()
	date := time.Now().UTC().Format("2006-01-02")
	require.NoError(t, store.Save(ctx, "alice", Fact{Content: "event", Kind: KindDaily}))

	// act — pass empty summaries map
	require.NoError(t, store.UpdateIndex("alice", map[string]string{}))

	// assert — falls back to "facts from {date}"
	data, _ := os.ReadFile(filepath.Join(dir, "user", "alice", "memory", "MEMORY.md"))
	assert.Contains(t, string(data), "facts from "+date)
}

func TestFileStore_UpdateIndex_OmitsGeneralWhenAbsent(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := NewFileStore(dir)
	ctx := context.Background()
	require.NoError(t, store.Save(ctx, "alice", Fact{Content: "event", Kind: KindDaily}))

	// act
	require.NoError(t, store.UpdateIndex("alice", nil))

	// assert
	data, _ := os.ReadFile(filepath.Join(dir, "user", "alice", "memory", "MEMORY.md"))
	assert.NotContains(t, string(data), "general.md")
}

func TestFileStore_UpdateIndex_PreservesExistingSummaries(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := NewFileStore(dir)
	ctx := context.Background()
	date := time.Now().UTC().Format("2006-01-02")
	require.NoError(t, store.Save(ctx, "alice", Fact{Content: "event", Kind: KindDaily}))
	// First sweep writes a good summary
	require.NoError(t, store.UpdateIndex("alice", map[string]string{
		"daily/" + date + ".md": "Original great summary.",
	}))

	// act — second sweep has no new summaries for the existing file
	require.NoError(t, store.UpdateIndex("alice", map[string]string{}))

	// assert — original summary preserved
	data, _ := os.ReadFile(filepath.Join(dir, "user", "alice", "memory", "MEMORY.md"))
	assert.Contains(t, string(data), "Original great summary.")
}

func TestFileStore_UpdateIndex_NewSummaryOverridesOld(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := NewFileStore(dir)
	ctx := context.Background()
	date := time.Now().UTC().Format("2006-01-02")
	require.NoError(t, store.Save(ctx, "alice", Fact{Content: "event", Kind: KindDaily}))
	require.NoError(t, store.UpdateIndex("alice", map[string]string{
		"daily/" + date + ".md": "Old summary.",
	}))

	// act — new sweep provides an updated summary
	require.NoError(t, store.UpdateIndex("alice", map[string]string{
		"daily/" + date + ".md": "New updated summary.",
	}))

	// assert — new summary replaces old
	data, _ := os.ReadFile(filepath.Join(dir, "user", "alice", "memory", "MEMORY.md"))
	assert.Contains(t, string(data), "New updated summary.")
	assert.NotContains(t, string(data), "Old summary.")
}

func TestFileStore_AllAsContext_IncludesMemoryIndexAndGeneral(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := NewFileStore(dir)
	ctx := context.Background()
	require.NoError(t, store.Save(ctx, "alice", Fact{Content: "Alice likes Go", Kind: KindGeneral}))
	require.NoError(t, store.UpdateIndex("alice", nil))

	// act
	got, err := store.AllAsContext(ctx, "alice")

	// assert
	require.NoError(t, err)
	assert.Contains(t, got, "Alice likes Go")
	assert.Contains(t, got, "general.md")
}

func TestFileStore_AllAsContext_EmptyForNewUser(t *testing.T) {
	// arrange
	store := NewFileStore(t.TempDir())

	// act
	got, err := store.AllAsContext(context.Background(), "nobody")

	// assert
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestFileStore_GeneralFileHasHeader(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := NewFileStore(dir)

	// act
	require.NoError(t, store.Save(context.Background(), "carol", Fact{Content: "a fact", Kind: KindGeneral}))

	// assert
	data, _ := os.ReadFile(filepath.Join(dir, "user", "carol", "memory", "general.md"))
	assert.True(t, strings.HasPrefix(string(data), "# general\n"))
}
