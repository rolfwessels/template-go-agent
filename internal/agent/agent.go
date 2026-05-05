package agent

import (
	"context"
	"fmt"
	"os"

	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"github.com/rolfwessels/template-go-agent/internal/config"
)

type generator = Generator

type Generator interface {
	Generate(ctx context.Context, input []*schema.Message, opts ...agent.AgentOption) (*schema.Message, error)
}

type Option func(*Agent)

func WithMemoryContext(ctx string) Option {
	return func(a *Agent) { a.memoryContext = ctx }
}

func WithInitialHistory(msgs []*schema.Message) Option {
	return func(a *Agent) { a.history.log = append(a.history.log, msgs...) }
}

func WithResetCallback(cb func(ctx context.Context) error) Option {
	return func(a *Agent) { a.resetCallback = cb }
}

type Agent struct {
	react         generator
	toolOpts      []agent.AgentOption
	systemPrompt  string
	memoryContext string
	history       ConversationHistory
	resetCallback func(ctx context.Context) error
}

func NewWithGenerator(gen Generator, systemPrompt string, opts ...Option) *Agent {
	a := &Agent{react: gen, systemPrompt: systemPrompt}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

func New(ctx context.Context, cfg *config.Config, opts ...Option) (*Agent, error) {
	a := &Agent{}
	for _, opt := range opts {
		opt(a)
	}

	model, err := einoopenai.NewChatModel(ctx, &einoopenai.ChatModelConfig{
		APIKey: cfg.OpenAIAPIKey,
		Model:  cfg.OpenAIModel,
	})
	if err != nil {
		return nil, fmt.Errorf("creating chat model: %w", err)
	}

	tools := []tool.BaseTool{newTavilyTool(cfg.TavilyAPIKey)}
	if a.resetCallback != nil {
		tools = append(tools, newNewSessionTool(a.resetCallback))
	}

	toolOpts, err := react.WithTools(ctx, tools...)
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

	a.react = ra
	a.toolOpts = toolOpts
	a.systemPrompt = systemPrompt
	return a, nil
}

func (a *Agent) Generate(ctx context.Context, question string) (string, error) {
	userMsg := schema.UserMessage(question)
	msgs := buildMessages(a.systemPrompt, a.memoryContext, a.history.all(), userMsg)

	out, err := a.react.Generate(ctx, msgs, a.toolOpts...)
	if err != nil {
		return "", fmt.Errorf("generating response: %w", err)
	}

	a.history.append(userMsg)
	a.history.append(out)
	return out.Content, nil
}

func buildMessages(systemPrompt, memoryContext string, history []*schema.Message, userMsg *schema.Message) []*schema.Message {
	prompt := systemPrompt
	if memoryContext != "" {
		prompt += "\n\n## Recalled memories\n" + memoryContext
	}
	msgs := make([]*schema.Message, 0, 1+len(history)+1)
	msgs = append(msgs, schema.SystemMessage(prompt))
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
