package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubDistiller struct {
	facts   []Fact
	summary string
}

func (s *stubDistiller) DistillAndSummarize(_ context.Context, _ string, _ []*schema.Message) ([]Fact, string, error) {
	return s.facts, s.summary, nil
}

type countingDistiller struct {
	lastCount int
}

func (d *countingDistiller) DistillAndSummarize(_ context.Context, _ string, msgs []*schema.Message) ([]Fact, string, error) {
	d.lastCount = len(msgs)
	return nil, "", nil
}

type capturingDistiller struct {
	receivedExisting string
	facts            []Fact
	summary          string
}

func (c *capturingDistiller) DistillAndSummarize(_ context.Context, existingDaily string, _ []*schema.Message) ([]Fact, string, error) {
	c.receivedExisting = existingDaily
	return c.facts, c.summary, nil
}

func newSweeperFixture(t *testing.T) (storeDir string, store *FileStore, sessions *SessionStore) {
	t.Helper()
	base := t.TempDir()
	storeDir = filepath.Join(base, "store")
	return storeDir, NewFileStore(storeDir), NewSessionStore(filepath.Join(base, "sessions"))
}

func TestSweeper_WritesCorrectFilesOnDestroy(t *testing.T) {
	// arrange
	storeDir, store, sessions := newSweeperFixture(t)
	distiller := &stubDistiller{
		facts: []Fact{
			{Content: "user prefers brevity", Kind: KindGeneral},
			{Content: "user works in Go", Kind: KindDaily},
		},
		summary: "Go developer who prefers brevity.",
	}
	sweeper := NewSweeper(store, distiller, sessions)
	require.NoError(t, sessions.Append("alice", "sess-1", "user", "hello"))
	require.NoError(t, sessions.Append("alice", "sess-1", "assistant", "hi"))
	date := time.Now().UTC().Format("2006-01-02")

	// act
	require.NoError(t, sweeper.OnEvict(context.Background(), "alice", "sess-1"))

	// assert — general.md, daily/{date}.md, and MEMORY.md all created
	_, err := os.Stat(filepath.Join(storeDir, "user", "alice", "memory", "general.md"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(storeDir, "user", "alice", "memory", "daily", date+".md"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(storeDir, "user", "alice", "memory", "MEMORY.md"))
	require.NoError(t, err)
}

func TestSweeper_MemoryMdUsesMarkdownLinks(t *testing.T) {
	// arrange
	_, store, sessions := newSweeperFixture(t)
	distiller := &stubDistiller{
		facts:   []Fact{{Content: "user works in Go", Kind: KindDaily}},
		summary: "Go developer.",
	}
	sweeper := NewSweeper(store, distiller, sessions)
	require.NoError(t, sessions.Append("bob", "sess-1", "user", "hello"))
	date := time.Now().UTC().Format("2006-01-02")

	// act
	require.NoError(t, sweeper.OnEvict(context.Background(), "bob", "sess-1"))

	// assert — MEMORY.md uses Markdown link syntax with LLM summary
	data, err := os.ReadFile(filepath.Join(store.MemoryDir("bob"), "MEMORY.md"))
	require.NoError(t, err)
	body := string(data)
	assert.Contains(t, body, "[daily/"+date+".md](daily/"+date+".md)")
	assert.Contains(t, body, "Go developer.")
}

func TestSweeper_AdvancesCursorAfterSweep(t *testing.T) {
	// arrange
	_, store, sessions := newSweeperFixture(t)
	distiller := &stubDistiller{facts: []Fact{{Content: "a fact", Kind: KindGeneral}}}
	sweeper := NewSweeper(store, distiller, sessions)
	require.NoError(t, sessions.Append("eve", "sess-1", "user", "msg1"))
	require.NoError(t, sessions.Append("eve", "sess-1", "assistant", "resp1"))

	// act
	require.NoError(t, sweeper.OnEvict(context.Background(), "eve", "sess-1"))

	// assert — cursor moved to 2 (both lines processed)
	cursor, err := sessions.ReadCursor("eve", "sess-1")
	require.NoError(t, err)
	assert.Equal(t, 2, cursor)
}

func TestSweeper_LayeredWrite_GeneralFactInBothFiles(t *testing.T) {
	// arrange
	storeDir, store, sessions := newSweeperFixture(t)
	distiller := &stubDistiller{
		facts:   []Fact{{Content: "user prefers Go", Kind: KindGeneral}},
		summary: "User prefers Go.",
	}
	sweeper := NewSweeper(store, distiller, sessions)
	require.NoError(t, sessions.Append("alice", "sess-1", "user", "msg"))
	date := time.Now().UTC().Format("2006-01-02")

	// act
	require.NoError(t, sweeper.OnEvict(context.Background(), "alice", "sess-1"))

	// assert — general fact written to both general.md and daily/{date}.md
	generalData, err := os.ReadFile(filepath.Join(storeDir, "user", "alice", "memory", "general.md"))
	require.NoError(t, err)
	assert.Contains(t, string(generalData), "user prefers Go")

	dailyData, err := os.ReadFile(filepath.Join(storeDir, "user", "alice", "memory", "daily", date+".md"))
	require.NoError(t, err)
	assert.Contains(t, string(dailyData), "user prefers Go")
}

func TestSweeper_LayeredWrite_UntaggedFactInDailyOnly(t *testing.T) {
	// arrange
	storeDir, store, sessions := newSweeperFixture(t)
	distiller := &stubDistiller{
		facts: []Fact{{Content: "user watched a movie", Kind: KindDaily}},
	}
	sweeper := NewSweeper(store, distiller, sessions)
	require.NoError(t, sessions.Append("alice", "sess-1", "user", "msg"))
	date := time.Now().UTC().Format("2006-01-02")

	// act
	require.NoError(t, sweeper.OnEvict(context.Background(), "alice", "sess-1"))

	// assert — untagged fact in daily only, not in general.md
	dailyData, err := os.ReadFile(filepath.Join(storeDir, "user", "alice", "memory", "daily", date+".md"))
	require.NoError(t, err)
	assert.Contains(t, string(dailyData), "user watched a movie")

	_, err = os.Stat(filepath.Join(storeDir, "user", "alice", "memory", "general.md"))
	assert.True(t, os.IsNotExist(err))
}

func TestSweeper_PassesExistingDailyContent(t *testing.T) {
	// arrange
	_, store, sessions := newSweeperFixture(t)
	capturer := &capturingDistiller{
		facts: []Fact{{Content: "first fact", Kind: KindDaily}},
	}
	sweeper := NewSweeper(store, capturer, sessions)

	// first sweep writes a fact
	require.NoError(t, sessions.Append("alice", "sess-1", "user", "first msg"))
	require.NoError(t, sweeper.OnEvict(context.Background(), "alice", "sess-1"))

	// second sweep — distiller should receive the daily file content from the first sweep
	capturer.facts = nil
	require.NoError(t, sessions.Append("alice", "sess-1", "user", "second msg"))
	require.NoError(t, sweeper.OnEvict(context.Background(), "alice", "sess-1"))

	// assert — existing daily content was passed to the distiller
	assert.Contains(t, capturer.receivedExisting, "first fact")
}

func TestSweeper_SkipsEmptyHistory(t *testing.T) {
	// arrange
	storeDir, store, sessions := newSweeperFixture(t)
	distiller := &stubDistiller{facts: []Fact{{Content: "should not appear", Kind: KindGeneral}}}
	sweeper := NewSweeper(store, distiller, sessions)

	// act — no session messages appended
	require.NoError(t, sweeper.OnEvict(context.Background(), "carol", "sess-1"))

	// assert — no files written
	_, err := os.ReadDir(filepath.Join(storeDir, "user", "carol", "memory"))
	assert.True(t, os.IsNotExist(err))
}

func TestSweeper_PartialSweep_OnlyProcessesNewMessages(t *testing.T) {
	// arrange
	_, store, sessions := newSweeperFixture(t)
	distiller := &countingDistiller{}
	sweeper := NewSweeper(store, distiller, sessions)
	require.NoError(t, sessions.Append("frank", "sess-1", "user", "msg1"))
	require.NoError(t, sessions.Append("frank", "sess-1", "assistant", "resp1"))

	// first sweep — processes 2 messages
	require.NoError(t, sweeper.OnEvict(context.Background(), "frank", "sess-1"))
	firstCount := distiller.lastCount

	// add 1 more message then sweep again
	require.NoError(t, sessions.Append("frank", "sess-1", "user", "msg2"))
	require.NoError(t, sweeper.OnEvict(context.Background(), "frank", "sess-1"))
	secondCount := distiller.lastCount

	assert.Equal(t, 2, firstCount)
	assert.Equal(t, 1, secondCount)
}

func TestSweeper_NoOpWhenCursorAtEOF(t *testing.T) {
	// arrange
	storeDir, store, sessions := newSweeperFixture(t)
	distiller := &stubDistiller{facts: []Fact{{Content: "only once", Kind: KindGeneral}}}
	sweeper := NewSweeper(store, distiller, sessions)
	require.NoError(t, sessions.Append("grace", "sess-1", "user", "msg"))

	// first sweep writes the fact
	require.NoError(t, sweeper.OnEvict(context.Background(), "grace", "sess-1"))

	// act — second sweep with cursor at EOF should be no-op
	require.NoError(t, sweeper.OnEvict(context.Background(), "grace", "sess-1"))

	// assert — fact appears exactly once in general.md
	data, _ := os.ReadFile(filepath.Join(storeDir, "user", "grace", "memory", "general.md"))
	count := strings.Count(string(data), "only once")
	assert.Equal(t, 1, count)
}
