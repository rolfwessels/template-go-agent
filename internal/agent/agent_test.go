package agent

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rolfwessels/template-go-agent/internal/usage/usagetest"
)

type fakeGenerator struct {
	msgs       []*schema.Message
	onGenerate func([]*schema.Message)
}

func (f *fakeGenerator) generate(_ context.Context, input []*schema.Message) ([]*schema.Message, error) {
	if f.onGenerate != nil {
		f.onGenerate(input)
	}
	return f.msgs, nil
}

func newFakeGenerator(response string) *fakeGenerator {
	return &fakeGenerator{msgs: []*schema.Message{{Role: schema.Assistant, Content: response}}}
}

func TestGenerate_MemoryContextInSystemPrompt(t *testing.T) {
	// arrange
	var capturedSystem string
	gen := &fakeGenerator{
		msgs: []*schema.Message{{Role: schema.Assistant, Content: "ok"}},
		onGenerate: func(msgs []*schema.Message) {
			capturedSystem = msgs[0].Content
		},
	}
	a := &Agent{react: gen, systemPrompt: "sys", memoryContext: "user likes Go"}

	// act
	_, err := a.Generate(context.Background(), "hello")

	// assert
	require.NoError(t, err)
	assert.Contains(t, capturedSystem, "sys")
	assert.Contains(t, capturedSystem, "user likes Go")
}

func TestGenerate_PassesFullHistory(t *testing.T) {
	tests := []struct {
		name             string
		giveQuestions    []string
		wantLastMsgCount int
	}{
		{"single message", []string{"q1"}, 2},             // system + user
		{"two messages", []string{"q1", "q2"}, 4},         // system + user + assistant + user
		{"three messages", []string{"q1", "q2", "q3"}, 6}, // system + user + assistant + user + assistant + user
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// arrange
			var lastInput []*schema.Message
			gen := &fakeGenerator{
				msgs: []*schema.Message{{Role: schema.Assistant, Content: "ok"}},
				onGenerate: func(msgs []*schema.Message) {
					lastInput = msgs
				},
			}
			a := &Agent{react: gen, systemPrompt: "sys"}

			// act
			for _, q := range tt.giveQuestions {
				_, err := a.Generate(context.Background(), q)
				require.NoError(t, err)
			}

			// assert
			assert.Len(t, lastInput, tt.wantLastMsgCount)
		})
	}
}

func TestGenerate_AllProducedMessagesStoredInHistory(t *testing.T) {
	// arrange
	toolCallMsg := &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: "c1"}}}
	toolResultMsg := &schema.Message{Role: schema.Tool, ToolCallID: "c1", Content: "result"}
	finalMsg := &schema.Message{Role: schema.Assistant, Content: "done"}

	gen := &fakeGenerator{msgs: []*schema.Message{toolCallMsg, toolResultMsg, finalMsg}}
	a := &Agent{react: gen, systemPrompt: "sys"}

	// act
	reply, err := a.Generate(context.Background(), "use a tool")

	// assert
	require.NoError(t, err)
	assert.Equal(t, "done", reply)
	history := a.history
	require.Len(t, history, 4) // user + tool-call + tool-result + final
	assert.Equal(t, schema.User, history[0].Role)
	assert.Equal(t, schema.Assistant, history[1].Role)
	assert.Equal(t, schema.Tool, history[2].Role)
	assert.Equal(t, schema.Assistant, history[3].Role)
	assert.Equal(t, "done", history[3].Content)
}

func TestGenerate_WithUsageTracker_RecordsAgentComponent(t *testing.T) {
	// arrange
	tracker := &usagetest.StubTracker{}
	gen := &fakeGenerator{msgs: []*schema.Message{{
		Role:    schema.Assistant,
		Content: "hello",
		ResponseMeta: &schema.ResponseMeta{
			Usage: &schema.TokenUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		},
	}}}
	a := &Agent{
		react:        gen,
		systemPrompt: "sys",
		trackerBinding: &trackerBinding{
			tracker:   tracker,
			userID:    "user1",
			sessionID: "sess1",
			model:     "gpt-test",
		},
	}

	// act
	_, err := a.Generate(context.Background(), "hello")

	// assert
	require.NoError(t, err)
	components := tracker.Snapshot()
	require.Len(t, components, 1)
	assert.Equal(t, "agent", components[0])
}

func TestGenerate_WithUsageTracker_NoRecordWhenNoUsage(t *testing.T) {
	// arrange
	tracker := &usagetest.StubTracker{}
	gen := newFakeGenerator("hello")
	a := &Agent{
		react:        gen,
		systemPrompt: "sys",
		trackerBinding: &trackerBinding{
			tracker:   tracker,
			userID:    "user1",
			sessionID: "sess1",
		},
	}

	// act
	_, err := a.Generate(context.Background(), "hello")

	// assert
	require.NoError(t, err)
	assert.Empty(t, tracker.Snapshot())
}
