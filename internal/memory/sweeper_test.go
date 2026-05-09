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

func (s *stubDistiller) Distill(_ context.Context, _ []*schema.Message) ([]Fact, error) {
	return s.facts, nil
}

func (s *stubDistiller) Summarize(_ context.Context, _ []Fact) (string, error) {
	return s.summary, nil
}

type countingDistiller struct {
	lastCount int
}

func (d *countingDistiller) Distill(_ context.Context, msgs []*schema.Message) ([]Fact, error) {
	d.lastCount = len(msgs)
	return nil, nil
}

func (d *countingDistiller) Summarize(_ context.Context, _ []Fact) (string, error) {
	return "", nil
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
	require.NoError(t, sweeper.OnDestroy(context.Background(), "alice", "sess-1", nil))

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
	require.NoError(t, sweeper.OnDestroy(context.Background(), "bob", "sess-1", nil))

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
	require.NoError(t, sweeper.OnDestroy(context.Background(), "eve", "sess-1", nil))

	// assert — cursor moved to 2 (both lines processed)
	cursor, err := sessions.ReadCursor("eve", "sess-1")
	require.NoError(t, err)
	assert.Equal(t, 2, cursor)
}

func TestSweeper_DailyFactDateTaggedByLLM(t *testing.T) {
	// arrange — distiller returns a fact with a specific date tag
	storeDir, store, sessions := newSweeperFixture(t)
	distiller := &stubDistiller{
		facts:   []Fact{{Content: "user watched a show", Kind: KindDaily, Date: "2026-04-18"}},
		summary: "Watched a show.",
	}
	sweeper := NewSweeper(store, distiller, sessions)
	require.NoError(t, sessions.Append("alice", "sess-1", "user", "msg"))

	// act
	require.NoError(t, sweeper.OnDestroy(context.Background(), "alice", "sess-1", nil))

	// assert — written to daily/2026-04-18.md (LLM-supplied date), not today
	_, err := os.Stat(filepath.Join(storeDir, "user", "alice", "memory", "daily", "2026-04-18.md"))
	require.NoError(t, err)
}

func TestSweeper_SkipsEmptyHistory(t *testing.T) {
	// arrange
	storeDir, store, sessions := newSweeperFixture(t)
	distiller := &stubDistiller{facts: []Fact{{Content: "should not appear", Kind: KindGeneral}}}
	sweeper := NewSweeper(store, distiller, sessions)

	// act — no session messages appended
	require.NoError(t, sweeper.OnDestroy(context.Background(), "carol", "sess-1", nil))

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
	require.NoError(t, sweeper.OnDestroy(context.Background(), "frank", "sess-1", nil))
	firstCount := distiller.lastCount

	// add 1 more message then sweep again
	require.NoError(t, sessions.Append("frank", "sess-1", "user", "msg2"))
	require.NoError(t, sweeper.OnDestroy(context.Background(), "frank", "sess-1", nil))
	secondCount := distiller.lastCount

	// +1 per sweep for the injected date-header message
	assert.Equal(t, 3, firstCount)
	assert.Equal(t, 2, secondCount)
}

func TestSweeper_NoOpWhenCursorAtEOF(t *testing.T) {
	// arrange
	storeDir, store, sessions := newSweeperFixture(t)
	distiller := &stubDistiller{facts: []Fact{{Content: "only once", Kind: KindGeneral}}}
	sweeper := NewSweeper(store, distiller, sessions)
	require.NoError(t, sessions.Append("grace", "sess-1", "user", "msg"))

	// first sweep writes the fact
	require.NoError(t, sweeper.OnDestroy(context.Background(), "grace", "sess-1", nil))

	// act — second sweep with cursor at EOF should be no-op
	require.NoError(t, sweeper.OnDestroy(context.Background(), "grace", "sess-1", nil))

	// assert — fact appears exactly once in general.md
	data, _ := os.ReadFile(filepath.Join(storeDir, "user", "grace", "memory", "general.md"))
	count := strings.Count(string(data), "only once")
	assert.Equal(t, 1, count)
}
