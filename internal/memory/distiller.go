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
}

const distillAndSummarizePrompt = `You are a memory distiller. You receive the existing content of a daily memory file for a specific date, followed by a conversation from that date where each line is prefixed with "User:" or "Assistant:".

Extract new facts about the user not already present in the existing daily memory. Base facts only on what the user said or confirmed — do not re-extract facts the assistant merely echoed or acknowledged. Classify each as:
- [general] — stable preferences, background info, or recurring behaviours that rarely change
- untagged (no prefix) — events or activities specific to this date

Return one fact per line. Do not re-extract facts already recorded.

End your response with exactly one line:
[summary] <one sentence covering all facts in the daily file, both existing and new combined>`

type Distiller interface {
	DistillAndSummarize(ctx context.Context, existingDaily string, messages []*schema.Message) ([]Fact, string, error)
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
	facts, summary := parseFacts(resp.Content)
	return facts, summary, nil
}

func parseFacts(content string) ([]Fact, string) {
	var facts []Fact
	summary := ""
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
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

