package agent

import (
	"context"
	"sync"
)

// turnLock is a zero-value, context-aware mutex. A closed stopping channel also
// releases pool callers waiting for a turn when shutdown starts.
type turnLock struct {
	once  sync.Once
	token chan struct{}
}

func (l *turnLock) lock(ctx context.Context, stopping <-chan struct{}) error {
	l.once.Do(func() {
		l.token = make(chan struct{}, 1)
		l.token <- struct{}{}
	})
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-stopping:
		return ErrPoolClosed
	case <-l.token:
		if err := ctx.Err(); err != nil {
			l.unlock()
			return err
		}
		return nil
	}
}

func (l *turnLock) unlock() {
	l.token <- struct{}{}
}
