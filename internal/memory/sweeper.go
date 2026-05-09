package memory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/cloudwego/eino/schema"
)

type Distiller interface {
	Distill(ctx context.Context, messages []*schema.Message) ([]string, error)
}

type Sweeper struct {
	store     *FileStore
	distiller Distiller
	sessions  *SessionStore
}

func NewSweeper(store *FileStore, distiller Distiller, sessions *SessionStore) *Sweeper {
	return &Sweeper{store: store, distiller: distiller, sessions: sessions}
}

func (s *Sweeper) OnDestroy(ctx context.Context, userID, sessionID string, _ []*schema.Message) error {
	cursor, err := s.sessions.ReadCursor(userID, sessionID)
	if err != nil {
		return fmt.Errorf("reading sweep cursor: %w", err)
	}
	messages, total, err := s.sessions.ReadFrom(userID, sessionID, cursor)
	if err != nil {
		return fmt.Errorf("reading session messages: %w", err)
	}
	if len(messages) == 0 {
		return nil
	}
	slog.Info("memory sweep started", "userID", userID, "sessionID", sessionID, "messages", len(messages))
	facts, err := s.distiller.Distill(ctx, messages)
	if err != nil {
		return fmt.Errorf("distilling memories for %s: %w", userID, err)
	}
	slog.Info("memory sweep complete", "userID", userID, "facts", len(facts))
	for i, fact := range facts {
		e := Entry{
			ID:      fmt.Sprintf("%s-%d-%d", userID, cursor, i),
			UserID:  userID,
			Content: strings.TrimSpace(fact),
		}
		if err := s.store.Save(ctx, e); err != nil {
			continue
		}
		slog.Info("memory fact stored", "userID", userID, "entryID", e.ID, "fact", e.Content)
	}
	return s.sessions.WriteCursor(userID, sessionID, total)
}
