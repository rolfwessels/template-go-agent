package discord

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/rolfwessels/template-go-agent/internal/platform"
)

type sender interface {
	ChannelMessageSend(channelID, content string, options ...discordgo.RequestOption) (*discordgo.Message, error)
}

type Adapter struct {
	token       string
	session     *discordgo.Session
	send        sender
	botID       string
	msgs        chan platform.Message
	transcriber Transcriber
	ctx         context.Context
}

func New(token string, tr Transcriber) *Adapter {
	return &Adapter{token: token, transcriber: tr}
}

func newWithSender(botID string, s sender, tr Transcriber) *Adapter {
	return &Adapter{botID: botID, send: s, msgs: make(chan platform.Message, 64), transcriber: tr}
}

func (a *Adapter) Connect(ctx context.Context) error {
	s, err := discordgo.New("Bot " + a.token)
	if err != nil {
		return fmt.Errorf("creating discord session: %w", err)
	}
	a.session = s
	a.send = s
	a.msgs = make(chan platform.Message, 64)

	s.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages | discordgo.IntentsMessageContent
	s.AddHandler(a.onMessage)

	if err := s.Open(); err != nil {
		return fmt.Errorf("opening discord connection: %w", err)
	}
	a.botID = s.State.User.ID
	a.ctx = ctx

	go func() {
		<-ctx.Done()
		close(a.msgs)
	}()
	return nil
}

func (a *Adapter) onMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.ID == a.botID {
		return
	}

	content := m.Content
	if m.GuildID != "" {
		mention := "<@" + a.botID + ">"
		if !strings.Contains(content, mention) {
			return
		}
		content = strings.TrimSpace(strings.ReplaceAll(content, mention, ""))
	}

	if content == "" {
		transcribed, ok := a.transcribeAttachment(m)
		if !ok {
			return
		}
		content = transcribed
	}

	if a.session != nil {
		_ = a.session.ChannelTyping(m.ChannelID)
	}

	select {
	case a.msgs <- platform.Message{UserID: m.Author.ID, ChannelID: m.ChannelID, Content: content}:
	default:
	}
}

func (a *Adapter) transcribeAttachment(m *discordgo.MessageCreate) (string, bool) {
	if a.transcriber == nil {
		return "", false
	}
	for _, att := range m.Attachments {
		if !strings.HasPrefix(att.ContentType, "audio/") {
			continue
		}
		slog.Info("voice note received", "channel", m.ChannelID, "user", m.Author.ID, "content_type", att.ContentType)
		text, err := a.transcriber.Transcribe(a.ctx, att.URL)
		if err != nil {
			slog.Error("voice note transcription failed", "err", err, "channel", m.ChannelID)
			_, _ = a.send.ChannelMessageSend(m.ChannelID, "Sorry, I couldn't transcribe your voice note.")
			return "", false
		}
		slog.Info("voice note transcribed", "channel", m.ChannelID, "user", m.Author.ID, "chars", len(text))
		return text, true
	}
	return "", false
}

const maxMessageLen = 2000

func (a *Adapter) SendMessage(_ context.Context, channelID string, msg string) error {
	for _, chunk := range splitMessage(msg) {
		if _, err := a.send.ChannelMessageSend(channelID, chunk); err != nil {
			return err
		}
	}
	return nil
}

func splitMessage(msg string) []string {
	if len(msg) <= maxMessageLen {
		return []string{msg}
	}
	var parts []string
	for len(msg) > maxMessageLen {
		cut := strings.LastIndex(msg[:maxMessageLen], "\n")
		if cut <= 0 {
			cut = strings.LastIndex(msg[:maxMessageLen], " ")
		}
		if cut <= 0 {
			cut = maxMessageLen
		}
		parts = append(parts, msg[:cut])
		msg = strings.TrimLeft(msg[cut:], "\n ")
	}
	if msg != "" {
		parts = append(parts, msg)
	}
	return parts
}

func (a *Adapter) ReceiveMessages(_ context.Context) (<-chan platform.Message, error) {
	return a.msgs, nil
}

func (a *Adapter) Disconnect(_ context.Context) error {
	if a.session != nil {
		return a.session.Close()
	}
	return nil
}
