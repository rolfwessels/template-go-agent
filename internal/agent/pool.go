package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
)

type DestroyHook func(ctx context.Context, userID, sessionID string, messages []*schema.Message)
type RecordHook func(userID, sessionID, role, content string)

type AgentFactory func(ctx context.Context, userID string) (*Agent, error)

type poolEntry struct {
	agent     *Agent
	timer     *time.Timer
	sessionID string
}

type AgentPool struct {
	mu       sync.Mutex
	agents   map[string]*poolEntry
	factory  AgentFactory
	timeout  time.Duration
	hook     DestroyHook
	recorder RecordHook
}

func WithRecordHook(h RecordHook) func(*AgentPool) {
	return func(p *AgentPool) { p.recorder = h }
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

func (p *AgentPool) Shutdown(ctx context.Context) {
	p.mu.Lock()
	entries := make(map[string]*poolEntry, len(p.agents))
	for id, e := range p.agents {
		entries[id] = e
	}
	p.agents = make(map[string]*poolEntry)
	p.mu.Unlock()

	var wg sync.WaitGroup
	for userID, entry := range entries {
		wg.Add(1)
		go func(uid string, e *poolEntry) {
			defer wg.Done()
			e.timer.Stop()
			p.callHook(ctx, uid, e.sessionID, e.agent.history.all())
		}(userID, entry)
	}
	wg.Wait()
}

func (p *AgentPool) getOrCreate(ctx context.Context, userID string) (*poolEntry, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if e, ok := p.agents[userID]; ok {
		return e, nil
	}

	a, err := p.factory(ctx, userID)
	if err != nil {
		return nil, err
	}

	sessionID := fmt.Sprintf("%d", time.Now().UnixNano())
	timer := time.AfterFunc(p.timeout, func() {
		p.destroy(context.Background(), userID)
	})

	entry := &poolEntry{agent: a, timer: timer, sessionID: sessionID}
	p.agents[userID] = entry
	slog.Info("agent session created", "userID", userID, "sessionID", sessionID)
	return entry, nil
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
	p.callHook(ctx, userID, e.sessionID, e.agent.history.all())
}

func (p *AgentPool) callHook(ctx context.Context, userID, sessionID string, messages []*schema.Message) {
	if p.hook != nil {
		p.hook(ctx, userID, sessionID, messages)
	}
}

func (p *AgentPool) record(userID, sessionID, role, content string) {
	if p.recorder != nil {
		p.recorder(userID, sessionID, role, content)
	}
}
