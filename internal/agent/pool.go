package agent

import (
	"context"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
)

type DestroyHook func(ctx context.Context, userID string, messages []*schema.Message)

type AgentFactory func(ctx context.Context, userID string) (*Agent, error)

type poolEntry struct {
	agent *Agent
	timer *time.Timer
}

type AgentPool struct {
	mu      sync.Mutex
	agents  map[string]*poolEntry
	factory AgentFactory
	timeout time.Duration
	hook    DestroyHook
}

func NewPool(factory AgentFactory, timeout time.Duration, hook DestroyHook) *AgentPool {
	return &AgentPool{
		agents:  make(map[string]*poolEntry),
		factory: factory,
		timeout: timeout,
		hook:    hook,
	}
}

func (p *AgentPool) Send(ctx context.Context, userID, message string) (string, error) {
	a, err := p.getOrCreate(ctx, userID)
	if err != nil {
		return "", err
	}
	resp, err := a.Generate(ctx, message)
	p.resetTimer(userID)
	return resp, err
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
			p.callHook(ctx, uid, e.agent.history.all())
		}(userID, entry)
	}
	wg.Wait()
}

func (p *AgentPool) getOrCreate(ctx context.Context, userID string) (*Agent, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if e, ok := p.agents[userID]; ok {
		return e.agent, nil
	}

	a, err := p.factory(ctx, userID)
	if err != nil {
		return nil, err
	}

	timer := time.AfterFunc(p.timeout, func() {
		p.destroy(context.Background(), userID)
	})

	p.agents[userID] = &poolEntry{agent: a, timer: timer}
	return a, nil
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
	p.callHook(ctx, userID, e.agent.history.all())
}

func (p *AgentPool) callHook(ctx context.Context, userID string, messages []*schema.Message) {
	if p.hook != nil {
		p.hook(ctx, userID, messages)
	}
}
