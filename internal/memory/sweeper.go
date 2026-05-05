package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

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

func (s *Sweeper) OnDestroy(ctx context.Context, userID string, messages []*schema.Message) {
	if len(messages) == 0 {
		return
	}
	sessionID := fmt.Sprintf("%d", time.Now().UnixNano())
	facts, err := s.distiller.Distill(ctx, messages)
	if err != nil || len(facts) == 0 {
		return
	}
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
		if s.vector != nil {
			_ = s.vector.Add(ctx, e)
		}
	}
}
