package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	einoagent "github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"github.com/rolfwessels/template-go-agent/internal/config"
	"github.com/rolfwessels/template-go-agent/internal/memory"
)

type msgGenerator interface {
	generate(ctx context.Context, input []*schema.Message) ([]*schema.Message, error)
}

type Generator interface {
	Generate(ctx context.Context, input []*schema.Message, opts ...einoagent.AgentOption) (*schema.Message, error)
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

func WithExtraInstructions(s string) Option {
	return func(a *Agent) { a.extraInstructions = s }
}

type schedulerBinding struct {
	manager   ScheduleManager
	userID    string
	channelID string
}

func WithScheduler(manager ScheduleManager, userID, channelID string) Option {
	return func(a *Agent) {
		a.schedulerBinding = &schedulerBinding{manager: manager, userID: userID, channelID: channelID}
	}
}

type Agent struct {
	react             msgGenerator
	systemPrompt      string
	memoryContext     string
	extraInstructions string
	history           ConversationHistory
	resetCallback     func(ctx context.Context) error
	schedulerBinding  *schedulerBinding
}

func NewWithGenerator(gen Generator, systemPrompt string, opts ...Option) *Agent {
	a := &Agent{react: &singleMsgGenerator{gen: gen}, systemPrompt: systemPrompt}
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

	tools := []tool.BaseTool{newTavilyTool(cfg.TavilyAPIKey), newCurrentTimeTool(), newDateMathTool(), newHTTPFetchTool(), newCalculatorTool()}
	if a.resetCallback != nil {
		tools = append(tools, newNewSessionTool(a.resetCallback))
	}
	if sb := a.schedulerBinding; sb != nil {
		tools = append(tools,
			newScheduleReminderTool(sb.manager, sb.userID, sb.channelID),
			newListRemindersTool(sb.manager, sb.userID),
			newCancelReminderTool(sb.manager, sb.userID),
		)
	}

	ra, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: model,
		ToolsConfig:      compose.ToolsNodeConfig{Tools: tools},
	})
	if err != nil {
		return nil, fmt.Errorf("creating react agent: %w", err)
	}

	systemPrompt, err := loadPrompts("prompts/soul.md", "prompts/instructions.md")
	if err != nil {
		return nil, err
	}
	systemPrompt += "\n\n" + memory.Instructions()
	if a.extraInstructions != "" {
		systemPrompt += "\n\n" + a.extraInstructions
	}

	a.react = &reactMsgGenerator{agent: ra}
	a.systemPrompt = systemPrompt
	return a, nil
}

func (a *Agent) Generate(ctx context.Context, question string) (string, error) {
	userMsg := schema.UserMessage(question)
	msgs := buildMessages(a.systemPrompt, a.memoryContext, a.history.all(), userMsg)

	produced, err := a.react.generate(ctx, msgs)
	if err != nil {
		return "", fmt.Errorf("generating response: %w", err)
	}

	a.history.append(userMsg)
	for _, msg := range produced {
		a.history.append(msg)
	}
	return produced[len(produced)-1].Content, nil
}

type reactMsgGenerator struct {
	agent *react.Agent
}

func (r *reactMsgGenerator) generate(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
	msgFutureOpt, msgFuture := react.WithMessageFuture()
	_, err := r.agent.Generate(ctx, input, msgFutureOpt)
	if err != nil {
		return nil, err
	}
	var msgs []*schema.Message
	iter := msgFuture.GetMessages()
	for msg, ok, iterErr := iter.Next(); ok; msg, ok, iterErr = iter.Next() {
		if iterErr != nil {
			return nil, iterErr
		}
		for _, tc := range msg.ToolCalls {
			slog.Info("tool call", "tool", tc.Function.Name, "args", tc.Function.Arguments)
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

type singleMsgGenerator struct {
	gen Generator
}

func (s *singleMsgGenerator) generate(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
	msg, err := s.gen.Generate(ctx, input)
	if err != nil {
		return nil, err
	}
	return []*schema.Message{msg}, nil
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
