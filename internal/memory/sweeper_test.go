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
	facts []string
}

func (s *stubDistiller) Distill(_ context.Context, _ []*schema.Message) ([]string, error) {
	return s.facts, nil
}

type countingDistiller struct {
	lastCount int
}

func (d *countingDistiller) Distill(_ context.Context, msgs []*schema.Message) ([]string, error) {
	d.lastCount = len(msgs)
	return nil, nil
}

func newSweeperFixture(t *testing.T) (storeDir string, store *FileStore, sessions *SessionStore) {
	t.Helper()
	base := t.TempDir()
	storeDir = filepath.Join(base, "store")
	return storeDir, NewFileStore(storeDir), NewSessionStore(filepath.Join(base, "sessions"))
}

func TestSweeper_WritesMarkdownFilesOnDestroy(t *testing.T) {
	// arrange
	storeDir, store, sessions := newSweeperFixture(t)
	distiller := &stubDistiller{facts: []string{"user prefers brevity", "user works in Go"}}
	sweeper := NewSweeper(store, distiller, sessions)
	require.NoError(t, sessions.Append("alice", "sess-1", "user", "hello"))
	require.NoError(t, sessions.Append("alice", "sess-1", "assistant", "hi"))
	date := time.Now().UTC().Format("2006-01-02")

	// act
	require.NoError(t, sweeper.OnDestroy(context.Background(), "alice", "sess-1", nil))

	// assert
	entries, err := os.ReadDir(filepath.Join(storeDir, "user", "alice", "memory"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, date+".md", entries[0].Name())
}

func TestSweeper_SkipsEmptyHistory(t *testing.T) {
	// arrange
	storeDir, store, sessions := newSweeperFixture(t)
	distiller := &stubDistiller{facts: []string{"should not appear"}}
	sweeper := NewSweeper(store, distiller, sessions)

	// act — no session messages appended
	require.NoError(t, sweeper.OnDestroy(context.Background(), "carol", "sess-1", nil))

	// assert — no files written
	_, err := os.ReadDir(filepath.Join(storeDir, "user", "carol", "memory"))
	assert.True(t, os.IsNotExist(err))
}

func TestSweeper_AdvancesCursorAfterSweep(t *testing.T) {
	// arrange
	_, store, sessions := newSweeperFixture(t)
	distiller := &stubDistiller{facts: []string{"a fact"}}
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

	assert.Equal(t, 2, firstCount)
	assert.Equal(t, 1, secondCount)
}

func TestSweeper_NoOpWhenCursorAtEOF(t *testing.T) {
	// arrange
	storeDir, store, sessions := newSweeperFixture(t)
	distiller := &stubDistiller{facts: []string{"only once"}}
	sweeper := NewSweeper(store, distiller, sessions)
	require.NoError(t, sessions.Append("grace", "sess-1", "user", "msg"))

	// first sweep writes the fact
	require.NoError(t, sweeper.OnDestroy(context.Background(), "grace", "sess-1", nil))

	// act — second sweep with cursor at EOF should be no-op
	require.NoError(t, sweeper.OnDestroy(context.Background(), "grace", "sess-1", nil))

	// assert — fact appears exactly once in the store file
	entries, err := os.ReadDir(filepath.Join(storeDir, "user", "grace", "memory"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	data, _ := os.ReadFile(filepath.Join(storeDir, "user", "grace", "memory", entries[0].Name()))
	count := strings.Count(string(data), "only once")
	assert.Equal(t, 1, count)
}
