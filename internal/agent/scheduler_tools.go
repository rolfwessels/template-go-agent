package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/rolfwessels/template-go-agent/internal/scheduler"
)

type ScheduleManager interface {
	Add(ctx context.Context, s *scheduler.Schedule) error
	Cancel(ctx context.Context, userID, id string) error
	ListForUser(userID string) []*scheduler.Schedule
}

// --- schedule_reminder ---

type scheduleReminderTool struct {
	sched     ScheduleManager
	userID    string
	channelID string
}

func newScheduleReminderTool(sched ScheduleManager, userID, channelID string) tool.InvokableTool {
	return &scheduleReminderTool{sched: sched, userID: userID, channelID: channelID}
}

type scheduleReminderInput struct {
	Prompt          string `json:"prompt"`
	NextFireAt      int64  `json:"next_fire_at"`
	IntervalSeconds int64  `json:"interval_seconds,omitempty"`
}

func (t *scheduleReminderTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "schedule_reminder",
		Desc: "Schedule a reminder to be delivered at a future time. Use get_current_time and date_math to compute next_fire_at.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"prompt": {
				Type:     schema.String,
				Desc:     "The message or question to deliver when the reminder fires.",
				Required: true,
			},
			"next_fire_at": {
				Type:     schema.Integer,
				Desc:     "Unix timestamp (seconds) for when the reminder should fire.",
				Required: true,
			},
			"interval_seconds": {
				Type: schema.Integer,
				Desc: "If set and non-zero, the reminder repeats every interval_seconds. Omit for a one-shot reminder.",
			},
		}),
	}, nil
}

func (t *scheduleReminderTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var input scheduleReminderInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return "", fmt.Errorf("parsing arguments: %w", err)
	}
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	sc := &scheduler.Schedule{
		ID:              id,
		UserID:          t.userID,
		ChannelID:       t.channelID,
		Prompt:          input.Prompt,
		NextFireAt:      input.NextFireAt,
		IntervalSeconds: input.IntervalSeconds,
	}
	if err := t.sched.Add(ctx, sc); err != nil {
		return "", fmt.Errorf("adding schedule: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": id})
	return string(b), nil
}

// --- list_reminders ---

type listRemindersTool struct {
	sched  ScheduleManager
	userID string
}

func newListRemindersTool(sched ScheduleManager, userID string) tool.InvokableTool {
	return &listRemindersTool{sched: sched, userID: userID}
}

type reminderSummary struct {
	ID              string `json:"id"`
	Prompt          string `json:"prompt"`
	NextFireAt      int64  `json:"next_fire_at"`
	IntervalSeconds int64  `json:"interval_seconds,omitempty"`
}

func (t *listRemindersTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name:        "list_reminders",
		Desc:        "List all active scheduled reminders for the current user.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{}),
	}, nil
}

func (t *listRemindersTool) InvokableRun(_ context.Context, _ string, _ ...tool.Option) (string, error) {
	schedules := t.sched.ListForUser(t.userID)
	summaries := make([]reminderSummary, len(schedules))
	for i, s := range schedules {
		summaries[i] = reminderSummary{
			ID:              s.ID,
			Prompt:          s.Prompt,
			NextFireAt:      s.NextFireAt,
			IntervalSeconds: s.IntervalSeconds,
		}
	}
	b, err := json.Marshal(summaries)
	if err != nil {
		return "", fmt.Errorf("encoding reminders: %w", err)
	}
	return string(b), nil
}

// --- cancel_reminder ---

type cancelReminderTool struct {
	sched  ScheduleManager
	userID string
}

func newCancelReminderTool(sched ScheduleManager, userID string) tool.InvokableTool {
	return &cancelReminderTool{sched: sched, userID: userID}
}

type cancelReminderInput struct {
	ID string `json:"id"`
}

func (t *cancelReminderTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "cancel_reminder",
		Desc: "Cancel a scheduled reminder by its ID.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"id": {
				Type:     schema.String,
				Desc:     "The ID of the reminder to cancel.",
				Required: true,
			},
		}),
	}, nil
}

func (t *cancelReminderTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var input cancelReminderInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return "", fmt.Errorf("parsing arguments: %w", err)
	}
	if err := t.sched.Cancel(ctx, t.userID, input.ID); err != nil {
		return "", fmt.Errorf("cancelling schedule: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"cancelled": input.ID})
	return string(b), nil
}
