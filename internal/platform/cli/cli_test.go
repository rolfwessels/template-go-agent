package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/rolfwessels/template-go-agent/internal/platform/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdapter_ReceivesMessages(t *testing.T) {
	// arrange
	adapter := cli.NewWithIO(strings.NewReader("hello\nworld\n"), &bytes.Buffer{})
	ctx := context.Background()

	// act
	require.NoError(t, adapter.Connect(ctx))
	msgs, err := adapter.ReceiveMessages(ctx)
	require.NoError(t, err)

	var received []string
	for msg := range msgs {
		received = append(received, msg.Content)
	}

	// assert
	assert.Equal(t, []string{"hello", "world"}, received)
}

func TestAdapter_SkipsEmptyLines(t *testing.T) {
	// arrange
	adapter := cli.NewWithIO(strings.NewReader("\nhello\n\nworld\n"), &bytes.Buffer{})
	ctx := context.Background()

	// act
	require.NoError(t, adapter.Connect(ctx))
	msgs, err := adapter.ReceiveMessages(ctx)
	require.NoError(t, err)

	var received []string
	for msg := range msgs {
		received = append(received, msg.Content)
	}

	// assert
	assert.Equal(t, []string{"hello", "world"}, received)
}

func TestAdapter_MessageHasCliUserID(t *testing.T) {
	// arrange
	adapter := cli.NewWithIO(strings.NewReader("hello\n"), &bytes.Buffer{})
	ctx := context.Background()

	// act
	require.NoError(t, adapter.Connect(ctx))
	msgs, err := adapter.ReceiveMessages(ctx)
	require.NoError(t, err)
	msg := <-msgs

	// assert
	assert.Equal(t, "cli", msg.UserID)
	assert.Equal(t, "", msg.ChannelID)
}

func TestAdapter_SendMessage(t *testing.T) {
	// arrange
	var out bytes.Buffer
	adapter := cli.NewWithIO(strings.NewReader(""), &out)
	ctx := context.Background()

	// act
	require.NoError(t, adapter.Connect(ctx))
	require.NoError(t, adapter.SendMessage(ctx, "", "hello"))

	// assert
	assert.Equal(t, "hello\n", out.String())
}
