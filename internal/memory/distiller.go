package memory

import (
	"context"
	"fmt"
	"strings"

	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"
)

const distillPrompt = `Extract key facts, preferences, or outcomes from this conversation worth remembering long-term.
Return one fact per line. If nothing is worth remembering, return nothing.`

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

func (d *llmDistiller) Distill(ctx context.Context, messages []*schema.Message) ([]string, error) {
	var history strings.Builder
	for _, m := range messages {
		if m.Role != schema.User {
			continue
		}
		history.WriteString(m.Content + "\n")
	}
	resp, err := d.model.Generate(ctx, []*schema.Message{
		schema.SystemMessage(distillPrompt),
		schema.UserMessage(history.String()),
	})
	if err != nil {
		return nil, fmt.Errorf("distilling history: %w", err)
	}
	var facts []string
	for _, line := range strings.Split(resp.Content, "\n") {
		if f := strings.TrimSpace(line); f != "" {
			facts = append(facts, f)
		}
	}
	return facts, nil
}
