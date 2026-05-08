package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type currentTimeTool struct {
	now func() time.Time
}

func newCurrentTimeTool() tool.InvokableTool {
	return &currentTimeTool{now: time.Now}
}

type timeInput struct {
	Timezone string `json:"timezone"`
}

type timeOutput struct {
	ISO      string `json:"iso"`
	UnixMS   int64  `json:"unix_ms"`
	Timezone string `json:"timezone"`
	Offset   string `json:"offset"`
}

func (t *currentTimeTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "get_current_time",
		Desc: "Returns the current date and time. Use this whenever the user asks about the current time, date, or 'today'.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"timezone": {
				Type: schema.String,
				Desc: "IANA timezone name (e.g. 'America/New_York', 'Africa/Johannesburg'). Defaults to UTC.",
			},
		}),
	}, nil
}

func (t *currentTimeTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var input timeInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return "", fmt.Errorf("parsing arguments: %w", err)
	}

	loc, tz, err := resolveLocation(input.Timezone)
	if err != nil {
		return "", err
	}

	now := t.now().In(loc)
	out := timeOutput{
		ISO:      now.Format(time.RFC3339),
		UnixMS:   now.UnixMilli(),
		Timezone: tz,
		Offset:   formatOffset(now),
	}

	b, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("encoding result: %w", err)
	}
	return string(b), nil
}

func resolveLocation(tz string) (*time.Location, string, error) {
	if tz == "" {
		return time.UTC, "UTC", nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, "", fmt.Errorf("unknown timezone %q: %w", tz, err)
	}
	return loc, tz, nil
}

func formatOffset(t time.Time) string {
	_, offset := t.Zone()
	h, m := offset/3600, (offset%3600)/60
	if offset < 0 {
		return fmt.Sprintf("-%02d:%02d", -h, -m)
	}
	return fmt.Sprintf("+%02d:%02d", h, m)
}
