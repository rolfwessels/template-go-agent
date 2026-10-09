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
	date := time.Now().UTC().Format("2006-01-02")
	err := store.Save(context.Background(), "alice", date, Fact{Content: "Alice likes Go", Kind: KindGeneral})

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
	err := store.Save(context.Background(), "alice", date, Fact{Content: "meeting today", Kind: KindDaily})

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
	date := time.Now().UTC().Format("2006-01-02")
	require.NoError(t, store.Save(ctx, "alice", date, Fact{Content: "fact one", Kind: KindGeneral}))
	require.NoError(t, store.Save(ctx, "alice", date, Fact{Content: "fact two", Kind: KindGeneral}))

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
	require.NoError(t, store.Save(ctx, "alice", date, Fact{Content: "pref", Kind: KindGeneral}))
	require.NoError(t, store.Save(ctx, "alice", date, Fact{Content: "event", Kind: KindDaily}))

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
	require.NoError(t, store.Save(ctx, "alice", date, Fact{Content: "event", Kind: KindDaily}))

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
	date := time.Now().UTC().Format("2006-01-02")
	require.NoError(t, store.Save(ctx, "alice", date, Fact{Content: "event", Kind: KindDaily}))

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
	require.NoError(t, store.Save(ctx, "alice", date, Fact{Content: "event", Kind: KindDaily}))
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
	require.NoError(t, store.Save(ctx, "alice", date, Fact{Content: "event", Kind: KindDaily}))
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
	date := time.Now().UTC().Format("2006-01-02")
	require.NoError(t, store.Save(ctx, "alice", date, Fact{Content: "Alice likes Go", Kind: KindGeneral}))
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
	date := time.Now().UTC().Format("2006-01-02")
	require.NoError(t, store.Save(context.Background(), "carol", date, Fact{Content: "a fact", Kind: KindGeneral}))

	// assert
	data, _ := os.ReadFile(filepath.Join(dir, "user", "carol", "memory", "general.md"))
	assert.True(t, strings.HasPrefix(string(data), "# general\n"))
}

func TestFileStore_SaveDeduplicatesTrimmedContentPerFile(t *testing.T) {
	store := NewFileStore(t.TempDir())
	ctx := context.Background()
	for _, date := range []string{"2026-04-18", "2026-04-19"} {
		for _, content := range []string{"fact", "  fact\n", "fact with more detail", "fact"} {
			require.NoError(t, store.Save(ctx, "alice", date, Fact{Content: content, Kind: KindGeneral}))
		}
		daily, err := store.ReadDailyFile("alice", date)
		require.NoError(t, err)
		assert.Equal(t, 1, strings.Count(daily, "- fact\n"))
		assert.Equal(t, 1, strings.Count(daily, "- fact with more detail\n"))
	}
	general, err := os.ReadFile(filepath.Join(store.MemoryDir("alice"), "general.md"))
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(general), "- fact\n"))
	assert.Equal(t, 1, strings.Count(string(general), "- fact with more detail\n"))
}

func TestFileStore_SaveGeneralRetriesPartialLayeredWrite(t *testing.T) {
	for _, blockedFile := range []string{"general.md", "daily/2026-04-18.md"} {
		t.Run(blockedFile, func(t *testing.T) {
			store := NewFileStore(t.TempDir())
			memDir := store.MemoryDir("alice")
			blockedPath := filepath.Join(memDir, blockedFile)
			// A directory at the destination reliably fails writes without
			// permission assumptions, including when tests run as root.
			require.NoError(t, os.MkdirAll(blockedPath, 0750))
			fact := Fact{Content: "general fact", Kind: KindGeneral}
			require.Error(t, store.Save(context.Background(), "alice", "2026-04-18", fact))
			if blockedFile == "general.md" {
				daily, err := store.ReadDailyFile("alice", "2026-04-18")
				require.NoError(t, err)
				assert.Empty(t, daily, "a general failure must not expose a daily copy to the distiller")
			} else {
				general, err := os.ReadFile(filepath.Join(memDir, "general.md"))
				require.NoError(t, err)
				assert.Contains(t, string(general), "- general fact\n")
			}
			require.NoError(t, os.Remove(blockedPath))

			store = NewFileStore(store.dir)
			require.NoError(t, store.Save(context.Background(), "alice", "2026-04-18", fact))
			for _, rel := range []string{"general.md", "daily/2026-04-18.md"} {
				data, err := os.ReadFile(filepath.Join(memDir, rel))
				require.NoError(t, err)
				assert.Equal(t, 1, strings.Count(string(data), "- general fact\n"))
			}
		})
	}
}

func TestFileStore_UpdateIndexReturnsReadFailures(t *testing.T) {
	for _, blockedFile := range []string{"MEMORY.md", "daily"} {
		t.Run(blockedFile, func(t *testing.T) {
			store := NewFileStore(t.TempDir())
			memDir := store.MemoryDir("alice")
			require.NoError(t, os.MkdirAll(memDir, 0750))
			if blockedFile == "MEMORY.md" {
				require.NoError(t, os.Mkdir(filepath.Join(memDir, blockedFile), 0750))
			} else {
				require.NoError(t, os.WriteFile(filepath.Join(memDir, blockedFile), []byte("not a directory"), 0600))
			}
			require.Error(t, store.UpdateIndex("alice", nil))
		})
	}
}
