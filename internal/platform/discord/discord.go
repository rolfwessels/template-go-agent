package discord

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/rolfwessels/template-go-agent/internal/platform"
)

type sender interface {
	ChannelMessageSend(channelID, content string, options ...discordgo.RequestOption) (*discordgo.Message, error)
}

type session interface {
	sender
	User(string, ...discordgo.RequestOption) (*discordgo.User, error)
	ChannelTyping(string, ...discordgo.RequestOption) error
	AddHandler(interface{}) func()
	Open() error
	Close() error
}

type Adapter struct {
	token         string
	session       session
	send          sender
	botID         string
	msgs          chan platform.Message
	transcriber   Transcriber
	ctx           context.Context
	cancel        context.CancelFunc
	removeHandler func()

	// lifecycle serializes connection setup and shutdown. mu guards admission
	// so no handler can increment handlers after shutdown starts waiting.
	lifecycle sync.Mutex
	mu        sync.Mutex
	handlers  sync.WaitGroup
	active    bool
	closed    bool
	started   bool
	closeErr  error
}

func New(token string, tr Transcriber) *Adapter {
	return &Adapter{token: token, transcriber: tr, msgs: make(chan platform.Message, 64)}
}

func newWithSender(botID string, s sender, tr Transcriber) *Adapter {
	a := New("", tr)
	a.botID = botID
	a.send = s
	a.ctx, a.cancel = context.WithCancel(context.Background())
	a.active = true
	return a
}

func (a *Adapter) Connect(ctx context.Context) error {
	s, err := discordgo.New("Bot " + a.token)
	if err != nil {
		return fmt.Errorf("creating discord session: %w", err)
	}
	s.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages | discordgo.IntentsMessageContent
	return a.connect(ctx, s)
}

func (a *Adapter) connect(ctx context.Context, s session) error {
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	if a.closed || a.started {
		return fmt.Errorf("discord adapter already connected or closed")
	}
	a.started = true
	a.session = s
	a.send = s
	a.ctx, a.cancel = context.WithCancel(ctx)

	// Resolve the bot identity before registering handlers: Open can dispatch
	// messages before it returns, and State.User is populated by ready events.
	user, err := s.User("@me", discordgo.WithContext(a.ctx))
	if err != nil {
		_ = a.shutdown()
		return fmt.Errorf("getting discord bot identity: %w", err)
	}
	a.botID = user.ID
	a.mu.Lock()
	a.active = true
	a.mu.Unlock()
	a.removeHandler = s.AddHandler(a.onMessage)

	if err := s.Open(); err != nil {
		_ = a.shutdown()
		return fmt.Errorf("opening discord connection: %w", err)
	}

	go func() {
		<-a.ctx.Done()
		_ = a.Disconnect(context.Background())
	}()
	return nil
}

func (a *Adapter) onMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	a.mu.Lock()
	if !a.active || a.closed || a.ctx.Err() != nil {
		a.mu.Unlock()
		return
	}
	a.handlers.Add(1)
	a.mu.Unlock()
	defer a.handlers.Done()

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

	if a.ctx.Err() != nil {
		return
	}
	if a.session != nil {
		_ = a.session.ChannelTyping(m.ChannelID, discordgo.WithContext(a.ctx))
	}

	select {
	case a.msgs <- platform.Message{UserID: m.Author.ID, ChannelID: m.ChannelID, Content: content}:
	case <-a.ctx.Done():
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
			_, _ = a.send.ChannelMessageSend(m.ChannelID, "Sorry, I couldn't transcribe your voice note.", discordgo.WithContext(a.ctx))
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
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	return a.shutdown()
}

// shutdown runs with lifecycle held, including when connection setup fails.
func (a *Adapter) shutdown() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return a.closeErr
	}
	a.closed = true
	a.mu.Unlock()

	if a.cancel != nil {
		a.cancel()
	}
	if a.removeHandler != nil {
		a.removeHandler()
	}
	a.handlers.Wait()
	if a.session != nil {
		a.closeErr = a.session.Close()
	}
	close(a.msgs)
	return a.closeErr
}
