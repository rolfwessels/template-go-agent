package usagetest

import (
	"sync"

	"github.com/rolfwessels/template-go-agent/internal/usage"
)

type StubTracker struct {
	mu         sync.Mutex
	components []string
}

func (s *StubTracker) Record(_, _, component, _ string, _ usage.TokenUsage) {
	s.mu.Lock()
	s.components = append(s.components, component)
	s.mu.Unlock()
}

func (s *StubTracker) Snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.components))
	copy(out, s.components)
	return out
}
