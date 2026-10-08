package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
)

type sessionIDKey struct{}

func SessionIDFromContext(ctx context.Context) string {
	s, _ := ctx.Value(sessionIDKey{}).(string)
	return s
}

type EvictionObserver interface {
	OnEvict(ctx context.Context, userID, sessionID string) error
}

type EvictionFunc func(ctx context.Context, userID, sessionID string) error

func (f EvictionFunc) OnEvict(ctx context.Context, userID, sessionID string) error {
	return f(ctx, userID, sessionID)
}

type RecordHook func(userID, sessionID, role, content string)

type AgentFactory func(ctx context.Context, userID, channelID string, history []*schema.Message) (*Agent, error)

type SessionProvider interface {
	LoadSession(userID string, windowSize int) (sessionID string, history []*schema.Message, err error)
}

type SessionCreator interface {
	NewSession(userID string) (string, error)
}

// turn protects entry state, including creation and sweeping. Shutdown takes
// exclusive ownership only after all admitted operations have finished.
// An entry stays in the map until its sweep has finished, so its replacement
// cannot load history or memory while that sweep is still running.
type poolEntry struct {
	turn      turnLock
	agent     *Agent
	timer     idleTimer
	revision  uint64
	sessionID string
}

type idleTimer interface {
	Stop() bool
}

// ErrPoolClosed is returned when work is submitted after shutdown starts.
var ErrPoolClosed = errors.New("agent pool is shut down")

type turnContextKey struct{}

// A reset requested by a tool must finish after Generate returns, rather than
// waiting for the very turn that is invoking the tool.
type turnContext struct {
	pool   *AgentPool
	userID string
	mu     sync.Mutex
	active bool
	reset  bool
}

type AgentPool struct {
	mu             sync.Mutex
	agents         map[string]*poolEntry
	factory        AgentFactory
	timeout        time.Duration
	observer       EvictionObserver
	recorder       RecordHook
	sessions       SessionProvider
	sessionCreator SessionCreator
	windowSize     int
	afterFunc      func(time.Duration, func()) idleTimer
	operations     sync.WaitGroup
	closed         bool
	stopping       chan struct{}
	shutdownDone   chan struct{}
	shutdownErr    error
}

func WithRecordHook(h RecordHook) func(*AgentPool) {
	return func(p *AgentPool) { p.recorder = h }
}

func WithSessionProvider(sp SessionProvider, windowSize int) func(*AgentPool) {
	return func(p *AgentPool) {
		p.sessions = sp
		p.windowSize = windowSize
	}
}

func WithSessionCreator(sc SessionCreator) func(*AgentPool) {
	return func(p *AgentPool) { p.sessionCreator = sc }
}

func NewPool(factory AgentFactory, timeout time.Duration, observer EvictionObserver, opts ...func(*AgentPool)) *AgentPool {
	p := &AgentPool{
		agents:       make(map[string]*poolEntry),
		factory:      factory,
		timeout:      timeout,
		observer:     observer,
		afterFunc:    func(d time.Duration, f func()) idleTimer { return time.AfterFunc(d, f) },
		stopping:     make(chan struct{}),
		shutdownDone: make(chan struct{}),
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// begin admits work under the same mutex that closes the pool. No operation
// can be added to the wait group after shutdown starts waiting.
func (p *AgentPool) begin() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return ErrPoolClosed
	}
	p.operations.Add(1)
	return nil
}

func (p *AgentPool) Send(ctx context.Context, userID, channelID, message string) (string, error) {
	if err := p.begin(); err != nil {
		return "", err
	}
	defer p.operations.Done()

	e, err := p.lockEntry(ctx, userID, true)
	if err != nil {
		return "", err
	}
	defer e.turn.unlock()
	p.stopTimer(e)
	if e.agent == nil {
		sessionID, history, err := p.resolveSession(userID)
		if err != nil {
			p.remove(userID, e)
			return "", err
		}
		a, err := p.factory(context.WithValue(ctx, sessionIDKey{}, sessionID), userID, channelID, history)
		if err != nil {
			p.remove(userID, e)
			return "", err
		}
		e.agent, e.sessionID = a, sessionID
		slog.Info("agent session created", "userID", userID, "sessionID", sessionID)
	}

	turn := &turnContext{pool: p, userID: userID, active: true}
	turnCtx := context.WithValue(ctx, turnContextKey{}, turn)
	slog.Info("agent send", "userID", userID, "sessionID", e.sessionID)
	p.record(userID, e.sessionID, "user", message)
	resp, generateErr := e.agent.Generate(turnCtx, message)
	if generateErr == nil {
		slog.Info("agent response", "userID", userID, "sessionID", e.sessionID)
		p.record(userID, e.sessionID, "assistant", resp)
	}
	turn.mu.Lock()
	turn.active = false
	reset := turn.reset
	turn.mu.Unlock()
	if reset {
		return resp, errors.Join(generateErr, p.resetEntry(ctx, userID, e))
	}
	// Recording belongs to the turn: idle time starts only after it completes.
	p.armTimer(userID, e)
	return resp, generateErr
}

// Shutdown stops admission and waits for active turns and sweeps before the
// final sweep. A canceled context releases the caller; cleanup still waits for
// active work, and a subsequent Shutdown can wait for its completion.
func (p *AgentPool) Shutdown(ctx context.Context) error {
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		close(p.stopping)
		go p.finishShutdown(ctx)
	}
	p.mu.Unlock()
	select {
	case <-p.shutdownDone:
		return p.shutdownErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *AgentPool) finishShutdown(ctx context.Context) {
	p.operations.Wait()
	p.mu.Lock()
	entries := p.agents
	p.agents = make(map[string]*poolEntry)
	p.mu.Unlock()

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for userID, entry := range entries {
		wg.Add(1)
		go func(uid string, e *poolEntry) {
			defer wg.Done()
			p.stopTimer(e)
			if e.agent == nil {
				return
			}
			if err := p.callObserver(ctx, uid, e.sessionID); err != nil {
				slog.Error("memory sweep failed on shutdown", "userID", uid, "err", err)
				mu.Lock()
				errs = append(errs, fmt.Errorf("sweep for %s: %w", uid, err))
				mu.Unlock()
			}
		}(userID, entry)
	}
	wg.Wait()
	p.shutdownErr = errors.Join(errs...)
	close(p.shutdownDone)
}

func (p *AgentPool) Reset(ctx context.Context, userID string) error {
	if turn, ok := ctx.Value(turnContextKey{}).(*turnContext); ok && turn.pool == p && turn.userID == userID {
		turn.mu.Lock()
		if turn.active {
			turn.reset = true
			turn.mu.Unlock()
			return nil
		}
		turn.mu.Unlock()
	}
	if err := p.begin(); err != nil {
		return err
	}
	defer p.operations.Done()
	e, err := p.lockEntry(ctx, userID, false)
	if err != nil || e == nil {
		return err
	}
	defer e.turn.unlock()
	return p.resetEntry(ctx, userID, e)
}

// resetEntry runs with the entry locked, including the observer and session
// creation. Waiting sends must retry with a new entry only after it finishes.
func (p *AgentPool) resetEntry(ctx context.Context, userID string, e *poolEntry) error {
	p.stopTimer(e)
	defer p.remove(userID, e)
	if e.agent == nil {
		return nil
	}
	slog.Info("session reset started", "userID", userID, "sessionID", e.sessionID)
	if err := p.callObserver(ctx, userID, e.sessionID); err != nil {
		return fmt.Errorf("sweeping session on reset: %w", err)
	}
	if p.sessionCreator != nil {
		if _, err := p.sessionCreator.NewSession(userID); err != nil {
			return fmt.Errorf("creating new session: %w", err)
		}
	}
	slog.Info("session reset complete", "userID", userID, "oldSessionID", e.sessionID)
	return nil
}

// lockEntry returns the current entry with its turn lock held. A waiter may
// have seen an entry that was evicted while it waited; in that case it retries.
func (p *AgentPool) lockEntry(ctx context.Context, userID string, create bool) (*poolEntry, error) {
	for {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return nil, ErrPoolClosed
		}
		e := p.agents[userID]
		if e == nil && create {
			e = &poolEntry{}
			// Acquire before publication so cancellation cannot leave an unused
			// placeholder in the pool. A new entry's lock cannot be contended.
			if err := e.turn.lock(ctx, nil); err != nil {
				p.mu.Unlock()
				return nil, err
			}
			p.agents[userID] = e
			p.mu.Unlock()
			return e, nil
		}
		p.mu.Unlock()
		if e == nil {
			return nil, nil
		}
		if err := e.turn.lock(ctx, p.stopping); err != nil {
			return nil, err
		}
		p.mu.Lock()
		current, closed := p.agents[userID] == e, p.closed
		p.mu.Unlock()
		if closed {
			e.turn.unlock()
			return nil, ErrPoolClosed
		}
		if current {
			return e, nil
		}
		e.turn.unlock()
	}
}

func (p *AgentPool) remove(userID string, e *poolEntry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.agents[userID] == e {
		delete(p.agents, userID)
	}
}

func (p *AgentPool) resolveSession(userID string) (string, []*schema.Message, error) {
	if p.sessions == nil {
		return fmt.Sprintf("%019d", time.Now().UnixNano()), nil, nil
	}
	sessionID, history, err := p.sessions.LoadSession(userID, p.windowSize)
	if err != nil {
		return "", nil, fmt.Errorf("loading session: %w", err)
	}
	return sessionID, history, nil
}

// stopTimer invalidates callbacks that Stop could not prevent from starting.
// The revision also protects a newly armed timer on the same entry.
func (p *AgentPool) stopTimer(e *poolEntry) {
	e.revision++
	if e.timer != nil {
		e.timer.Stop()
		e.timer = nil
	}
}

func (p *AgentPool) armTimer(userID string, e *poolEntry) {
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return
	}
	revision := e.revision
	e.timer = p.afterFunc(p.timeout, func() {
		p.destroy(context.Background(), userID, e, revision)
	})
}

func (p *AgentPool) destroy(ctx context.Context, userID string, e *poolEntry, revision uint64) {
	if err := p.begin(); err != nil {
		return
	}
	defer p.operations.Done()
	if err := e.turn.lock(ctx, p.stopping); err != nil {
		return
	}
	defer e.turn.unlock()
	p.mu.Lock()
	current, closed := p.agents[userID] == e, p.closed
	p.mu.Unlock()
	if !current || closed || e.revision != revision {
		return
	}
	p.stopTimer(e)
	defer p.remove(userID, e)
	slog.Info("agent session destroyed", "userID", userID, "sessionID", e.sessionID)
	if err := p.callObserver(ctx, userID, e.sessionID); err != nil {
		slog.Error("memory sweep failed on eviction", "userID", userID, "err", err)
	}
}

func (p *AgentPool) callObserver(ctx context.Context, userID, sessionID string) error {
	if p.observer != nil {
		return p.observer.OnEvict(ctx, userID, sessionID)
	}
	return nil
}

func (p *AgentPool) record(userID, sessionID, role, content string) {
	if p.recorder != nil {
		p.recorder(userID, sessionID, role, content)
	}
}
