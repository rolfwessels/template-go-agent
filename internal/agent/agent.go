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

type generator interface {
	Generate(ctx context.Context, input []*schema.Message, opts ...agent.AgentOption) (*schema.Message, error)
}

type Agent struct {
	react        generator
	toolOpts     []agent.AgentOption
	systemPrompt string
	history      ConversationHistory
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
	userMsg := schema.UserMessage(question)
	msgs := buildMessages(a.systemPrompt, a.history.all(), userMsg)

	out, err := a.react.Generate(ctx, msgs, a.toolOpts...)
	if err != nil {
		return "", fmt.Errorf("generating response: %w", err)
	}

	a.history.append(userMsg)
	a.history.append(out)
	return out.Content, nil
}

func buildMessages(systemPrompt string, history []*schema.Message, userMsg *schema.Message) []*schema.Message {
	msgs := make([]*schema.Message, 0, 1+len(history)+1)
	msgs = append(msgs, schema.SystemMessage(systemPrompt))
	msgs = append(msgs, history...)
	msgs = append(msgs, userMsg)
	return msgs
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
