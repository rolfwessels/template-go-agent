package memory

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

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

	facts, err := s.distiller.Distill(ctx, batchWithDateHeaders(dates, byDate))
	if err != nil {
		return fmt.Errorf("distilling memories for %s: %w", userID, err)
	}
	slog.Info("memory sweep complete", "userID", userID, "facts", len(facts))

	dailyByDate := make(map[string][]Fact)
	for _, fact := range facts {
		if err := s.store.Save(ctx, userID, fact); err != nil {
			slog.Warn("memory fact store failed", "userID", userID, "err", err)
			continue
		}
		slog.Info("memory fact stored", "userID", userID, "kind", fact.Kind, "date", fact.Date, "fact", fact.Content)
		if fact.Kind == KindDaily {
			dailyByDate[fact.Date] = append(dailyByDate[fact.Date], fact)
		}
	}

	summaries := make(map[string]string)
	for date, dailyFacts := range dailyByDate {
		rel := "daily/" + resolveDate(date) + ".md"
		summary, err := s.distiller.Summarize(ctx, dailyFacts)
		if err != nil {
			slog.Warn("memory summary failed", "userID", userID, "date", date, "err", err)
			continue
		}
		if summary != "" {
			summaries[rel] = summary
		}
	}

	if err := s.store.UpdateIndex(userID, summaries); err != nil {
		slog.Warn("memory index update failed", "userID", userID, "err", err)
	}
	return s.sessions.WriteCursor(userID, sessionID, total)
}

func batchWithDateHeaders(dates []string, byDate map[string][]*schema.Message) []*schema.Message {
	var all []*schema.Message
	for _, date := range dates {
		all = append(all, schema.UserMessage("--- "+date+" ---"))
		all = append(all, byDate[date]...)
	}
	return all
}

func resolveDate(date string) string {
	if date == "" {
		return time.Now().UTC().Format("2006-01-02")
	}
	return date
}

func sortedKeys(m map[string][]*schema.Message) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
