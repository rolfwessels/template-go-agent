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
	vector    VectorStore
	distiller Distiller
}

func NewSweeper(store *FileStore, vector VectorStore, distiller Distiller) *Sweeper {
	return &Sweeper{store: store, vector: vector, distiller: distiller}
}

func (s *Sweeper) OnDestroy(ctx context.Context, userID, sessionID string, messages []*schema.Message) {
	if len(messages) == 0 {
		return
	}
	slog.Info("memory sweep started", "userID", userID, "sessionID", sessionID, "messages", len(messages))
	facts, err := s.distiller.Distill(ctx, messages)
	if err != nil || len(facts) == 0 {
		slog.Info("memory sweep complete", "userID", userID, "facts", 0)
		return
	}
	slog.Info("memory sweep complete", "userID", userID, "facts", len(facts))
	for i, fact := range facts {
		e := Entry{
			ID:        fmt.Sprintf("%s-%d", sessionID, i),
			UserID:    userID,
			SessionID: sessionID,
			Content:   strings.TrimSpace(fact),
		}
		if err := s.store.Save(ctx, e); err != nil {
			continue
		}
		slog.Info("memory fact stored", "userID", userID, "entryID", e.ID, "fact", e.Content)
		if s.vector != nil {
			_ = s.vector.Add(ctx, e)
		}
	}
}
