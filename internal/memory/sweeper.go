package memory

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/cloudwego/eino/schema"
)

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
	byDate, total, err := s.sessions.ReadFromByDate(userID, sessionID, cursor)
	if err != nil {
		return fmt.Errorf("reading session messages: %w", err)
	}
	if len(byDate) == 0 {
		return nil
	}

	dates := sortedKeys(byDate)
	msgCount := 0
	for _, msgs := range byDate {
		msgCount += len(msgs)
	}
	slog.Info("memory sweep started", "userID", userID, "sessionID", sessionID, "messages", msgCount, "dates", len(dates))

	summaries := make(map[string]string)
	totalFacts := 0
	for _, date := range dates {
		existing, err := s.store.ReadDailyFile(userID, date)
		if err != nil {
			slog.Warn("memory read daily file failed", "userID", userID, "date", date, "err", err)
		}
		facts, summary, err := s.distiller.DistillAndSummarize(ctx, existing, byDate[date])
		if err != nil {
			slog.Warn("memory distill failed", "userID", userID, "date", date, "err", err)
			continue
		}
		for _, fact := range facts {
			if err := s.store.Save(ctx, userID, date, fact); err != nil {
				slog.Warn("memory fact store failed", "userID", userID, "err", err)
				continue
			}
			slog.Info("memory fact stored", "userID", userID, "kind", fact.Kind, "date", date, "fact", fact.Content)
		}
		totalFacts += len(facts)
		if summary != "" {
			summaries["daily/"+date+".md"] = summary
		}
	}
	slog.Info("memory sweep complete", "userID", userID, "facts", totalFacts)

	if err := s.store.UpdateIndex(userID, summaries); err != nil {
		slog.Warn("memory index update failed", "userID", userID, "err", err)
	}
	return s.sessions.WriteCursor(userID, sessionID, total)
}

func sortedKeys(m map[string][]*schema.Message) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
