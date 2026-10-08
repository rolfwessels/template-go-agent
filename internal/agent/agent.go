package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	einoagent "github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"github.com/rolfwessels/template-go-agent/internal/config"
	"github.com/rolfwessels/template-go-agent/internal/memory"
	"github.com/rolfwessels/template-go-agent/internal/usage"
	"github.com/rolfwessels/template-go-agent/prompts"
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
	return func(a *Agent) { a.history = append(a.history, msgs...) }
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

func WithMemoryDir(dir string) Option {
	return func(a *Agent) { a.memoryDir = dir }
}

type trackerBinding struct {
	tracker   usage.Tracker
	userID    string
	sessionID string
	model     string
}

func WithUsageTracker(tracker usage.Tracker, userID, sessionID string) Option {
	return func(a *Agent) {
		a.trackerBinding = &trackerBinding{tracker: tracker, userID: userID, sessionID: sessionID}
	}
}

type Agent struct {
	react             msgGenerator
	systemPrompt      string
	memoryContext     string
	extraInstructions string
	memoryDir         string
	history           []*schema.Message
	resetCallback     func(ctx context.Context) error
	schedulerBinding  *schedulerBinding
	trackerBinding    *trackerBinding
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

	systemPrompt, err := loadPrompts(cfg.PromptsDir)
	if err != nil {
		return nil, err
	}

	model, err := einoopenai.NewChatModel(ctx, &einoopenai.ChatModelConfig{
		APIKey: cfg.OpenAIAPIKey,
		Model:  cfg.OpenAIModel,
		// Newer reasoning models reject function tools on /v1/chat/completions unless effort is "none".
		ReasoningEffort: einoopenai.ReasoningEffortLevel(cfg.OpenAIReasoningEffort),
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
	if a.memoryDir != "" {
		tools = append(tools, newReadMemoryFileTool(a.memoryDir), newSearchMemoryTool(a.memoryDir))
	}

	ra, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: model,
		ToolsConfig:      compose.ToolsNodeConfig{Tools: tools},
	})
	if err != nil {
		return nil, fmt.Errorf("creating react agent: %w", err)
	}

	systemPrompt += "\n\n" + memory.Instructions()
	if a.extraInstructions != "" {
		systemPrompt += "\n\n" + a.extraInstructions
	}

	if a.trackerBinding != nil {
		a.trackerBinding.model = cfg.OpenAIModel
	}
	a.react = &reactMsgGenerator{agent: ra}
	a.systemPrompt = systemPrompt
	return a, nil
}

func (a *Agent) Generate(ctx context.Context, question string) (string, error) {
	userMsg := schema.UserMessage(question)
	msgs := buildMessages(a.systemPrompt, a.memoryContext, a.history, userMsg)

	produced, err := a.react.generate(ctx, msgs)
	if err != nil {
		return "", fmt.Errorf("generating response: %w", err)
	}

	a.recordUsage(produced)
	a.history = append(a.history, userMsg)
	a.history = append(a.history, produced...)
	return produced[len(produced)-1].Content, nil
}

func (a *Agent) recordUsage(msgs []*schema.Message) {
	if a.trackerBinding == nil {
		return
	}
	for _, msg := range msgs {
		if msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
			continue
		}
		u := msg.ResponseMeta.Usage
		a.trackerBinding.tracker.Record(a.trackerBinding.userID, a.trackerBinding.sessionID, usage.ComponentAgent, a.trackerBinding.model, usage.TokenUsage{
			PromptTokens:     u.PromptTokens,
			CompletionTokens: u.CompletionTokens,
			TotalTokens:      u.TotalTokens,
		})
	}
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

func loadPrompts(dir string) (string, error) {
	if dir == "" {
		return prompts.Defaults(), nil
	}

	soul, soulErr := os.ReadFile(filepath.Join(dir, "soul.md"))
	instructions, instructionsErr := os.ReadFile(filepath.Join(dir, "instructions.md"))
	if os.IsNotExist(soulErr) && os.IsNotExist(instructionsErr) {
		return prompts.Defaults(), nil
	}
	if soulErr != nil {
		return "", fmt.Errorf("loading soul prompt from PROMPTS_DIR %q (both soul.md and instructions.md are required): %w", dir, soulErr)
	}
	if instructionsErr != nil {
		return "", fmt.Errorf("loading instructions prompt from PROMPTS_DIR %q (both soul.md and instructions.md are required): %w", dir, instructionsErr)
	}
	return string(soul) + "\n\n" + string(instructions), nil
}
