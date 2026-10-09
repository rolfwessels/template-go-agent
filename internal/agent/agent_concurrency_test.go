package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerate_ConcurrentCallsSerializeHistory(t *testing.T) {
	gen := newBlockingGenerator()
	a := &Agent{react: gen, systemPrompt: "sys"}
	generate := func(ctx context.Context, question string) <-chan error {
		done := make(chan error, 1)
		go func() {
			_, err := a.Generate(ctx, question)
			done <- err
		}()
		return done
	}
	first := generate(context.Background(), "first")
	receive(t, gen.entered)
	ctx := &waitingContext{Context: context.Background(), waiting: make(chan struct{})}
	second := generate(ctx, "second")
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
	assert.Len(t, a.history, 4)
}

func TestGenerate_WaitingCallRespectsCancellation(t *testing.T) {
	gen := newBlockingGenerator()
	a := &Agent{react: gen}
	first := make(chan error, 1)
	go func() {
		_, err := a.Generate(context.Background(), "first")
		first <- err
	}()
	receive(t, gen.entered)
	base, cancel := context.WithCancel(context.Background())
	ctx := &waitingContext{Context: base, waiting: make(chan struct{})}
	second := make(chan error, 1)
	go func() {
		_, err := a.Generate(ctx, "canceled")
		second <- err
	}()
	receive(t, ctx.waiting)
	cancel()
	require.ErrorIs(t, receive(t, second), context.Canceled)
	gen.release <- struct{}{}
	require.NoError(t, receive(t, first))
	assert.Empty(t, gen.entered)
	assert.Len(t, a.history, 2)
}
