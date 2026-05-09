package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type AgentSender interface {
	Send(ctx context.Context, userID, channelID, message string) (string, error)
}

type MessageSender interface {
	SendMessage(ctx context.Context, channelID string, msg string) error
}

type Scheduler struct {
	mu        sync.Mutex
	schedules map[string][]*Schedule
	store     *ScheduleStore
	pool      AgentSender
	platform  MessageSender
	stopCh    chan struct{}
	wg        sync.WaitGroup
	tickCh    <-chan time.Time
}

func New(store *ScheduleStore, pool AgentSender, platform MessageSender, opts ...func(*Scheduler)) *Scheduler {
	s := &Scheduler{
		schedules: make(map[string][]*Schedule),
		store:     store,
		pool:      pool,
		platform:  platform,
		stopCh:    make(chan struct{}),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

func WithTickCh(ch <-chan time.Time) func(*Scheduler) {
	return func(s *Scheduler) { s.tickCh = ch }
}

func (s *Scheduler) Start(ctx context.Context) error {
	all, err := s.store.LoadAll()
	if err != nil {
		return fmt.Errorf("loading schedules: %w", err)
	}
	s.mu.Lock()
	if all != nil {
		s.schedules = all
	}
	total := s.totalCount()
	next := s.nextSchedule()
	s.mu.Unlock()

	if next != nil {
		slog.Info("scheduler started", "schedules", total, "next", next.Prompt, "in", humanDuration(time.Until(time.Unix(next.NextFireAt, 0))))
	} else {
		slog.Info("scheduler started", "schedules", total)
	}

	s.wg.Add(1)
	go s.run()
	return nil
}

func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

func (s *Scheduler) Add(_ context.Context, schedule *Schedule) error {
	s.mu.Lock()
	s.schedules[schedule.UserID] = append(s.schedules[schedule.UserID], schedule)
	copy := cloneSlice(s.schedules[schedule.UserID])
	s.mu.Unlock()
	slog.Info("schedule added", "id", schedule.ID, "prompt", schedule.Prompt, "in", humanDuration(time.Until(time.Unix(schedule.NextFireAt, 0))))
	return s.store.Save(schedule.UserID, copy)
}

func (s *Scheduler) Cancel(_ context.Context, userID, id string) error {
	s.mu.Lock()
	s.schedules[userID] = removeByID(s.schedules[userID], id)
	copy := cloneSlice(s.schedules[userID])
	s.mu.Unlock()
	return s.store.Save(userID, copy)
}

func (s *Scheduler) ListForUser(userID string) []*Schedule {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneSlice(s.schedules[userID])
}

func (s *Scheduler) run() {
	defer s.wg.Done()
	var tickCh <-chan time.Time
	if s.tickCh != nil {
		tickCh = s.tickCh
	} else {
		t := time.NewTicker(60 * time.Second)
		defer t.Stop()
		tickCh = t.C
	}
	for {
		select {
		case <-s.stopCh:
			return
		case t := <-tickCh:
			s.fire(context.Background(), t)
		}
	}
}

func (s *Scheduler) fire(ctx context.Context, now time.Time) {
	s.mu.Lock()
	due := s.dueSchedules(now)
	s.mu.Unlock()

	for _, sc := range due {
		slog.Info("schedule firing", "id", sc.ID, "prompt", sc.Prompt)
		resp, err := s.pool.Send(ctx, sc.UserID, sc.ChannelID, "[Scheduled reminder] "+sc.Prompt)
		if err != nil {
			slog.Error("agent send failed for schedule", "id", sc.ID, "err", err)
			continue
		}
		slog.Info("schedule sending response", "id", sc.ID, "channel", sc.ChannelID)
		if err := s.platform.SendMessage(ctx, sc.ChannelID, resp); err != nil {
			slog.Error("platform send failed for schedule", "id", sc.ID, "err", err)
		}
		s.mu.Lock()
		s.advanceOrRemove(sc)
		userSchedules := cloneSlice(s.schedules[sc.UserID])
		s.mu.Unlock()
		if err := s.store.Save(sc.UserID, userSchedules); err != nil {
			slog.Error("saving schedules after fire", "id", sc.ID, "err", err)
		}
	}
}

func (s *Scheduler) dueSchedules(now time.Time) []*Schedule {
	var due []*Schedule
	for _, schedules := range s.schedules {
		for _, sc := range schedules {
			if now.Unix() >= sc.NextFireAt {
				due = append(due, sc)
			}
		}
	}
	return due
}

func (s *Scheduler) advanceOrRemove(sc *Schedule) {
	if sc.IntervalSeconds == 0 {
		s.schedules[sc.UserID] = removeByID(s.schedules[sc.UserID], sc.ID)
		return
	}
	for _, existing := range s.schedules[sc.UserID] {
		if existing.ID == sc.ID {
			existing.NextFireAt += sc.IntervalSeconds
			return
		}
	}
}

func (s *Scheduler) totalCount() int {
	n := 0
	for _, ss := range s.schedules {
		n += len(ss)
	}
	return n
}

func (s *Scheduler) nextSchedule() *Schedule {
	var next *Schedule
	for _, ss := range s.schedules {
		for _, sc := range ss {
			if next == nil || sc.NextFireAt < next.NextFireAt {
				next = sc
			}
		}
	}
	return next
}

func humanDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func cloneSlice(src []*Schedule) []*Schedule {
	out := make([]*Schedule, len(src))
	copy(out, src)
	return out
}

func removeByID(schedules []*Schedule, id string) []*Schedule {
	out := schedules[:0:0]
	for _, s := range schedules {
		if s.ID != id {
			out = append(out, s)
		}
	}
	return out
}
