package integration_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	einoagent "github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rolfwessels/template-go-agent/internal/agent"
	"github.com/rolfwessels/template-go-agent/internal/memory"
	"github.com/rolfwessels/template-go-agent/internal/usage"
)

// spyGenerator is a stub LLM that returns a fixed response and records what it received.
type spyGenerator struct {
	mu       sync.Mutex
	response string
	calls    [][]*schema.Message
}

func (g *spyGenerator) Generate(_ context.Context, msgs []*schema.Message, _ ...einoagent.AgentOption) (*schema.Message, error) {
	g.mu.Lock()
	g.calls = append(g.calls, msgs)
	g.mu.Unlock()
	return &schema.Message{Role: schema.Assistant, Content: g.response}, nil
}

func (g *spyGenerator) lastSystemPrompt() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.calls) == 0 {
		return ""
	}
	last := g.calls[len(g.calls)-1]
	if len(last) == 0 {
		return ""
	}
	return last[0].Content
}

func (g *spyGenerator) callCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.calls)
}

// stubDistiller returns a fixed list of facts regardless of input.
type stubDistiller struct {
	facts []memory.Fact
}

func (d *stubDistiller) DistillAndSummarize(_ context.Context, _ string, _ []*schema.Message) ([]memory.Fact, string, error) {
	return d.facts, "", nil
}

// newTestPool creates an AgentPool wired to a temp FileStore, SessionStore, and stub distiller.
// The factory fn is called for each new user session and receives the loaded memory context.
// Optional onSweep callbacks run after the real sweeper finishes.
func newTestPool(t *testing.T, dir string, distillerFacts []memory.Fact, timeout time.Duration, makeGen func() *spyGenerator, onSweep ...func(error)) (*agent.AgentPool, *memory.FileStore) {
	t.Helper()
	fileStore := memory.NewFileStore(dir)
	sessions := memory.NewSessionStore(dir)
	sweeper := memory.NewSweeper(fileStore, &stubDistiller{facts: distillerFacts}, sessions)
	var observer agent.EvictionObserver = sweeper
	if len(onSweep) > 0 {
		observer = agent.EvictionFunc(func(ctx context.Context, userID, sessionID string) error {
			err := sweeper.OnEvict(ctx, userID, sessionID)
			for _, notify := range onSweep {
				notify(err)
			}
			return err
		})
	}

	pool := agent.NewPool(
		func(ctx context.Context, userID, _ string, history []*schema.Message) (*agent.Agent, error) {
			memCtx, _ := fileStore.AllAsContext(ctx, userID)
			return agent.NewWithGenerator(makeGen(), "base-prompt", agent.WithMemoryContext(memCtx), agent.WithInitialHistory(history)), nil
		},
		timeout,
		observer,
		agent.WithRecordHook(func(userID, sessionID, role, content string) {
			_ = sessions.Append(userID, sessionID, role, content)
		}),
		agent.WithSessionProvider(sessions, 20),
		agent.WithSessionCreator(sessions),
	)
	return pool, fileStore
}

func TestIntegration_MultiTurnPreservesContext(t *testing.T) {
	// arrange
	gen := &spyGenerator{response: "ok"}
	pool := agent.NewPool(
		func(_ context.Context, _, _ string, _ []*schema.Message) (*agent.Agent, error) {
			return agent.NewWithGenerator(gen, "sys"), nil
		},
		time.Minute,
		nil,
	)
	ctx := context.Background()

	// act — two turns from the same user
	_, err1 := pool.Send(ctx, "user1", "", "first question")
	_, err2 := pool.Send(ctx, "user1", "", "second question")
	require.NoError(t, err1)
	require.NoError(t, err2)

	// assert — second call receives system + Q1 + A1 + Q2 = 4 messages
	gen.mu.Lock()
	defer gen.mu.Unlock()
	require.Len(t, gen.calls, 2)
	assert.Len(t, gen.calls[1], 4, "second turn should include full history")
}

func TestIntegration_TimeoutTriggersMemorySweep(t *testing.T) {
	// arrange
	dir := t.TempDir()
	ctx := context.Background()
	sweepDone := make(chan error, 1)
	pool, fileStore := newTestPool(t, dir, []memory.Fact{{Content: "remembered fact", Kind: memory.KindGeneral}}, 20*time.Millisecond, func() *spyGenerator {
		return &spyGenerator{response: "ok"}
	}, func(err error) { sweepDone <- err })
	t.Cleanup(func() { require.NoError(t, pool.Shutdown(ctx)) })

	// act — send a message then wait for inactivity timeout
	_, err := pool.Send(ctx, "alice", "", "something memorable")
	require.NoError(t, err)

	select {
	case err := <-sweepDone:
		require.NoError(t, err, "idle eviction sweep failed")
	case <-time.After(5 * time.Second):
		t.Fatal("idle timeout did not complete the memory sweep")
	}

	// assert — at least one file written for alice
	memCtx, err := fileStore.AllAsContext(ctx, "alice")
	require.NoError(t, err)
	assert.NotEmpty(t, memCtx, "memory sweep should have written at least one entry")
}

func TestIntegration_ShutdownTriggersSweepForAllAgents(t *testing.T) {
	// arrange
	dir := t.TempDir()
	ctx := context.Background()
	pool, fileStore := newTestPool(t, dir, []memory.Fact{{Content: "a fact", Kind: memory.KindGeneral}}, time.Minute, func() *spyGenerator {
		return &spyGenerator{response: "ok"}
	})

	// act — two different users, then shutdown
	_, _ = pool.Send(ctx, "user1", "", "hello")
	_, _ = pool.Send(ctx, "user2", "", "hello")
	require.NoError(t, pool.Shutdown(ctx))

	// assert — both users have memory files
	m1, _ := fileStore.AllAsContext(ctx, "user1")
	m2, _ := fileStore.AllAsContext(ctx, "user2")
	assert.NotEmpty(t, m1, "user1 should have memory entries after shutdown")
	assert.NotEmpty(t, m2, "user2 should have memory entries after shutdown")
}

func TestIntegration_MemoryFromPreviousSessionInjectedInNext(t *testing.T) {
	// arrange
	dir := t.TempDir()
	ctx := context.Background()

	var mu sync.Mutex
	var gens []*spyGenerator
	makeGen := func() *spyGenerator {
		g := &spyGenerator{response: "ok"}
		mu.Lock()
		gens = append(gens, g)
		mu.Unlock()
		return g
	}

	// session 1 — facts are distilled on shutdown
	pool1, _ := newTestPool(t, dir, []memory.Fact{{Content: "user's favourite language is Go", Kind: memory.KindGeneral}}, time.Minute, makeGen)
	_, err := pool1.Send(ctx, "bob", "", "I love Go")
	require.NoError(t, err)
	pool1.Shutdown(ctx)

	// session 2 — new pool, same file store dir; factory loads prior memories
	pool2, _ := newTestPool(t, dir, nil, time.Minute, makeGen)
	_, err = pool2.Send(ctx, "bob", "", "what do you know about me?")
	require.NoError(t, err)

	// assert — session 2's system prompt contains the fact from session 1
	mu.Lock()
	require.Len(t, gens, 2, "should have created two agents")
	session2Prompt := gens[1].lastSystemPrompt()
	mu.Unlock()

	assert.True(t,
		strings.Contains(session2Prompt, "user's favourite language is Go"),
		"session 2 system prompt should contain memory from session 1, got: %q", session2Prompt,
	)
}

func TestIntegration_SessionResetClearsHistoryAndPreservesLongTermMemory(t *testing.T) {
	// arrange
	dir := t.TempDir()
	ctx := context.Background()

	var mu sync.Mutex
	var gens []*spyGenerator
	makeGen := func() *spyGenerator {
		g := &spyGenerator{response: "ok"}
		mu.Lock()
		gens = append(gens, g)
		mu.Unlock()
		return g
	}

	pool, _ := newTestPool(t, dir, []memory.Fact{{Content: "user likes cats", Kind: memory.KindGeneral}}, time.Minute, makeGen)

	// act — build up some conversation history
	_, err := pool.Send(ctx, "carol", "", "I love cats")
	require.NoError(t, err)
	_, err = pool.Send(ctx, "carol", "", "cats are the best")
	require.NoError(t, err)

	// simulate new_session tool invocation
	require.NoError(t, pool.Reset(ctx, "carol"))

	// first message in the new session
	_, err = pool.Send(ctx, "carol", "", "hello again")
	require.NoError(t, err)

	// assert — two agents were created: one before reset, one after
	mu.Lock()
	require.Len(t, gens, 2, "should have created two agents (before and after reset)")
	gen2 := gens[1]
	mu.Unlock()

	gen2.mu.Lock()
	calls := gen2.calls
	gen2.mu.Unlock()

	// after reset the agent starts fresh: system + current user message only (no prior history)
	require.Len(t, calls, 1, "second agent should have received exactly one call")
	assert.Len(t, calls[0], 2, "after reset, only system prompt and new user message expected")

	// long-term memory distilled from the swept session should appear in the system prompt
	assert.Contains(t, calls[0][0].Content, "user likes cats",
		"post-reset system prompt should contain memory swept from the previous session")
}

// spyGeneratorWithUsage returns a fixed response with token usage in ResponseMeta.
type spyGeneratorWithUsage struct {
	mu       sync.Mutex
	response string
	calls    int
}

func (g *spyGeneratorWithUsage) Generate(_ context.Context, msgs []*schema.Message, _ ...einoagent.AgentOption) (*schema.Message, error) {
	g.mu.Lock()
	g.calls++
	g.mu.Unlock()
	return &schema.Message{
		Role:    schema.Assistant,
		Content: g.response,
		ResponseMeta: &schema.ResponseMeta{
			Usage: &schema.TokenUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		},
	}, nil
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		if line := scanner.Text(); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func readLedgerLines(t *testing.T, dir, userID string) []map[string]any {
	t.Helper()
	path := filepath.Join(dir, "user", userID, "metrics", "costs.jsonl")
	var records []map[string]any
	for _, line := range readLines(t, path) {
		var r map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &r))
		records = append(records, r)
	}
	return records
}

func TestIntegration_AgentUsageTrackerWritesToCostLedger(t *testing.T) {
	// arrange
	dir := t.TempDir()
	ctx := context.Background()
	counter := usage.NewCounter()
	tracker := usage.NewFileTracker(dir, counter)

	fileStore := memory.NewFileStore(dir)
	sessions := memory.NewSessionStore(dir)
	sweeper := memory.NewSweeper(fileStore, &stubDistiller{}, sessions)

	gen := &spyGeneratorWithUsage{response: "ok"}
	pool := agent.NewPool(
		func(ctx context.Context, userID, _ string, history []*schema.Message) (*agent.Agent, error) {
			sessionID := agent.SessionIDFromContext(ctx)
			return agent.NewWithGenerator(gen, "sys",
				agent.WithUsageTracker(tracker, userID, sessionID),
			), nil
		},
		time.Minute,
		sweeper,
		agent.WithSessionProvider(sessions, 20),
		agent.WithSessionCreator(sessions),
	)

	// act
	_, err := pool.Send(ctx, "alice", "", "hello")
	require.NoError(t, err)

	// assert
	records := readLedgerLines(t, dir, "alice")
	require.NotEmpty(t, records, "cost ledger should have at least one entry")
	assert.Equal(t, "agent", records[0]["component"])
	assert.Greater(t, records[0]["prompt_tokens"], float64(0))
}

func TestIntegration_DistillerUsageTrackerWritesToCostLedger(t *testing.T) {
	// arrange
	dir := t.TempDir()
	ctx := context.Background()
	counter := usage.NewCounter()
	tracker := usage.NewFileTracker(dir, counter)

	fileStore := memory.NewFileStore(dir)
	sessions := memory.NewSessionStore(dir)

	trackingDistiller := &trackableDistiller{
		delegate: &stubDistiller{facts: []memory.Fact{{Content: "a fact", Kind: memory.KindGeneral}}},
		tracker:  tracker,
	}
	sweeper := memory.NewSweeper(fileStore, trackingDistiller, sessions, memory.WithSweeperTracker(tracker))

	pool := agent.NewPool(
		func(ctx context.Context, userID, _ string, history []*schema.Message) (*agent.Agent, error) {
			return agent.NewWithGenerator(&spyGenerator{response: "ok"}, "sys"), nil
		},
		time.Minute,
		sweeper,
		agent.WithSessionProvider(sessions, 20),
		agent.WithSessionCreator(sessions),
		agent.WithRecordHook(func(userID, sessionID, role, content string) {
			_ = sessions.Append(userID, sessionID, role, content)
		}),
	)

	// act — send a message then reset to trigger sweep
	_, err := pool.Send(ctx, "bob", "", "hello")
	require.NoError(t, err)
	require.NoError(t, pool.Reset(ctx, "bob"))

	// assert
	records := readLedgerLines(t, dir, "bob")
	var distillerRecords []map[string]any
	for _, r := range records {
		if r["component"] == "distiller" {
			distillerRecords = append(distillerRecords, r)
		}
	}
	require.NotEmpty(t, distillerRecords, "cost ledger should have at least one distiller entry")
}

type trackableDistiller struct {
	delegate memory.Distiller
	tracker  usage.Tracker
}

func (d *trackableDistiller) DistillAndSummarize(ctx context.Context, existingDaily string, msgs []*schema.Message) ([]memory.Fact, string, error) {
	facts, summary, err := d.delegate.DistillAndSummarize(ctx, existingDaily, msgs)
	if err == nil {
		userID, sessionID := usage.FromContext(ctx)
		d.tracker.Record(userID, sessionID, "distiller", "test-model", usage.TokenUsage{
			PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15,
		})
	}
	return facts, summary, err
}
