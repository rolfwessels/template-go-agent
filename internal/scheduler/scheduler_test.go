package scheduler_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rolfwessels/template-go-agent/internal/scheduler"
)

func sampleSchedule(t *testing.T) *scheduler.Schedule {
	t.Helper()
	return &scheduler.Schedule{
		ID:         "sched-1",
		UserID:     "user-1",
		ChannelID:  "chan-1",
		Prompt:     "say hello",
		NextFireAt: time.Now().Add(time.Hour).Unix(),
	}
}

func recurringSchedule(t *testing.T) *scheduler.Schedule {
	t.Helper()
	s := sampleSchedule(t)
	s.IntervalSeconds = 3600
	return s
}

func setupScheduler(t *testing.T, opts ...func(*scheduler.Scheduler)) (*scheduler.Scheduler, *fakeAgentSender, *fakePlatform) {
	t.Helper()
	dir := t.TempDir()
	store := scheduler.NewStore(dir)
	pool := &fakeAgentSender{response: "hello back"}
	plat := &fakePlatform{}
	sched := scheduler.New(store, pool, plat, opts...)
	return sched, pool, plat
}

// --- ScheduleStore tests ---

func TestScheduleStore_RoundTrip(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := scheduler.NewStore(dir)
	ctx := context.Background()
	sc := sampleSchedule(t)
	sched := scheduler.New(store, &fakeAgentSender{}, &fakePlatform{})

	// act — add then reload
	err := sched.Add(ctx, sc)
	require.NoError(t, err)

	loaded, err := store.Load("user-1")
	require.NoError(t, err)

	// assert
	require.Len(t, loaded, 1)
	assert.Equal(t, sc.ID, loaded[0].ID)
	assert.Equal(t, sc.UserID, loaded[0].UserID)
	assert.Equal(t, sc.ChannelID, loaded[0].ChannelID)
	assert.Equal(t, sc.Prompt, loaded[0].Prompt)
	assert.Equal(t, sc.NextFireAt, loaded[0].NextFireAt)
	assert.Equal(t, sc.IntervalSeconds, loaded[0].IntervalSeconds)
}

func TestScheduleStore_Cancel_RemovesFromDisk(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := scheduler.NewStore(dir)
	ctx := context.Background()
	sc := sampleSchedule(t)
	sched := scheduler.New(store, &fakeAgentSender{}, &fakePlatform{})
	require.NoError(t, sched.Add(ctx, sc))

	// act
	require.NoError(t, sched.Cancel(ctx, sc.UserID, sc.ID))

	// assert
	loaded, err := store.Load("user-1")
	require.NoError(t, err)
	assert.Empty(t, loaded)
}

func TestScheduleStore_LoadNonExistent_ReturnsEmpty(t *testing.T) {
	store := scheduler.NewStore(t.TempDir())
	loaded, err := store.Load("nobody")
	require.NoError(t, err)
	assert.Nil(t, loaded)
}

// --- Scheduler memory tests ---

func TestScheduler_Add_StoresInMemoryAndDisk(t *testing.T) {
	// arrange
	sched, _, _ := setupScheduler(t)
	ctx := context.Background()
	sc := sampleSchedule(t)

	// act
	require.NoError(t, sched.Add(ctx, sc))

	// assert — in memory
	list := sched.ListForUser("user-1")
	require.Len(t, list, 1)
	assert.Equal(t, "sched-1", list[0].ID)
}

func TestScheduler_Cancel_RemovesFromMemoryAndDisk(t *testing.T) {
	// arrange
	sched, _, _ := setupScheduler(t)
	ctx := context.Background()
	sc := sampleSchedule(t)
	require.NoError(t, sched.Add(ctx, sc))

	// act
	require.NoError(t, sched.Cancel(ctx, "user-1", "sched-1"))

	// assert
	assert.Empty(t, sched.ListForUser("user-1"))
}

func TestScheduler_ListForUser_ReturnsActiveSchedules(t *testing.T) {
	// arrange
	sched, _, _ := setupScheduler(t)
	ctx := context.Background()
	sc1 := &scheduler.Schedule{ID: "s1", UserID: "user-1", ChannelID: "c", Prompt: "a", NextFireAt: 1}
	sc2 := &scheduler.Schedule{ID: "s2", UserID: "user-1", ChannelID: "c", Prompt: "b", NextFireAt: 2}
	sc3 := &scheduler.Schedule{ID: "s3", UserID: "user-2", ChannelID: "c", Prompt: "c", NextFireAt: 3}
	require.NoError(t, sched.Add(ctx, sc1))
	require.NoError(t, sched.Add(ctx, sc2))
	require.NoError(t, sched.Add(ctx, sc3))

	// act
	list := sched.ListForUser("user-1")

	// assert
	assert.Len(t, list, 2)
	assert.Empty(t, sched.ListForUser("unknown"))
}

// --- Goroutine / firing tests ---

func TestScheduler_OneShotFiresOnceAndIsRemoved(t *testing.T) {
	// arrange
	tickCh := make(chan time.Time, 1)
	sched, pool, plat := setupScheduler(t, scheduler.WithTickCh(tickCh))
	ctx := context.Background()

	sc := &scheduler.Schedule{
		ID: "s1", UserID: "user-1", ChannelID: "chan-1",
		Prompt:     "remind me",
		NextFireAt: time.Now().Add(-time.Second).Unix(),
	}
	require.NoError(t, sched.Add(ctx, sc))
	require.NoError(t, sched.Start(ctx))
	t.Cleanup(sched.Stop)

	// act — fire one tick
	tickCh <- time.Now()
	require.Eventually(t, func() bool {
		return pool.callCount() == 1
	}, time.Second, 5*time.Millisecond)

	// assert
	assert.Equal(t, 1, pool.callCount())
	assert.Equal(t, 1, plat.callCount())
	assert.Empty(t, sched.ListForUser("user-1"))
}

func TestScheduler_RecurringAdvancesNextFireAt(t *testing.T) {
	// arrange
	tickCh := make(chan time.Time, 1)
	sched, pool, _ := setupScheduler(t, scheduler.WithTickCh(tickCh))
	ctx := context.Background()

	interval := int64(3600)
	fireAt := time.Now().Add(-time.Second).Unix()
	sc := &scheduler.Schedule{
		ID: "s1", UserID: "user-1", ChannelID: "chan-1",
		Prompt: "daily", NextFireAt: fireAt, IntervalSeconds: interval,
	}
	require.NoError(t, sched.Add(ctx, sc))
	require.NoError(t, sched.Start(ctx))
	t.Cleanup(sched.Stop)

	// act — first tick fires
	tickCh <- time.Now()
	require.Eventually(t, func() bool {
		return pool.callCount() == 1
	}, time.Second, 5*time.Millisecond)

	// assert — still present, NextFireAt advanced
	list := sched.ListForUser("user-1")
	require.Len(t, list, 1)
	assert.Equal(t, fireAt+interval, list[0].NextFireAt)

	// act — second tick fires (schedule is due again if we move time past the new NextFireAt)
	tickCh <- time.Unix(fireAt+interval+1, 0)
	require.Eventually(t, func() bool {
		return pool.callCount() == 2
	}, time.Second, 5*time.Millisecond)

	list = sched.ListForUser("user-1")
	require.Len(t, list, 1)
	assert.Equal(t, fireAt+2*interval, list[0].NextFireAt)
}

func TestScheduler_CleanShutdown_DoesNotFirePendingSchedules(t *testing.T) {
	// arrange
	tickCh := make(chan time.Time, 1)
	sched, pool, _ := setupScheduler(t, scheduler.WithTickCh(tickCh))
	ctx := context.Background()

	sc := &scheduler.Schedule{
		ID: "s1", UserID: "user-1", ChannelID: "chan-1",
		Prompt:     "pending",
		NextFireAt: time.Now().Add(-time.Second).Unix(),
	}
	require.NoError(t, sched.Add(ctx, sc))
	require.NoError(t, sched.Start(ctx))

	// act — stop before any tick; Stop blocks until goroutine exits
	sched.Stop()

	// send tick after stop — goroutine is gone, nobody reads it
	tickCh <- time.Now()

	// assert
	assert.Equal(t, 0, pool.callCount())
}

func TestScheduler_Start_LoadsFromDisk(t *testing.T) {
	// arrange — populate disk via a separate scheduler instance
	dir := t.TempDir()
	store := scheduler.NewStore(dir)
	pool := &fakeAgentSender{response: "ok"}
	plat := &fakePlatform{}

	seedSched := scheduler.New(store, pool, plat)
	ctx := context.Background()
	sc := sampleSchedule(t)
	require.NoError(t, seedSched.Add(ctx, sc))

	// act — new scheduler instance loads from disk
	tickCh := make(chan time.Time, 1)
	fresh := scheduler.New(store, pool, plat, scheduler.WithTickCh(tickCh))
	require.NoError(t, fresh.Start(ctx))
	t.Cleanup(fresh.Stop)

	// assert — schedule is in memory without re-adding
	list := fresh.ListForUser("user-1")
	require.Len(t, list, 1)
	assert.Equal(t, sc.ID, list[0].ID)
}

// --- fakes ---

type fakeAgentSender struct {
	mu       sync.Mutex
	calls    int
	response string
}

func (f *fakeAgentSender) Send(_ context.Context, _, _, _ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.response, nil
}

func (f *fakeAgentSender) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakePlatform struct {
	mu    sync.Mutex
	calls int
}

func (f *fakePlatform) SendMessage(_ context.Context, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return nil
}

func (f *fakePlatform) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}
