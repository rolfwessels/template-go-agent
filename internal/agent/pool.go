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

type DestroyHook func(ctx context.Context, userID, sessionID string, messages []*schema.Message) error
type RecordHook func(userID, sessionID, role, content string)

type AgentFactory func(ctx context.Context, userID string, history []*schema.Message) (*Agent, error)

type SessionProvider interface {
	LoadSession(userID string, windowSize int) (sessionID string, history []*schema.Message, err error)
}

type SessionCreator interface {
	NewSession(userID string) (string, error)
}

type poolEntry struct {
	agent     *Agent
	timer     *time.Timer
	sessionID string
}

type AgentPool struct {
	mu             sync.Mutex
	agents         map[string]*poolEntry
	factory        AgentFactory
	timeout        time.Duration
	hook           DestroyHook
	recorder       RecordHook
	sessions       SessionProvider
	sessionCreator SessionCreator
	windowSize     int
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

func NewPool(factory AgentFactory, timeout time.Duration, hook DestroyHook, opts ...func(*AgentPool)) *AgentPool {
	p := &AgentPool{
		agents:  make(map[string]*poolEntry),
		factory: factory,
		timeout: timeout,
		hook:    hook,
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

func (p *AgentPool) Send(ctx context.Context, userID, message string) (string, error) {
	e, err := p.getOrCreate(ctx, userID)
	if err != nil {
		return "", err
	}
	slog.Info("agent send", "userID", userID, "sessionID", e.sessionID)
	p.record(userID, e.sessionID, "user", message)
	resp, err := e.agent.Generate(ctx, message)
	p.resetTimer(userID)
	if err != nil {
		return "", err
	}
	slog.Info("agent response", "userID", userID, "sessionID", e.sessionID)
	p.record(userID, e.sessionID, "assistant", resp)
	return resp, nil
}

func (p *AgentPool) Shutdown(ctx context.Context) error {
	p.mu.Lock()
	entries := make(map[string]*poolEntry, len(p.agents))
	for id, e := range p.agents {
		entries[id] = e
	}
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
			e.timer.Stop()
			if err := p.callHook(ctx, uid, e.sessionID, e.agent.history.all()); err != nil {
				slog.Error("memory sweep failed on shutdown", "userID", uid, "err", err)
				mu.Lock()
				errs = append(errs, fmt.Errorf("sweep for %s: %w", uid, err))
				mu.Unlock()
			}
		}(userID, entry)
	}
	wg.Wait()
	return errors.Join(errs...)
}

func (p *AgentPool) Reset(ctx context.Context, userID string) error {
	p.mu.Lock()
	e, ok := p.agents[userID]
	if !ok {
		p.mu.Unlock()
		return nil
	}
	delete(p.agents, userID)
	p.mu.Unlock()

	e.timer.Stop()
	slog.Info("session reset started", "userID", userID, "sessionID", e.sessionID)
	if err := p.callHook(ctx, userID, e.sessionID, e.agent.history.all()); err != nil {
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

func (p *AgentPool) getOrCreate(ctx context.Context, userID string) (*poolEntry, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if e, ok := p.agents[userID]; ok {
		return e, nil
	}

	sessionID, history, err := p.resolveSession(userID)
	if err != nil {
		return nil, err
	}

	a, err := p.factory(ctx, userID, history)
	if err != nil {
		return nil, err
	}

	timer := time.AfterFunc(p.timeout, func() {
		p.destroy(context.Background(), userID)
	})

	entry := &poolEntry{agent: a, timer: timer, sessionID: sessionID}
	p.agents[userID] = entry
	slog.Info("agent session created", "userID", userID, "sessionID", sessionID)
	return entry, nil
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

func (p *AgentPool) resetTimer(userID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.agents[userID]; ok {
		e.timer.Reset(p.timeout)
	}
}

func (p *AgentPool) destroy(ctx context.Context, userID string) {
	p.mu.Lock()
	e, ok := p.agents[userID]
	if !ok {
		p.mu.Unlock()
		return
	}
	delete(p.agents, userID)
	p.mu.Unlock()

	e.timer.Stop()
	slog.Info("agent session destroyed", "userID", userID, "sessionID", e.sessionID)
	if err := p.callHook(ctx, userID, e.sessionID, e.agent.history.all()); err != nil {
		slog.Error("memory sweep failed on eviction", "userID", userID, "err", err)
	}
}

func (p *AgentPool) callHook(ctx context.Context, userID, sessionID string, messages []*schema.Message) error {
	if p.hook != nil {
		return p.hook(ctx, userID, sessionID, messages)
	}
	return nil
}

func (p *AgentPool) record(userID, sessionID, role, content string) {
	if p.recorder != nil {
		p.recorder(userID, sessionID, role, content)
	}
}
