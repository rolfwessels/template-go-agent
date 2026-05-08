package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type dateMathTool struct{}

func newDateMathTool() tool.InvokableTool {
	return &dateMathTool{}
}

type dateMathInput struct {
	Operation string `json:"operation"`
	Date      string `json:"date"`
	Amount    int    `json:"amount"`
	Unit      string `json:"unit"`
	From      string `json:"from"`
	To        string `json:"to"`
}

type diffOutput struct {
	Days    int    `json:"days"`
	Hours   int    `json:"hours"`
	Minutes int    `json:"minutes"`
	Seconds int    `json:"seconds"`
	Human   string `json:"human"`
}

func (d *dateMathTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "date_math",
		Desc: `Perform date arithmetic: add or subtract a duration from a date ("add"), or find the difference between two dates ("diff").`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"operation": {
				Type:     schema.String,
				Desc:     `"add" to shift a date by a duration, "diff" to find the gap between two dates`,
				Required: true,
			},
			"date": {
				Type: schema.String,
				Desc: `ISO 8601 date/datetime (e.g. "2026-05-08" or "2026-05-08T10:00:00Z"). Required for "add".`,
			},
			"amount": {
				Type: schema.Integer,
				Desc: `Number of units to add; use a negative value to subtract. Required for "add".`,
			},
			"unit": {
				Type: schema.String,
				Desc: `One of: seconds, minutes, hours, days, weeks, months, years. Required for "add".`,
			},
			"from": {
				Type: schema.String,
				Desc: `Start date (ISO 8601). Required for "diff".`,
			},
			"to": {
				Type: schema.String,
				Desc: `End date (ISO 8601). Required for "diff".`,
			},
		}),
	}, nil
}

func (d *dateMathTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var input dateMathInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return "", fmt.Errorf("parsing arguments: %w", err)
	}
	switch input.Operation {
	case "add":
		return d.add(input)
	case "diff":
		return d.diff(input)
	default:
		return "", fmt.Errorf("unknown operation %q: use 'add' or 'diff'", input.Operation)
	}
}

func (d *dateMathTool) add(input dateMathInput) (string, error) {
	t, err := parseFlexTime(input.Date)
	if err != nil {
		return "", fmt.Errorf("parsing date: %w", err)
	}
	var result time.Time
	switch input.Unit {
	case "seconds":
		result = t.Add(time.Duration(input.Amount) * time.Second)
	case "minutes":
		result = t.Add(time.Duration(input.Amount) * time.Minute)
	case "hours":
		result = t.Add(time.Duration(input.Amount) * time.Hour)
	case "days":
		result = t.AddDate(0, 0, input.Amount)
	case "weeks":
		result = t.AddDate(0, 0, input.Amount*7)
	case "months":
		result = t.AddDate(0, input.Amount, 0)
	case "years":
		result = t.AddDate(input.Amount, 0, 0)
	default:
		return "", fmt.Errorf("unknown unit %q: use seconds, minutes, hours, days, weeks, months, or years", input.Unit)
	}
	return result.Format(time.RFC3339), nil
}

func (d *dateMathTool) diff(input dateMathInput) (string, error) {
	from, err := parseFlexTime(input.From)
	if err != nil {
		return "", fmt.Errorf("parsing 'from': %w", err)
	}
	to, err := parseFlexTime(input.To)
	if err != nil {
		return "", fmt.Errorf("parsing 'to': %w", err)
	}

	dur := to.Sub(from)
	totalSecs := int(math.Abs(dur.Seconds()))
	days := totalSecs / 86400
	hours := (totalSecs % 86400) / 3600
	minutes := (totalSecs % 3600) / 60
	seconds := totalSecs % 60

	out := diffOutput{
		Days:    days,
		Hours:   hours,
		Minutes: minutes,
		Seconds: seconds,
		Human:   formatDiff(days, hours, minutes, seconds, dur < 0),
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("encoding result: %w", err)
	}
	return string(b), nil
}

func formatDiff(days, hours, minutes, seconds int, negative bool) string {
	parts := []string{}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%d day%s", days, plural(days)))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%d hour%s", hours, plural(hours)))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%d minute%s", minutes, plural(minutes)))
	}
	if seconds > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%d second%s", seconds, plural(seconds)))
	}

	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	if negative {
		result += " ago"
	}
	return result
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func parseFlexTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", s)
}
