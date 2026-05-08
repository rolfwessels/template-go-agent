package discord

import (
	"context"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type senderStub struct {
	lastChannelID string
	lastContent   string
}

func (s *senderStub) ChannelMessageSend(channelID, content string, _ ...discordgo.RequestOption) (*discordgo.Message, error) {
	s.lastChannelID = channelID
	s.lastContent = content
	return &discordgo.Message{}, nil
}

func newTestMessage(authorID, channelID, guildID, content string) *discordgo.MessageCreate {
	return &discordgo.MessageCreate{Message: &discordgo.Message{
		Author:    &discordgo.User{ID: authorID},
		ChannelID: channelID,
		GuildID:   guildID,
		Content:   content,
	}}
}

func TestAdapter_ReceivesDirectMessage(t *testing.T) {
	// arrange
	a := newWithSender("bot-id", &senderStub{})
	m := newTestMessage("user-123", "ch-456", "", "hello bot")

	// act
	a.onMessage(nil, m)

	// assert
	msg := <-a.msgs
	assert.Equal(t, "user-123", msg.UserID)
	assert.Equal(t, "ch-456", msg.ChannelID)
	assert.Equal(t, "hello bot", msg.Content)
}

func TestAdapter_ReceivesMentionInChannel(t *testing.T) {
	// arrange
	a := newWithSender("bot-id", &senderStub{})
	m := newTestMessage("user-123", "ch-789", "guild-1", "<@bot-id> what is the weather?")

	// act
	a.onMessage(nil, m)

	// assert
	msg := <-a.msgs
	assert.Equal(t, "user-123", msg.UserID)
	assert.Equal(t, "what is the weather?", msg.Content)
}

func TestAdapter_IgnoresChannelMessageWithoutMention(t *testing.T) {
	// arrange
	a := newWithSender("bot-id", &senderStub{})
	m := newTestMessage("user-123", "ch-789", "guild-1", "just chatting")

	// act
	a.onMessage(nil, m)

	// assert
	assert.Empty(t, a.msgs)
}

func TestAdapter_IgnoresSelfMessages(t *testing.T) {
	// arrange
	a := newWithSender("bot-id", &senderStub{})
	m := newTestMessage("bot-id", "ch-456", "", "I said something")

	// act
	a.onMessage(nil, m)

	// assert
	assert.Empty(t, a.msgs)
}

func TestAdapter_IgnoresEmptyMentionContent(t *testing.T) {
	// arrange
	a := newWithSender("bot-id", &senderStub{})
	m := newTestMessage("user-123", "ch-789", "guild-1", "<@bot-id>")

	// act
	a.onMessage(nil, m)

	// assert
	assert.Empty(t, a.msgs)
}

func TestAdapter_SendMessage(t *testing.T) {
	// arrange
	stub := &senderStub{}
	a := newWithSender("bot-id", stub)

	// act
	err := a.SendMessage(context.Background(), "ch-456", "hello world")

	// assert
	require.NoError(t, err)
	assert.Equal(t, "ch-456", stub.lastChannelID)
	assert.Equal(t, "hello world", stub.lastContent)
}

func TestAdapter_ReceiveMessages_ReturnsChannel(t *testing.T) {
	// arrange
	a := newWithSender("bot-id", &senderStub{})
	ctx := context.Background()

	// act
	ch, err := a.ReceiveMessages(ctx)

	// assert
	require.NoError(t, err)
	assert.NotNil(t, ch)
}

func TestSplitMessage_ShortMessage(t *testing.T) {
	assert.Equal(t, []string{"hello"}, splitMessage("hello"))
}

func TestSplitMessage_SplitsOnNewline(t *testing.T) {
	long := strings.Repeat("a", 1990) + "\n" + strings.Repeat("b", 100)
	parts := splitMessage(long)
	assert.Len(t, parts, 2)
	assert.Equal(t, strings.Repeat("a", 1990), parts[0])
	assert.Equal(t, strings.Repeat("b", 100), parts[1])
}

func TestSplitMessage_SplitsOnSpace(t *testing.T) {
	long := strings.Repeat("a", 1995) + " " + strings.Repeat("b", 100)
	parts := splitMessage(long)
	assert.Len(t, parts, 2)
	assert.Equal(t, strings.Repeat("a", 1995), parts[0])
}

func TestSplitMessage_HardSplitWhenNoBreak(t *testing.T) {
	long := strings.Repeat("a", 2500)
	parts := splitMessage(long)
	assert.Len(t, parts, 2)
	assert.Equal(t, 2000, len(parts[0]))
	assert.Equal(t, 500, len(parts[1]))
}
