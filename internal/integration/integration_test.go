package integration_test

import (
	"context"
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
	facts []string
}

func (d *stubDistiller) Distill(_ context.Context, _ []*schema.Message) ([]string, error) {
	return d.facts, nil
}

// newTestPool creates an AgentPool wired to a temp FileStore, SessionStore, and stub distiller.
// The factory fn is called for each new user session and receives the loaded memory context.
func newTestPool(t *testing.T, dir string, distillerFacts []string, timeout time.Duration, makeGen func() *spyGenerator) (*agent.AgentPool, *memory.FileStore) {
	t.Helper()
	fileStore := memory.NewFileStore(dir)
	sessions := memory.NewSessionStore(dir)
	sweeper := memory.NewSweeper(fileStore, nil, &stubDistiller{facts: distillerFacts}, sessions)

	pool := agent.NewPool(
		func(ctx context.Context, userID string, history []*schema.Message) (*agent.Agent, error) {
			memCtx, _ := fileStore.AllAsContext(ctx, userID)
			return agent.NewWithGenerator(makeGen(), "base-prompt", agent.WithMemoryContext(memCtx), agent.WithInitialHistory(history)), nil
		},
		timeout,
		sweeper.OnDestroy,
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
		func(_ context.Context, _ string, _ []*schema.Message) (*agent.Agent, error) {
			return agent.NewWithGenerator(gen, "sys"), nil
		},
		time.Minute,
		nil,
	)
	ctx := context.Background()

	// act — two turns from the same user
	_, err1 := pool.Send(ctx, "user1", "first question")
	_, err2 := pool.Send(ctx, "user1", "second question")
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
	pool, fileStore := newTestPool(t, dir, []string{"remembered fact"}, 20*time.Millisecond, func() *spyGenerator {
		return &spyGenerator{response: "ok"}
	})

	// act — send a message then wait for inactivity timeout
	_, err := pool.Send(ctx, "alice", "something memorable")
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// assert — at least one file written for alice
	entries, err := fileStore.All(ctx, "alice")
	require.NoError(t, err)
	assert.NotEmpty(t, entries, "memory sweep should have written at least one entry")
}

func TestIntegration_ShutdownTriggersSweepForAllAgents(t *testing.T) {
	// arrange
	dir := t.TempDir()
	ctx := context.Background()
	pool, fileStore := newTestPool(t, dir, []string{"a fact"}, time.Minute, func() *spyGenerator {
		return &spyGenerator{response: "ok"}
	})

	// act — two different users, then shutdown
	_, _ = pool.Send(ctx, "user1", "hello")
	_, _ = pool.Send(ctx, "user2", "hello")
	require.NoError(t, pool.Shutdown(ctx))

	// assert — both users have memory files
	e1, _ := fileStore.All(ctx, "user1")
	e2, _ := fileStore.All(ctx, "user2")
	assert.NotEmpty(t, e1, "user1 should have memory entries after shutdown")
	assert.NotEmpty(t, e2, "user2 should have memory entries after shutdown")
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
	pool1, _ := newTestPool(t, dir, []string{"user's favourite language is Go"}, time.Minute, makeGen)
	_, err := pool1.Send(ctx, "bob", "I love Go")
	require.NoError(t, err)
	pool1.Shutdown(ctx)

	// session 2 — new pool, same file store dir; factory loads prior memories
	pool2, _ := newTestPool(t, dir, nil, time.Minute, makeGen)
	_, err = pool2.Send(ctx, "bob", "what do you know about me?")
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

	pool, _ := newTestPool(t, dir, []string{"user likes cats"}, time.Minute, makeGen)

	// act — build up some conversation history
	_, err := pool.Send(ctx, "carol", "I love cats")
	require.NoError(t, err)
	_, err = pool.Send(ctx, "carol", "cats are the best")
	require.NoError(t, err)

	// simulate new_session tool invocation
	require.NoError(t, pool.Reset(ctx, "carol"))

	// first message in the new session
	_, err = pool.Send(ctx, "carol", "hello again")
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
