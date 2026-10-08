package memory

import (
	"context"
	"errors"
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
	err     error
	calls   int
	msgs    []*schema.Message
}

func (s *stubDistiller) DistillAndSummarize(_ context.Context, _ string, msgs []*schema.Message) ([]Fact, string, error) {
	s.calls++
	s.msgs = msgs
	return s.facts, s.summary, s.err
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

// failingSweepStore injects failures while using real file persistence, so retry
// assertions exercise FileStore's deduplication and index rebuilding.
type failingSweepStore struct {
	*FileStore
	readErr         error
	saveErr         error
	failSaveAt      int
	saveCalls       int
	indexErr        error
	indexCalls      int
	indexWriteFirst bool
}

func (s *failingSweepStore) ReadDailyFile(userID, date string) (string, error) {
	if s.readErr != nil {
		return "", s.readErr
	}
	return s.FileStore.ReadDailyFile(userID, date)
}

func (s *failingSweepStore) Save(ctx context.Context, userID, date string, fact Fact) error {
	s.saveCalls++
	if s.saveCalls == s.failSaveAt {
		return s.saveErr
	}
	return s.FileStore.Save(ctx, userID, date, fact)
}

func (s *failingSweepStore) UpdateIndex(userID string, summaries map[string]string) error {
	s.indexCalls++
	if s.indexWriteFirst || s.indexErr == nil {
		if err := s.FileStore.UpdateIndex(userID, summaries); err != nil {
			return err
		}
	}
	return s.indexErr
}

func writeSweepHistory(t *testing.T, sessions *SessionStore) {
	t.Helper()
	require.NoError(t, os.MkdirAll(sessions.sessDir("alice"), 0750))
	// The cursor already covers the first line. Only lines (1, 3] are swept.
	data := `{"timestamp":"2026-04-17T08:00:00Z","role":"user","content":"already swept"}
{"timestamp":"2026-04-18T08:00:00Z","role":"user","content":"new message"}
{"timestamp":"2026-04-18T09:00:00Z","role":"assistant","content":"new reply"}
`
	require.NoError(t, os.WriteFile(sessions.sessionPath("alice", "sess-1"), []byte(data), 0600))
	require.NoError(t, sessions.WriteCursor("alice", "sess-1", 1))
}

func requireSweepCursor(t *testing.T, sessions *SessionStore, want int) {
	t.Helper()
	cursor, err := sessions.ReadCursor("alice", "sess-1")
	require.NoError(t, err)
	require.Equal(t, want, cursor)
}

func TestSweeper_FailuresLeaveCursorAndRetryWithoutDuplicates(t *testing.T) {
	for _, failure := range []string{"daily read", "distiller", "second fact", "index before write", "index after write"} {
		t.Run(failure, func(t *testing.T) {
			_, files, sessions := newSweeperFixture(t)
			writeSweepHistory(t, sessions)
			store := &failingSweepStore{FileStore: files}
			distiller := &stubDistiller{
				facts: []Fact{
					{Content: "general fact", Kind: KindGeneral},
					{Content: "daily fact", Kind: KindDaily},
				},
				summary: "A summary.",
			}
			injected := errors.New("injected failure")
			switch failure {
			case "daily read":
				store.readErr = injected
			case "distiller":
				distiller.err = injected
			case "second fact":
				store.failSaveAt, store.saveErr = 2, injected
			case "index before write", "index after write":
				store.indexErr = injected
				store.indexWriteFirst = failure == "index after write"
			}
			sweeper := NewSweeper(store, distiller, sessions)

			err := sweeper.OnEvict(context.Background(), "alice", "sess-1")
			require.ErrorIs(t, err, injected)
			requireSweepCursor(t, sessions, 1)
			if failure == "daily read" || failure == "distiller" {
				assert.Zero(t, store.saveCalls)
			}
			if failure == "second fact" {
				daily, err := files.ReadDailyFile("alice", "2026-04-18")
				require.NoError(t, err)
				assert.Contains(t, daily, "- general fact\n")
				assert.NotContains(t, daily, "- daily fact\n")
			}
			if !strings.HasPrefix(failure, "index") {
				assert.Zero(t, store.indexCalls)
			}

			store.readErr, store.saveErr, store.indexErr, distiller.err = nil, nil, nil, nil
			store.failSaveAt = 0
			// A fresh sweeper and FileStore prove retry identity is persisted on
			// disk rather than relying on an in-memory record of successful facts.
			store.FileStore = NewFileStore(files.dir)
			sweeper = NewSweeper(store, distiller, sessions)
			require.NoError(t, sweeper.OnEvict(context.Background(), "alice", "sess-1"))
			requireSweepCursor(t, sessions, 3)
			require.Len(t, distiller.msgs, 2)
			assert.Equal(t, "new message", distiller.msgs[0].Content)
			assert.Equal(t, "new reply", distiller.msgs[1].Content)

			daily, err := files.ReadDailyFile("alice", "2026-04-18")
			require.NoError(t, err)
			assert.Equal(t, 1, strings.Count(daily, "- general fact\n"))
			assert.Equal(t, 1, strings.Count(daily, "- daily fact\n"))
			general, err := os.ReadFile(filepath.Join(files.MemoryDir("alice"), "general.md"))
			require.NoError(t, err)
			assert.Equal(t, 1, strings.Count(string(general), "- general fact\n"))
			index, err := os.ReadFile(filepath.Join(files.MemoryDir("alice"), "MEMORY.md"))
			require.NoError(t, err)
			assert.Equal(t, 1, strings.Count(string(index), "- [general.md](general.md)"))
			assert.Equal(t, 1, strings.Count(string(index), "- [daily/2026-04-18.md](daily/2026-04-18.md)"))
			assert.Contains(t, string(index), "A summary.")

			calls := distiller.calls
			require.NoError(t, sweeper.OnEvict(context.Background(), "alice", "sess-1"))
			assert.Equal(t, calls, distiller.calls, "committed messages must not be redistilled")
		})
	}
}

type distillerFunc func(context.Context, string, []*schema.Message) ([]Fact, string, error)

func (f distillerFunc) DistillAndSummarize(ctx context.Context, existing string, msgs []*schema.Message) ([]Fact, string, error) {
	return f(ctx, existing, msgs)
}

func TestSweeper_LaterDateFailureRetriesEntireRange(t *testing.T) {
	_, store, sessions := newSweeperFixture(t)
	writeSweepHistory(t, sessions)
	f, err := os.OpenFile(sessions.sessionPath("alice", "sess-1"), os.O_APPEND|os.O_WRONLY, 0600)
	require.NoError(t, err)
	_, err = f.WriteString(`{"timestamp":"2026-04-19T08:00:00Z","role":"user","content":"later date"}` + "\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	injected := errors.New("later date distillation failed")
	fail := true
	var received []string
	distiller := distillerFunc(func(_ context.Context, _ string, msgs []*schema.Message) ([]Fact, string, error) {
		received = append(received, msgs[0].Content)
		if fail && msgs[0].Content == "later date" {
			return nil, "", injected
		}
		return []Fact{{Content: msgs[0].Content, Kind: KindDaily}}, "", nil
	})
	sweeper := NewSweeper(store, distiller, sessions)
	require.ErrorIs(t, sweeper.OnEvict(context.Background(), "alice", "sess-1"), injected)
	requireSweepCursor(t, sessions, 1)
	fail = false
	require.NoError(t, sweeper.OnEvict(context.Background(), "alice", "sess-1"))
	requireSweepCursor(t, sessions, 4)
	assert.Equal(t, []string{"new message", "later date", "new message", "later date"}, received)
	for _, date := range []string{"2026-04-18", "2026-04-19"} {
		daily, err := store.ReadDailyFile("alice", date)
		require.NoError(t, err)
		assert.Equal(t, 1, strings.Count(daily, "\n- "))
	}
}

func TestSweeper_CommitsOnlyReadRange(t *testing.T) {
	_, store, sessions := newSweeperFixture(t)
	writeSweepHistory(t, sessions)
	distiller := distillerFunc(func(_ context.Context, _ string, _ []*schema.Message) ([]Fact, string, error) {
		// New messages arriving during distillation belong to the next batch.
		require.NoError(t, sessions.Append("alice", "sess-1", "user", "arrived during sweep"))
		return nil, "", nil
	})
	sweeper := NewSweeper(store, distiller, sessions)
	require.NoError(t, sweeper.OnEvict(context.Background(), "alice", "sess-1"))
	requireSweepCursor(t, sessions, 3)
	msgs, total, err := sessions.ReadFrom("alice", "sess-1", 3)
	require.NoError(t, err)
	assert.Equal(t, 4, total)
	require.Len(t, msgs, 1)
	assert.Equal(t, "arrived during sweep", msgs[0].Content)
}
