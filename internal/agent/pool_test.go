package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSessionProvider struct {
	sessionID string
	messages  []*schema.Message
}

func (f *fakeSessionProvider) LoadSession(_ string, _ int) (string, []*schema.Message, error) {
	return f.sessionID, f.messages, nil
}

func stubFactory() AgentFactory {
	return func(_ context.Context, _, _ string, _ []*schema.Message) (*Agent, error) {
		return &Agent{react: newFakeGenerator("ok"), systemPrompt: "sys"}, nil
	}
}

func TestAgentPool_IsolatedHistories(t *testing.T) {
	// arrange
	var (
		mu      sync.Mutex
		created []*Agent
	)
	factory := func(_ context.Context, _, _ string, _ []*schema.Message) (*Agent, error) {
		a := &Agent{react: newFakeGenerator("ok"), systemPrompt: "sys"}
		mu.Lock()
		created = append(created, a)
		mu.Unlock()
		return a, nil
	}
	pool := NewPool(factory, time.Minute, nil)
	ctx := context.Background()

	// act
	_, err1 := pool.Send(ctx, "user1", "", "first")
	_, err2 := pool.Send(ctx, "user2", "", "first")
	_, err3 := pool.Send(ctx, "user1", "", "second")
	require.NoError(t, err1)
	require.NoError(t, err2)
	require.NoError(t, err3)

	// assert — user1 has 2 exchanges (4 messages), user2 has 1 (2 messages)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, created, 2)
	assert.Len(t, created[0].history, 4)
	assert.Len(t, created[1].history, 2)
}

func TestAgentPool_TimeoutCreatesNewAgent(t *testing.T) {
	// arrange
	var count int
	factory := func(_ context.Context, _, _ string, _ []*schema.Message) (*Agent, error) {
		count++
		return &Agent{react: newFakeGenerator("ok"), systemPrompt: "sys"}, nil
	}
	clock := &manualTimers{}
	pool := NewPool(factory, time.Minute, nil, clock.option())
	ctx := context.Background()

	// act — first message creates agent
	_, err := pool.Send(ctx, "user1", "", "hello")
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	clock.timer(t, 0).fire()

	// second message after timeout creates a fresh agent
	_, err = pool.Send(ctx, "user1", "", "hello again")
	require.NoError(t, err)

	// assert
	assert.Equal(t, 2, count)
}

func TestAgentPool_DestroyHookCalledOnShutdown(t *testing.T) {
	// arrange
	var (
		mu        sync.Mutex
		hookCalls []string
	)
	hook := EvictionFunc(func(_ context.Context, userID, _ string) error {
		mu.Lock()
		hookCalls = append(hookCalls, userID)
		mu.Unlock()
		return nil
	})
	pool := NewPool(stubFactory(), time.Minute, hook)
	ctx := context.Background()

	// act
	_, _ = pool.Send(ctx, "user1", "", "hello")
	_, _ = pool.Send(ctx, "user2", "", "hello")
	require.NoError(t, pool.Shutdown(ctx))

	// assert
	mu.Lock()
	defer mu.Unlock()
	assert.ElementsMatch(t, []string{"user1", "user2"}, hookCalls)
}

func TestAgentPool_DestroyHookCalledOnTimeout(t *testing.T) {
	// arrange
	called := make(chan string, 1)
	hook := EvictionFunc(func(_ context.Context, userID, _ string) error {
		called <- userID
		return nil
	})
	clock := &manualTimers{}
	pool := NewPool(stubFactory(), time.Minute, hook, clock.option())
	ctx := context.Background()

	// act
	_, err := pool.Send(ctx, "user1", "", "hello")
	require.NoError(t, err)

	// assert
	clock.timer(t, 0).fire()
	assert.Equal(t, "user1", <-called)
}

func TestAgentPool_SessionIDStableAcrossEviction(t *testing.T) {
	// arrange
	sp := &fakeSessionProvider{sessionID: "fixed-session"}
	var (
		mu         sync.Mutex
		sessionIDs []string
	)
	clock := &manualTimers{}
	pool := NewPool(stubFactory(), time.Minute, nil, clock.option(),
		WithSessionProvider(sp, 20),
		WithRecordHook(func(_, sessionID, _, _ string) {
			mu.Lock()
			sessionIDs = append(sessionIDs, sessionID)
			mu.Unlock()
		}))
	ctx := context.Background()

	// act
	_, err := pool.Send(ctx, "user1", "", "hello")
	require.NoError(t, err)
	clock.timer(t, 0).fire() // evict before the next send
	_, err = pool.Send(ctx, "user1", "", "hello again")
	require.NoError(t, err)

	// assert — both sends use the same session ID from the provider
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, sessionIDs, 4) // user + assistant, twice
	assert.Equal(t, "fixed-session", sessionIDs[0])
	assert.Equal(t, "fixed-session", sessionIDs[2])
}

func TestAgentPool_RecreatedAgentLoadsHistory(t *testing.T) {
	// arrange
	history := []*schema.Message{
		schema.UserMessage("prev question"),
		{Role: schema.Assistant, Content: "prev answer"},
	}
	sp := &fakeSessionProvider{sessionID: "sess", messages: history}
	var (
		mu           sync.Mutex
		factoryCount int
		capturedHist []*schema.Message
	)
	factory := func(_ context.Context, _, _ string, h []*schema.Message) (*Agent, error) {
		mu.Lock()
		factoryCount++
		capturedHist = h
		mu.Unlock()
		return &Agent{react: newFakeGenerator("ok"), systemPrompt: "sys"}, nil
	}
	clock := &manualTimers{}
	pool := NewPool(factory, time.Minute, nil, clock.option(), WithSessionProvider(sp, 20))
	ctx := context.Background()

	// act
	_, _ = pool.Send(ctx, "user1", "", "hello")
	clock.timer(t, 0).fire() // evict before the next send
	_, _ = pool.Send(ctx, "user1", "", "hello again")

	// assert — factory called twice and second call received history from provider
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 2, factoryCount)
	assert.Equal(t, history, capturedHist)
}
