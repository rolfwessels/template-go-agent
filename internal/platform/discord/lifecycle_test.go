package discord

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/rolfwessels/template-go-agent/internal/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSession struct {
	mu         sync.Mutex
	handler    func(*discordgo.Session, *discordgo.MessageCreate)
	closeCalls int
	removed    chan struct{}
	closing    chan struct{}
	userErr    error
	closeErr   error
	onAdd      func()
	onOpen     func() error
	onClose    func()
	onTyping   func(context.Context)
}

func newFakeSession() *fakeSession {
	return &fakeSession{removed: make(chan struct{}), closing: make(chan struct{})}
}

func (s *fakeSession) User(string, ...discordgo.RequestOption) (*discordgo.User, error) {
	return &discordgo.User{ID: "bot-id"}, s.userErr
}

func (s *fakeSession) ChannelMessageSend(string, string, ...discordgo.RequestOption) (*discordgo.Message, error) {
	return &discordgo.Message{}, nil
}

func (s *fakeSession) ChannelTyping(_ string, options ...discordgo.RequestOption) error {
	if s.onTyping != nil {
		cfg := &discordgo.RequestConfig{Request: &http.Request{}}
		for _, option := range options {
			option(cfg)
		}
		s.onTyping(cfg.Request.Context())
	}
	return nil
}

func (s *fakeSession) AddHandler(handler interface{}) func() {
	s.mu.Lock()
	s.handler = handler.(func(*discordgo.Session, *discordgo.MessageCreate))
	s.mu.Unlock()
	if s.onAdd != nil {
		s.onAdd()
	}
	return func() {
		s.mu.Lock()
		s.handler = nil
		s.mu.Unlock()
		close(s.removed)
	}
}

// snapshot models an event already dispatched by Discord when its handler is removed.
func (s *fakeSession) snapshot() func(*discordgo.Session, *discordgo.MessageCreate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.handler
}

func (s *fakeSession) Open() error {
	if s.onOpen != nil {
		return s.onOpen()
	}
	return nil
}

func (s *fakeSession) Close() error {
	s.mu.Lock()
	s.closeCalls++
	if s.closeCalls == 1 {
		close(s.closing)
	}
	s.mu.Unlock()
	if s.onClose != nil {
		s.onClose()
	}
	return s.closeErr
}

type transcriberFunc func(context.Context, string) (string, error)

func (f transcriberFunc) Transcribe(ctx context.Context, url string) (string, error) {
	return f(ctx, url)
}

func TestAdapter_InitializesStateBeforeRegisteringAndOpening(t *testing.T) {
	a := New("unused", transcriberFunc(func(ctx context.Context, _ string) (string, error) {
		require.NotNil(t, ctx)
		require.NoError(t, ctx.Err())
		return "voice", nil
	}))
	s := newFakeSession()
	s.onAdd = func() {
		handler := s.snapshot()
		handler(nil, newTestMessage("bot-id", "channel", "", "self"))
		handler(nil, newVoiceMessage("user", "channel", "guild", "<@bot-id>"))
	}
	s.onOpen = func() error {
		s.snapshot()(nil, newTestMessage("user", "channel", "guild", "<@bot-id> hello"))
		return nil
	}
	require.NoError(t, a.connect(context.Background(), s))
	t.Cleanup(func() { assert.NoError(t, a.Disconnect(context.Background())) })

	require.Len(t, a.msgs, 2)
	assert.Equal(t, "voice", (<-a.msgs).Content)
	assert.Equal(t, "hello", (<-a.msgs).Content)
}

func assertMessagesStillOpen(t *testing.T, msgs <-chan platform.Message) {
	t.Helper()
	select {
	case _, ok := <-msgs:
		t.Fatalf("message channel should be open and empty; open = %v", ok)
	default:
	}
}

func TestAdapter_ShutdownWaitsForHandlersThenSession(t *testing.T) {
	for _, trigger := range []string{"disconnect", "cancel", "open failure"} {
		t.Run(trigger, func(t *testing.T) {
			entered := make(chan struct{})
			cancelled := make(chan struct{})
			releaseHandler := make(chan struct{})
			releaseClose := make(chan struct{})
			handled := make(chan struct{})
			result := make(chan error, 1)
			a := New("unused", transcriberFunc(func(ctx context.Context, _ string) (string, error) {
				close(entered)
				<-ctx.Done()
				close(cancelled)
				<-releaseHandler
				return "voice", nil
			}))
			s := newFakeSession()
			s.closeErr = errors.New("close error")
			s.onClose = func() { <-releaseClose }
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var queued func(*discordgo.Session, *discordgo.MessageCreate)
			startHandler := func() {
				queued = s.snapshot()
				go func() {
					queued(nil, newVoiceMessage("user", "channel", "", ""))
					close(handled)
				}()
				<-entered
			}
			openErr := errors.New("open error")
			if trigger == "open failure" {
				s.onOpen = func() error {
					startHandler()
					return openErr
				}
				go func() { result <- a.connect(ctx, s) }()
			} else {
				require.NoError(t, a.connect(ctx, s))
				startHandler()
				if trigger == "cancel" {
					cancel()
				} else {
					go func() { result <- a.Disconnect(context.Background()) }()
				}
			}

			// Removal is a barrier proving shutdown has rejected new handlers.
			<-s.removed
			<-cancelled
			assert.Nil(t, s.snapshot())
			assertMessagesStillOpen(t, a.msgs)
			select {
			case <-s.closing:
				t.Fatal("session closed while a handler was still active")
			default:
			}
			select {
			case <-result:
				t.Fatal("shutdown returned while a handler was still active")
			default:
			}
			// An already dispatched callback must be rejected too. Admission would
			// call the transcriber again and panic by closing entered twice.
			queued(nil, newVoiceMessage("user", "channel", "", ""))
			close(releaseHandler)
			<-handled
			<-s.closing
			assertMessagesStillOpen(t, a.msgs)
			close(releaseClose)

			_, ok := <-a.msgs
			require.False(t, ok)
			if trigger == "open failure" {
				require.ErrorIs(t, <-result, openErr)
			} else if trigger == "disconnect" {
				require.ErrorIs(t, <-result, s.closeErr)
			}
			queued(nil, newTestMessage("user", "channel", "", "after shutdown"))
			queued(nil, newVoiceMessage("user", "channel", "", ""))

			// Concurrent repeated shutdown must preserve the first Close result.
			var repeated sync.WaitGroup
			for range 8 {
				repeated.Go(func() {
					assert.ErrorIs(t, a.Disconnect(context.Background()), s.closeErr)
				})
			}
			repeated.Wait()
			s.mu.Lock()
			assert.Equal(t, 1, s.closeCalls)
			s.mu.Unlock()
		})
	}
}

func TestAdapter_ShutdownReleasesHandlerBeforeSendToFullChannel(t *testing.T) {
	a := New("unused", nil)
	s := newFakeSession()
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	s.onTyping = func(ctx context.Context) {
		close(entered)
		<-ctx.Done()
		close(cancelled)
		<-release
	}
	require.NoError(t, a.connect(context.Background(), s))
	for range cap(a.msgs) {
		a.msgs <- platform.Message{Content: "buffered"}
	}
	handled := make(chan struct{})
	queued := s.snapshot()
	go func() {
		queued(nil, newTestMessage("user", "channel", "", "late message"))
		close(handled)
	}()
	<-entered
	result := make(chan error, 1)
	go func() { result <- a.Disconnect(context.Background()) }()
	<-s.removed
	<-cancelled
	close(release)
	<-handled
	require.NoError(t, <-result)

	var count int
	for msg := range a.msgs {
		assert.Equal(t, "buffered", msg.Content)
		count++
	}
	assert.Equal(t, cap(a.msgs), count)
	queued(nil, newTestMessage("user", "channel", "", "after shutdown"))
}

func TestAdapter_DisconnectBeforeConnect(t *testing.T) {
	a := New("unused", nil)
	require.NoError(t, a.Disconnect(context.Background()))
	require.NoError(t, a.Disconnect(context.Background()))
	_, ok := <-a.msgs
	require.False(t, ok)
	require.Error(t, a.connect(context.Background(), newFakeSession()))
}

func TestAdapter_IdentityLookupFailureClosesSessionAndChannel(t *testing.T) {
	a := New("unused", nil)
	s := newFakeSession()
	s.userErr = errors.New("identity error")
	require.ErrorIs(t, a.connect(context.Background(), s), s.userErr)
	assert.Nil(t, s.snapshot())
	_, ok := <-a.msgs
	require.False(t, ok)
	require.NoError(t, a.Disconnect(context.Background()))
	assert.Equal(t, 1, s.closeCalls)
}
