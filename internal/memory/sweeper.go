package memory

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/cloudwego/eino/schema"

	"github.com/rolfwessels/template-go-agent/internal/usage"
)

// sweepStore is the persistence needed to commit a sweep. Save and UpdateIndex
// must be idempotent so an uncommitted message range can safely be retried.
type sweepStore interface {
	ReadDailyFile(userID, date string) (string, error)
	Save(ctx context.Context, userID, date string, fact Fact) error
	UpdateIndex(userID string, summaries map[string]string) error
}

type Sweeper struct {
	store     sweepStore
	distiller Distiller
	sessions  *SessionStore
	tracker   usage.Tracker
}

type SweeperOption func(*Sweeper)

func WithSweeperTracker(tracker usage.Tracker) SweeperOption {
	return func(s *Sweeper) { s.tracker = tracker }
}

func NewSweeper(store sweepStore, distiller Distiller, sessions *SessionStore, opts ...SweeperOption) *Sweeper {
	s := &Sweeper{store: store, distiller: distiller, sessions: sessions}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// OnEvict processes session lines (cursor, total], grouped by date. The cursor
// counts physical JSONL lines and is committed only after all facts and the index
// have been persisted; any error leaves the entire range available for retry.
func (s *Sweeper) OnEvict(ctx context.Context, userID, sessionID string) error {
	if s.tracker != nil {
		ctx = usage.WithContext(ctx, userID, sessionID)
	}
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
			return fmt.Errorf("reading daily memory for %s: %w", date, err)
		}
		facts, summary, err := s.distiller.DistillAndSummarize(ctx, existing, byDate[date])
		if err != nil {
			return fmt.Errorf("distilling memory for %s: %w", date, err)
		}
		for _, fact := range facts {
			if err := s.store.Save(ctx, userID, date, fact); err != nil {
				return fmt.Errorf("storing memory fact for %s: %w", date, err)
			}
			slog.Info("memory fact stored", "userID", userID, "kind", fact.Kind, "date", date)
		}
		totalFacts += len(facts)
		if summary != "" {
			summaries["daily/"+date+".md"] = summary
		}
	}
	if err := s.store.UpdateIndex(userID, summaries); err != nil {
		return fmt.Errorf("updating memory index: %w", err)
	}
	if err := s.sessions.WriteCursor(userID, sessionID, total); err != nil {
		return fmt.Errorf("committing sweep cursor: %w", err)
	}
	slog.Info("memory sweep complete", "userID", userID, "facts", totalFacts)
	return nil
}

func sortedKeys(m map[string][]*schema.Message) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
