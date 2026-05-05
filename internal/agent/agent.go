package agent

import (
	"context"
	"fmt"

	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"github.com/rolfwessels/template-go-agent/internal/config"
)

type Agent struct {
	react    *react.Agent
	toolOpts []agent.AgentOption
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

	return &Agent{react: ra, toolOpts: toolOpts}, nil
}

func (a *Agent) Generate(ctx context.Context, question string) (string, error) {
	msg := schema.UserMessage(question)
	out, err := a.react.Generate(ctx, []*schema.Message{msg}, a.toolOpts...)
	if err != nil {
		return "", fmt.Errorf("generating response: %w", err)
	}
	return out.Content, nil
}
