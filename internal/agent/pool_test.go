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

func stubFactory() AgentFactory {
	return func(_ context.Context, _ string) (*Agent, error) {
		return &Agent{react: &fakeGenerator{response: "ok"}, systemPrompt: "sys"}, nil
	}
}

func TestAgentPool_IsolatedHistories(t *testing.T) {
	// arrange
	var (
		mu      sync.Mutex
		created []*Agent
	)
	factory := func(_ context.Context, _ string) (*Agent, error) {
		a := &Agent{react: &fakeGenerator{response: "ok"}, systemPrompt: "sys"}
		mu.Lock()
		created = append(created, a)
		mu.Unlock()
		return a, nil
	}
	pool := NewPool(factory, time.Minute, nil)
	ctx := context.Background()

	// act
	_, err1 := pool.Send(ctx, "user1", "first")
	_, err2 := pool.Send(ctx, "user2", "first")
	_, err3 := pool.Send(ctx, "user1", "second")
	require.NoError(t, err1)
	require.NoError(t, err2)
	require.NoError(t, err3)

	// assert — user1 has 2 exchanges (4 messages), user2 has 1 (2 messages)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, created, 2)
	assert.Len(t, created[0].history.all(), 4)
	assert.Len(t, created[1].history.all(), 2)
}

func TestAgentPool_TimeoutCreatesNewAgent(t *testing.T) {
	// arrange
	var count int
	factory := func(_ context.Context, _ string) (*Agent, error) {
		count++
		return &Agent{react: &fakeGenerator{response: "ok"}, systemPrompt: "sys"}, nil
	}
	pool := NewPool(factory, 20*time.Millisecond, nil)
	ctx := context.Background()

	// act — first message creates agent
	_, err := pool.Send(ctx, "user1", "hello")
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	time.Sleep(60 * time.Millisecond)

	// second message after timeout creates a fresh agent
	_, err = pool.Send(ctx, "user1", "hello again")
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
	hook := func(_ context.Context, userID, _ string, _ []*schema.Message) {
		mu.Lock()
		hookCalls = append(hookCalls, userID)
		mu.Unlock()
	}
	pool := NewPool(stubFactory(), time.Minute, hook)
	ctx := context.Background()

	// act
	_, _ = pool.Send(ctx, "user1", "hello")
	_, _ = pool.Send(ctx, "user2", "hello")
	pool.Shutdown(ctx)

	// assert
	mu.Lock()
	defer mu.Unlock()
	assert.ElementsMatch(t, []string{"user1", "user2"}, hookCalls)
}

func TestAgentPool_DestroyHookCalledOnTimeout(t *testing.T) {
	// arrange
	called := make(chan string, 1)
	hook := func(_ context.Context, userID, _ string, _ []*schema.Message) {
		called <- userID
	}
	pool := NewPool(stubFactory(), 20*time.Millisecond, hook)
	ctx := context.Background()

	// act
	_, err := pool.Send(ctx, "user1", "hello")
	require.NoError(t, err)

	// assert
	select {
	case uid := <-called:
		assert.Equal(t, "user1", uid)
	case <-time.After(time.Second):
		t.Fatal("destroy hook was not called within timeout")
	}
}
