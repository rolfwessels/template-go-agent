package memory

import (
	"context"
	"fmt"
	"strings"

	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"
)

type FactKind string

const (
	KindGeneral FactKind = "general"
	KindDaily   FactKind = "daily"
)

type Fact struct {
	Content string
	Kind    FactKind
	Date    string // YYYY-MM-DD; for KindDaily only; empty defaults to today
}

const distillPrompt = `Extract key facts worth remembering long-term from this conversation.
The conversation may contain date markers such as "--- YYYY-MM-DD ---"; use the nearest marker above each fact to assign its date.

Classify each fact as:
- [general] — stable preferences, background info, or recurring behaviours that rarely change
- [daily:YYYY-MM-DD] — time-sensitive or event-specific, tied to that date

Return one fact per line. If nothing is worth remembering, return nothing.`

const summarizePrompt = `Summarize the following facts in one concise sentence of no more than 15 words.`

type Distiller interface {
	Distill(ctx context.Context, messages []*schema.Message) ([]Fact, error)
	Summarize(ctx context.Context, facts []Fact) (string, error)
}

type llmDistiller struct {
	model *einoopenai.ChatModel
}

func NewLLMDistiller(ctx context.Context, apiKey, model string) (Distiller, error) {
	m, err := einoopenai.NewChatModel(ctx, &einoopenai.ChatModelConfig{
		APIKey: apiKey,
		Model:  model,
	})
	if err != nil {
		return nil, fmt.Errorf("creating distiller model: %w", err)
	}
	return &llmDistiller{model: m}, nil
}

func (d *llmDistiller) Distill(ctx context.Context, messages []*schema.Message) ([]Fact, error) {
	var history strings.Builder
	for _, m := range messages {
		switch m.Role {
		case schema.User:
			history.WriteString(m.Content + "\n")
		case schema.Assistant:
			if isMemoryMarker(m.Content) {
				history.WriteString("Note: " + m.Content + "\n")
			}
		}
	}
	resp, err := d.model.Generate(ctx, []*schema.Message{
		schema.SystemMessage(distillPrompt),
		schema.UserMessage(history.String()),
	})
	if err != nil {
		return nil, fmt.Errorf("distilling history: %w", err)
	}
	return parseFacts(resp.Content), nil
}

func (d *llmDistiller) Summarize(ctx context.Context, facts []Fact) (string, error) {
	var sb strings.Builder
	for _, f := range facts {
		sb.WriteString("- " + f.Content + "\n")
	}
	resp, err := d.model.Generate(ctx, []*schema.Message{
		schema.SystemMessage(summarizePrompt),
		schema.UserMessage(sb.String()),
	})
	if err != nil {
		return "", fmt.Errorf("summarizing facts: %w", err)
	}
	return strings.TrimSpace(resp.Content), nil
}

func parseFacts(content string) []Fact {
	var facts []Fact
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		kind := KindDaily
		date := ""
		switch {
		case strings.HasPrefix(line, "[general]"):
			kind = KindGeneral
			line = strings.TrimSpace(strings.TrimPrefix(line, "[general]"))
		case strings.HasPrefix(line, "[daily:"):
			end := strings.Index(line, "]")
			if end > 7 {
				date = line[7:end]
			}
			line = strings.TrimSpace(line[end+1:])
		case strings.HasPrefix(line, "[daily]"):
			line = strings.TrimSpace(strings.TrimPrefix(line, "[daily]"))
		}
		if line != "" {
			facts = append(facts, Fact{Content: line, Kind: kind, Date: date})
		}
	}
	return facts
}

func isMemoryMarker(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "i'll remember") || strings.Contains(lower, "i will remember")
}
