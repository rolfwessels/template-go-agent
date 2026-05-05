package agent

import (
	"context"
	"fmt"
	"os"

	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"github.com/rolfwessels/template-go-agent/internal/config"
)

type Agent struct {
	react        *react.Agent
	toolOpts     []agent.AgentOption
	systemPrompt string
}

func New(ctx context.Context, cfg *config.Config) (*Agent, error) {
	model, err := einoopenai.NewChatModel(ctx, &einoopenai.ChatModelConfig{
		APIKey: cfg.OpenAIAPIKey,
		Model:  cfg.OpenAIModel,
	})
	if err != nil {
		return nil, fmt.Errorf("creating chat model: %w", err)
	}

	tavily := newTavilyTool(cfg.TavilyAPIKey)

	toolOpts, err := react.WithTools(ctx, tavily)
	if err != nil {
		return nil, fmt.Errorf("configuring tools: %w", err)
	}

	ra, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: model,
	})
	if err != nil {
		return nil, fmt.Errorf("creating react agent: %w", err)
	}

	systemPrompt, err := loadPrompts("prompts/soul.md", "prompts/instructions.md")
	if err != nil {
		return nil, err
	}

	return &Agent{react: ra, toolOpts: toolOpts, systemPrompt: systemPrompt}, nil
}

func (a *Agent) Generate(ctx context.Context, question string) (string, error) {
	msgs := []*schema.Message{
		schema.SystemMessage(a.systemPrompt),
		schema.UserMessage(question),
	}
	out, err := a.react.Generate(ctx, msgs, a.toolOpts...)
	if err != nil {
		return "", fmt.Errorf("generating response: %w", err)
	}
	return out.Content, nil
}

func loadPrompts(soulPath, instructionsPath string) (string, error) {
	soul, err := os.ReadFile(soulPath)
	if err != nil {
		return "", fmt.Errorf("loading soul prompt: %w", err)
	}
	instructions, err := os.ReadFile(instructionsPath)
	if err != nil {
		return "", fmt.Errorf("loading instructions prompt: %w", err)
	}
	return string(soul) + "\n\n" + string(instructions), nil
}
