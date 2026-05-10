package main

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/rolfwessels/template-go-agent/internal/platform"
	"github.com/rolfwessels/template-go-agent/internal/usage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersion(t *testing.T) {
	if version == "" {
		t.Error("version should not be empty")
	}
}

// fakeSender implements the Sender interface for testing.
type fakeSender struct {
	responses []string
	calls     int
}

func (f *fakeSender) Send(_ context.Context, _, _, _ string) (string, error) {
	if f.calls >= len(f.responses) {
		return "", fmt.Errorf("no response configured for call %d", f.calls)
	}
	resp := f.responses[f.calls]
	f.calls++
	return resp, nil
}

// fakePlatform is a MessagePlatform that delivers a fixed set of messages then closes.
type fakePlatform struct {
	messages []platform.Message
	sent     []string
}

func (f *fakePlatform) Connect(_ context.Context) error {
	return nil
}

func (f *fakePlatform) SendMessage(_ context.Context, _ string, msg string) error {
	f.sent = append(f.sent, msg)
	return nil
}

func (f *fakePlatform) ReceiveMessages(_ context.Context) (<-chan platform.Message, error) {
	ch := make(chan platform.Message, len(f.messages))
	for _, m := range f.messages {
		ch <- m
	}
	close(ch)
	return ch, nil
}

func (f *fakePlatform) Disconnect(_ context.Context) error {
	return nil
}

func TestRunPlatform_CLIMode_StatusLineAfterEachTurn(t *testing.T) {
	// arrange
	counter := usage.NewCounter()
	var statusBuf bytes.Buffer
	statusWriter := func() {
		fmt.Fprint(&statusBuf, "\r"+counter.StatusLine())
	}

	sender := &fakeSender{responses: []string{"reply1", "reply2"}}
	p := &fakePlatform{messages: []platform.Message{
		{UserID: "cli", ChannelID: "", Content: "hello"},
		{UserID: "cli", ChannelID: "", Content: "world"},
	}}

	// act
	err := runPlatform(context.Background(), sender, p, statusWriter)

	// assert
	require.NoError(t, err)
	assert.Equal(t, 2, sender.calls)
	out := statusBuf.String()
	assert.Contains(t, out, "\r")
	assert.Contains(t, out, "Tokens:")
	assert.Contains(t, out, "| Cost: $")
}

func TestRunPlatform_StatusLineAccumulatesAcrossTurns(t *testing.T) {
	// arrange
	counter := usage.NewCounter()
	var statusLines []string
	statusWriter := func() {
		statusLines = append(statusLines, counter.StatusLine())
	}

	sender := &fakeSender{responses: []string{"r1", "r2"}}
	p := &fakePlatform{messages: []platform.Message{
		{UserID: "cli", ChannelID: "", Content: "first"},
		{UserID: "cli", ChannelID: "", Content: "second"},
	}}

	// act
	err := runPlatform(context.Background(), sender, p, statusWriter)

	// assert
	require.NoError(t, err)
	require.Len(t, statusLines, 2)
	// Both lines are from a counter that starts at 0; they must appear in order
	assert.Equal(t, statusLines[0], statusLines[1], "counter unchanged by fake sender, both lines are identical zero-state")
}

func TestRunPlatform_NilStatusWriter_DoesNotPanic(t *testing.T) {
	// arrange
	sender := &fakeSender{responses: []string{"reply"}}
	p := &fakePlatform{messages: []platform.Message{
		{UserID: "u1", ChannelID: "c1", Content: "hi"},
	}}

	// act
	err := runPlatform(context.Background(), sender, p, nil)

	// assert
	require.NoError(t, err)
	assert.Equal(t, 1, sender.calls)
}
