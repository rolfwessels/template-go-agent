package agent

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fire deliberately invokes stopped timers too: Stop cannot prevent an already
// dispatched AfterFunc callback from running.
type manualTimer struct {
	callback func()
	stopped  atomic.Bool
}

func (t *manualTimer) Stop() bool { return !t.stopped.Swap(true) }
func (t *manualTimer) fire()      { t.callback() }

type manualTimers struct {
	mu     sync.Mutex
	timers []*manualTimer
}

func (c *manualTimers) option() func(*AgentPool) {
	return func(p *AgentPool) {
		p.afterFunc = func(_ time.Duration, callback func()) idleTimer {
			t := &manualTimer{callback: callback}
			c.mu.Lock()
			c.timers = append(c.timers, t)
			c.mu.Unlock()
			return t
		}
	}
}

func (c *manualTimers) timer(t *testing.T, index int) *manualTimer {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	require.Greater(t, len(c.timers), index)
	return c.timers[index]
}

func (c *manualTimers) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.timers)
}

// Done is evaluated when the lock selects on cancellation. This barrier lets
// tests know a competing call has reached the lock without using sleeps.
type waitingContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *waitingContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

type blockingGenerator struct {
	entered chan []*schema.Message
	release chan struct{}
	active  atomic.Int32
	overlap atomic.Bool
}

func newBlockingGenerator() *blockingGenerator {
	return &blockingGenerator{entered: make(chan []*schema.Message, 8), release: make(chan struct{}, 8)}
}

func (g *blockingGenerator) generate(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
	if g.active.Add(1) != 1 {
		g.overlap.Store(true)
	}
	defer g.active.Add(-1)
	g.entered <- input
	select {
	case <-g.release:
		return []*schema.Message{schema.AssistantMessage("ok", nil)}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Timeouts only guard against deadlocked tests; ordering uses channels.
func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for test barrier")
		var zero T
		return zero
	}
}

func sendAsync(pool *AgentPool, ctx context.Context, userID, message string) <-chan error {
	done := make(chan error, 1)
	go func() {
		_, err := pool.Send(ctx, userID, "", message)
		done <- err
	}()
	return done
}

func TestAgentPool_SameUserTurnsAreSerialized(t *testing.T) {
	gen := newBlockingGenerator()
	clock := &manualTimers{}
	var records []string
	pool := NewPool(func(context.Context, string, string, []*schema.Message) (*Agent, error) {
		return &Agent{react: gen, systemPrompt: "sys"}, nil
	}, time.Minute, nil, clock.option(), WithRecordHook(func(_, _, role, content string) {
		records = append(records, role+":"+content)
	}))

	first := sendAsync(pool, context.Background(), "user", "first")
	require.Len(t, receive(t, gen.entered), 2)
	ctx := &waitingContext{Context: context.Background(), waiting: make(chan struct{})}
	second := sendAsync(pool, ctx, "user", "second")
	receive(t, ctx.waiting)
	gen.release <- struct{}{}
	require.NoError(t, receive(t, first))
	input := receive(t, gen.entered)
	require.Len(t, input, 4)
	assert.Equal(t, "first", input[1].Content)
	assert.Equal(t, "ok", input[2].Content)
	assert.Equal(t, "second", input[3].Content)
	gen.release <- struct{}{}
	require.NoError(t, receive(t, second))
	assert.False(t, gen.overlap.Load())
	assert.Equal(t, []string{"user:first", "assistant:ok", "user:second", "assistant:ok"}, records)
	require.NoError(t, pool.Shutdown(context.Background()))
}

func TestAgentPool_WaitingTurnRespectsCancellation(t *testing.T) {
	gen := newBlockingGenerator()
	pool := NewPool(func(context.Context, string, string, []*schema.Message) (*Agent, error) {
		return &Agent{react: gen}, nil
	}, time.Minute, nil, (&manualTimers{}).option())
	first := sendAsync(pool, context.Background(), "user", "first")
	receive(t, gen.entered)
	base, cancel := context.WithCancel(context.Background())
	ctx := &waitingContext{Context: base, waiting: make(chan struct{})}
	second := sendAsync(pool, ctx, "user", "canceled")
	receive(t, ctx.waiting)
	cancel()
	require.ErrorIs(t, receive(t, second), context.Canceled)
	assert.EqualValues(t, 1, gen.active.Load())
	gen.release <- struct{}{}
	require.NoError(t, receive(t, first))
	assert.Empty(t, gen.entered)
	require.NoError(t, pool.Shutdown(context.Background()))
}

func TestAgentPool_DifferentUsersRunConcurrently(t *testing.T) {
	gens := map[string]*blockingGenerator{"alice": newBlockingGenerator(), "bob": newBlockingGenerator()}
	pool := NewPool(func(_ context.Context, userID, _ string, _ []*schema.Message) (*Agent, error) {
		return &Agent{react: gens[userID]}, nil
	}, time.Minute, nil, (&manualTimers{}).option())
	alice := sendAsync(pool, context.Background(), "alice", "hello")
	receive(t, gens["alice"].entered)
	bob := sendAsync(pool, context.Background(), "bob", "hello")
	receive(t, gens["bob"].entered)
	assert.EqualValues(t, 1, gens["alice"].active.Load())
	assert.EqualValues(t, 1, gens["bob"].active.Load())
	gens["alice"].release <- struct{}{}
	gens["bob"].release <- struct{}{}
	require.NoError(t, receive(t, alice))
	require.NoError(t, receive(t, bob))
	require.NoError(t, pool.Shutdown(context.Background()))
}

func TestAgentPool_TimerDuringTurnWaitsForRecording(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gen := newBlockingGenerator()
		clock := &manualTimers{}
		var records []string
		var sweeps [][]string
		recording := make(chan struct{})
		finishRecord := make(chan struct{})
		pool := NewPool(func(context.Context, string, string, []*schema.Message) (*Agent, error) {
			return &Agent{react: gen}, nil
		}, time.Minute, EvictionFunc(func(context.Context, string, string) error {
			sweeps = append(sweeps, append([]string(nil), records...))
			return nil
		}), clock.option(), WithRecordHook(func(_, _, role, content string) {
			if role == "assistant" && len(records) == 3 {
				close(recording)
				<-finishRecord
			}
			records = append(records, role+":"+content)
		}))
		first := sendAsync(pool, context.Background(), "user", "first")
		receive(t, gen.entered)
		assert.Zero(t, clock.count(), "a new entry is not idle during its first turn")
		gen.release <- struct{}{}
		require.NoError(t, receive(t, first))
		oldTimer := clock.timer(t, 0)
		second := sendAsync(pool, context.Background(), "user", "second")
		receive(t, gen.entered)
		assert.True(t, oldTimer.stopped.Load())
		fired := make(chan struct{})
		go func() { oldTimer.fire(); close(fired) }()
		synctest.Wait() // the callback has reached the active turn lock
		assert.Empty(t, sweeps)
		gen.release <- struct{}{}
		receive(t, recording)
		synctest.Wait()
		assert.Equal(t, 1, clock.count(), "idle timer must not be armed before recording finishes")
		close(finishRecord)
		require.NoError(t, receive(t, second))
		receive(t, fired)
		assert.Empty(t, sweeps, "the timer invalidated by the active turn must not sweep")
		clock.timer(t, 1).fire()
		require.Len(t, sweeps, 1)
		assert.Equal(t, []string{"user:first", "assistant:ok", "user:second", "assistant:ok"}, sweeps[0])
		require.NoError(t, pool.Shutdown(context.Background()))
	})
}

func TestAgentPool_StaleTimerCannotEvictReplacement(t *testing.T) {
	clock := &manualTimers{}
	var creations, sweeps int
	pool := NewPool(func(context.Context, string, string, []*schema.Message) (*Agent, error) {
		creations++
		return &Agent{react: newFakeGenerator("ok")}, nil
	}, time.Minute, EvictionFunc(func(context.Context, string, string) error {
		sweeps++
		return nil
	}), clock.option())
	_, err := pool.Send(context.Background(), "user", "", "old")
	require.NoError(t, err)
	oldTimer := clock.timer(t, 0)
	require.NoError(t, pool.Reset(context.Background(), "user"))
	_, err = pool.Send(context.Background(), "user", "", "new")
	require.NoError(t, err)
	oldTimer.fire()
	assert.Equal(t, 1, sweeps)
	_, err = pool.Send(context.Background(), "user", "", "still new")
	require.NoError(t, err)
	assert.Equal(t, 2, creations)
	require.NoError(t, pool.Shutdown(context.Background()))
	assert.Equal(t, 2, sweeps)
}

func TestAgentPool_ShutdownWaitsForTurnAndRecording(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gen := newBlockingGenerator()
		clock := &manualTimers{}
		var records []string
		recording := make(chan struct{})
		finishRecord := make(chan struct{})
		swept := make(chan []string, 1)
		pool := NewPool(func(context.Context, string, string, []*schema.Message) (*Agent, error) {
			return &Agent{react: gen}, nil
		}, time.Minute, EvictionFunc(func(context.Context, string, string) error {
			swept <- append([]string(nil), records...)
			return nil
		}), clock.option(), WithRecordHook(func(_, _, role, content string) {
			if role == "assistant" {
				close(recording)
				<-finishRecord
			}
			records = append(records, role+":"+content)
		}))
		turn := sendAsync(pool, context.Background(), "user", "hello")
		receive(t, gen.entered)
		ctx := &waitingContext{Context: context.Background(), waiting: make(chan struct{})}
		waiting := sendAsync(pool, ctx, "user", "queued")
		receive(t, ctx.waiting)
		shutdown := make(chan error, 1)
		go func() { shutdown <- pool.Shutdown(context.Background()) }()
		receive(t, pool.stopping)
		synctest.Wait()
		require.ErrorIs(t, receive(t, waiting), ErrPoolClosed)
		_, err := pool.Send(context.Background(), "other", "", "late")
		require.ErrorIs(t, err, ErrPoolClosed)
		assert.Empty(t, swept)
		gen.release <- struct{}{}
		receive(t, recording)
		synctest.Wait()
		assert.Empty(t, swept)
		assert.Empty(t, shutdown)
		close(finishRecord)
		require.NoError(t, receive(t, turn))
		assert.Equal(t, []string{"user:hello", "assistant:ok"}, receive(t, swept))
		require.NoError(t, receive(t, shutdown))
		require.NoError(t, pool.Shutdown(context.Background()))
		assert.Zero(t, clock.count())
	})
}

func TestAgentPool_CanceledShutdownStillDrainsActiveTurn(t *testing.T) {
	gen := newBlockingGenerator()
	swept := make(chan struct{}, 1)
	pool := NewPool(func(context.Context, string, string, []*schema.Message) (*Agent, error) {
		return &Agent{react: gen}, nil
	}, time.Minute, EvictionFunc(func(context.Context, string, string) error {
		swept <- struct{}{}
		return nil
	}), (&manualTimers{}).option())
	turn := sendAsync(pool, context.Background(), "user", "hello")
	receive(t, gen.entered)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, pool.Shutdown(ctx), context.Canceled)
	assert.Empty(t, swept)
	gen.release <- struct{}{}
	require.NoError(t, receive(t, turn))
	require.NoError(t, pool.Shutdown(context.Background()))
	receive(t, swept)
}

func TestAgentPool_ShutdownReportsSweepErrors(t *testing.T) {
	failure := errors.New("sweep failed")
	pool := NewPool(stubFactory(), time.Minute, EvictionFunc(func(context.Context, string, string) error {
		return failure
	}), (&manualTimers{}).option())
	_, err := pool.Send(context.Background(), "user", "", "hello")
	require.NoError(t, err)
	require.ErrorIs(t, pool.Shutdown(context.Background()), failure)
	require.ErrorIs(t, pool.Shutdown(context.Background()), failure)
}

func TestAgentPool_ResetWaitsForTurnAndSweepBeforeReplacement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gen := newBlockingGenerator()
		clock := &manualTimers{}
		var records []string
		var creations atomic.Int32
		sweeping := make(chan []string, 1)
		finishSweep := make(chan struct{})
		pool := NewPool(func(context.Context, string, string, []*schema.Message) (*Agent, error) {
			if creations.Add(1) == 1 {
				return &Agent{react: gen}, nil
			}
			return &Agent{react: newFakeGenerator("fresh")}, nil
		}, time.Minute, EvictionFunc(func(context.Context, string, string) error {
			sweeping <- append([]string(nil), records...)
			<-finishSweep
			return nil
		}), clock.option(), WithRecordHook(func(_, _, role, content string) {
			records = append(records, role+":"+content)
		}))
		turn := sendAsync(pool, context.Background(), "user", "hello")
		receive(t, gen.entered)
		ctx := &waitingContext{Context: context.Background(), waiting: make(chan struct{})}
		reset := make(chan error, 1)
		go func() { reset <- pool.Reset(ctx, "user") }()
		receive(t, ctx.waiting)
		synctest.Wait()
		assert.Empty(t, sweeping)
		gen.release <- struct{}{}
		require.NoError(t, receive(t, turn))
		assert.Equal(t, []string{"user:hello", "assistant:ok"}, receive(t, sweeping))
		waitingCtx := &waitingContext{Context: context.Background(), waiting: make(chan struct{})}
		next := sendAsync(pool, waitingCtx, "user", "next")
		receive(t, waitingCtx.waiting)
		synctest.Wait()
		assert.EqualValues(t, 1, creations.Load(), "replacement must wait for the sweep")
		close(finishSweep)
		require.NoError(t, receive(t, reset))
		require.NoError(t, receive(t, next))
		assert.EqualValues(t, 2, creations.Load())
		require.NoError(t, pool.Shutdown(context.Background()))
	})
}

type resetGenerator struct {
	reset func(context.Context) error
}

func (g *resetGenerator) generate(ctx context.Context, _ []*schema.Message) ([]*schema.Message, error) {
	if err := g.reset(ctx); err != nil {
		return nil, err
	}
	return []*schema.Message{schema.AssistantMessage("reset requested", nil)}, nil
}

type sessionCreatorFunc func(string) (string, error)

func (f sessionCreatorFunc) NewSession(userID string) (string, error) { return f(userID) }

func TestAgentPool_ToolResetRunsAfterResponseIsRecorded(t *testing.T) {
	clock := &manualTimers{}
	var events []string
	var creations int
	var pool *AgentPool
	pool = NewPool(func(context.Context, string, string, []*schema.Message) (*Agent, error) {
		creations++
		if creations == 1 {
			return &Agent{react: &resetGenerator{reset: func(ctx context.Context) error {
				return pool.Reset(ctx, "user")
			}}}, nil
		}
		return &Agent{react: newFakeGenerator("fresh")}, nil
	}, time.Minute, EvictionFunc(func(context.Context, string, string) error {
		events = append(events, "sweep")
		return nil
	}), clock.option(), WithRecordHook(func(_, _, role, _ string) {
		events = append(events, role)
	}), WithSessionCreator(sessionCreatorFunc(func(string) (string, error) {
		events = append(events, "new session")
		return "new", nil
	})))
	// A timeout guards the regression where a tool waits for its own turn lock.
	require.NoError(t, receive(t, sendAsync(pool, context.Background(), "user", "reset")))
	assert.Equal(t, []string{"user", "assistant", "sweep", "new session"}, events)
	assert.Zero(t, clock.count(), "the reset entry must not be rearmed")
	_, err := pool.Send(context.Background(), "user", "", "hello")
	require.NoError(t, err)
	assert.Equal(t, 2, creations)
	require.NoError(t, pool.Shutdown(context.Background()))
}

func TestAgentPool_ToolResetReportsDeferredFailure(t *testing.T) {
	failure := errors.New("sweep failed")
	var pool *AgentPool
	pool = NewPool(func(context.Context, string, string, []*schema.Message) (*Agent, error) {
		return &Agent{react: &resetGenerator{reset: func(ctx context.Context) error {
			return pool.Reset(ctx, "user")
		}}}, nil
	}, time.Minute, EvictionFunc(func(context.Context, string, string) error {
		return failure
	}), (&manualTimers{}).option())
	require.ErrorIs(t, receive(t, sendAsync(pool, context.Background(), "user", "reset")), failure)
	require.NoError(t, pool.Shutdown(context.Background()))
}

func TestAgentPool_CreationDoesNotBlockOtherUsers(t *testing.T) {
	creating := make(chan struct{})
	finishCreate := make(chan struct{})
	pool := NewPool(func(_ context.Context, userID, _ string, _ []*schema.Message) (*Agent, error) {
		if userID == "alice" {
			close(creating)
			<-finishCreate
		}
		return &Agent{react: newFakeGenerator("ok")}, nil
	}, time.Minute, nil, (&manualTimers{}).option())
	alice := sendAsync(pool, context.Background(), "alice", "hello")
	receive(t, creating)
	require.NoError(t, receive(t, sendAsync(pool, context.Background(), "bob", "hello")))
	close(finishCreate)
	require.NoError(t, receive(t, alice))
	require.NoError(t, pool.Shutdown(context.Background()))
}

func TestAgentPool_ShutdownWaitsForEvictionSweep(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clock := &manualTimers{}
		sweeping := make(chan struct{})
		finishSweep := make(chan struct{})
		var sweeps atomic.Int32
		pool := NewPool(stubFactory(), time.Minute, EvictionFunc(func(context.Context, string, string) error {
			sweeps.Add(1)
			close(sweeping)
			<-finishSweep
			return nil
		}), clock.option())
		_, err := pool.Send(context.Background(), "user", "", "hello")
		require.NoError(t, err)
		fired := make(chan struct{})
		timer := clock.timer(t, 0)
		go func() { timer.fire(); close(fired) }()
		receive(t, sweeping)
		shutdown := make(chan error, 1)
		go func() { shutdown <- pool.Shutdown(context.Background()) }()
		receive(t, pool.stopping)
		synctest.Wait()
		assert.Empty(t, shutdown)
		close(finishSweep)
		receive(t, fired)
		require.NoError(t, receive(t, shutdown))
		assert.EqualValues(t, 1, sweeps.Load(), "shutdown must not repeat an in-progress eviction sweep")
	})
}
