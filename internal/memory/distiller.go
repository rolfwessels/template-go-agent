package memory

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/rolfwessels/template-go-agent/internal/usage"
)

type FactKind string

const (
	KindGeneral FactKind = "general"
	KindDaily   FactKind = "daily"
)

type Fact struct {
	Content string
	Kind    FactKind
}

//go:embed distiller_prompt.md
var distillAndSummarizePrompt string

type Distiller interface {
	DistillAndSummarize(ctx context.Context, existingDaily string, messages []*schema.Message) ([]Fact, string, error)
}

type llmDistiller struct {
	model     model.BaseChatModel
	modelName string
	tracker   usage.Tracker
}

type DistillerOption func(*llmDistiller)

func WithDistillerTracker(tracker usage.Tracker) DistillerOption {
	return func(d *llmDistiller) { d.tracker = tracker }
}

func NewLLMDistiller(ctx context.Context, apiKey, modelName string, opts ...DistillerOption) (Distiller, error) {
	m, err := einoopenai.NewChatModel(ctx, &einoopenai.ChatModelConfig{
		APIKey: apiKey,
		Model:  modelName,
	})
	if err != nil {
		return nil, fmt.Errorf("creating distiller model: %w", err)
	}
	d := &llmDistiller{model: m, modelName: modelName}
	for _, opt := range opts {
		opt(d)
	}
	return d, nil
}

func (d *llmDistiller) DistillAndSummarize(ctx context.Context, existingDaily string, messages []*schema.Message) ([]Fact, string, error) {
	var sb strings.Builder
	if existingDaily != "" {
		sb.WriteString("## Existing daily memory\n")
		sb.WriteString(existingDaily)
		sb.WriteString("\n## New messages\n")
	}
	for _, m := range messages {
		switch m.Role {
		case schema.User:
			sb.WriteString("User: " + m.Content + "\n")
		case schema.Assistant:
			sb.WriteString("Assistant: " + m.Content + "\n")
		}
	}
	resp, err := d.model.Generate(ctx, []*schema.Message{
		schema.SystemMessage(distillAndSummarizePrompt),
		schema.UserMessage(sb.String()),
	})
	if err != nil {
		return nil, "", fmt.Errorf("distilling history: %w", err)
	}
	d.recordUsage(ctx, resp)
	facts, summary := parseFacts(resp.Content)
	return facts, summary, nil
}

func (d *llmDistiller) recordUsage(ctx context.Context, resp *schema.Message) {
	if d.tracker == nil || resp.ResponseMeta == nil || resp.ResponseMeta.Usage == nil {
		return
	}
	userID, sessionID := usage.FromContext(ctx)
	u := resp.ResponseMeta.Usage
	d.tracker.Record(userID, sessionID, usage.ComponentDistiller, d.modelName, usage.TokenUsage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	})
}

func parseFacts(content string) ([]Fact, string) {
	var facts []Fact
	summary := ""
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = stripBulletPrefix(line)
		if strings.HasPrefix(line, "[summary]") {
			summary = strings.TrimSpace(strings.TrimPrefix(line, "[summary]"))
			continue
		}
		kind := KindDaily
		if strings.HasPrefix(line, "[general]") {
			kind = KindGeneral
			line = strings.TrimSpace(strings.TrimPrefix(line, "[general]"))
		}
		if line != "" {
			facts = append(facts, Fact{Content: line, Kind: kind})
		}
	}
	return facts, summary
}

func stripBulletPrefix(line string) string {
	for {
		switch {
		case strings.HasPrefix(line, "- "):
			line = strings.TrimSpace(line[2:])
		case strings.HasPrefix(line, "* "):
			line = strings.TrimSpace(line[2:])
		default:
			return line
		}
	}
}

