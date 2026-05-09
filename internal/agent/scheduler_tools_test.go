package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rolfwessels/template-go-agent/internal/scheduler"
)

type fakeScheduleManager struct {
	added    []*scheduler.Schedule
	cancelled []struct{ userID, id string }
	schedules []*scheduler.Schedule
}

func (f *fakeScheduleManager) Add(_ context.Context, s *scheduler.Schedule) error {
	f.added = append(f.added, s)
	return nil
}

func (f *fakeScheduleManager) Cancel(_ context.Context, userID, id string) error {
	f.cancelled = append(f.cancelled, struct{ userID, id string }{userID, id})
	return nil
}

func (f *fakeScheduleManager) ListForUser(_ string) []*scheduler.Schedule {
	return f.schedules
}

func TestScheduleReminderTool_CreatesScheduleWithCorrectFields(t *testing.T) {
	// arrange
	mgr := &fakeScheduleManager{}
	tl := newScheduleReminderTool(mgr, "user-1", "chan-1")
	fireAt := time.Now().Add(time.Hour).Unix()
	input := `{"prompt":"check the oven","next_fire_at":` + jsonInt(fireAt) + `,"interval_seconds":3600}`

	// act
	result, err := tl.InvokableRun(context.Background(), input)
	require.NoError(t, err)

	// assert
	require.Len(t, mgr.added, 1)
	sc := mgr.added[0]
	assert.Equal(t, "user-1", sc.UserID)
	assert.Equal(t, "chan-1", sc.ChannelID)
	assert.Equal(t, "check the oven", sc.Prompt)
	assert.Equal(t, fireAt, sc.NextFireAt)
	assert.Equal(t, int64(3600), sc.IntervalSeconds)
	assert.NotEmpty(t, sc.ID)
	assert.Contains(t, result, sc.ID)
}

func TestScheduleReminderTool_OneShotWhenNoInterval(t *testing.T) {
	// arrange
	mgr := &fakeScheduleManager{}
	tl := newScheduleReminderTool(mgr, "user-1", "chan-1")
	fireAt := time.Now().Add(time.Hour).Unix()
	input := `{"prompt":"wake up","next_fire_at":` + jsonInt(fireAt) + `}`

	// act
	_, err := tl.InvokableRun(context.Background(), input)
	require.NoError(t, err)

	// assert — IntervalSeconds stays zero (one-shot)
	require.Len(t, mgr.added, 1)
	assert.Equal(t, int64(0), mgr.added[0].IntervalSeconds)
}

func TestListRemindersTool_ReturnsSchedulesForUser(t *testing.T) {
	// arrange
	fireAt := time.Now().Add(time.Hour).Unix()
	mgr := &fakeScheduleManager{
		schedules: []*scheduler.Schedule{
			{ID: "s1", Prompt: "remind me", NextFireAt: fireAt, IntervalSeconds: 3600},
			{ID: "s2", Prompt: "one shot", NextFireAt: fireAt + 60},
		},
	}
	tl := newListRemindersTool(mgr, "user-1")

	// act
	result, err := tl.InvokableRun(context.Background(), "{}")
	require.NoError(t, err)

	// assert
	var summaries []map[string]any
	require.NoError(t, json.Unmarshal([]byte(result), &summaries))
	require.Len(t, summaries, 2)
	assert.Equal(t, "s1", summaries[0]["id"])
	assert.Equal(t, "remind me", summaries[0]["prompt"])
	assert.Equal(t, float64(3600), summaries[0]["interval_seconds"])
	assert.Equal(t, "s2", summaries[1]["id"])
	assert.Nil(t, summaries[1]["interval_seconds"])
}

func TestCancelReminderTool_CallsCancelWithCorrectArgs(t *testing.T) {
	// arrange
	mgr := &fakeScheduleManager{}
	tl := newCancelReminderTool(mgr, "user-1")

	// act
	result, err := tl.InvokableRun(context.Background(), `{"id":"sched-42"}`)
	require.NoError(t, err)

	// assert
	require.Len(t, mgr.cancelled, 1)
	assert.Equal(t, "user-1", mgr.cancelled[0].userID)
	assert.Equal(t, "sched-42", mgr.cancelled[0].id)
	assert.Contains(t, result, "sched-42")
}

func jsonInt(i int64) string {
	b, _ := json.Marshal(i)
	return string(b)
}
