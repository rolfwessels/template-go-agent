package memory

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rolfwessels/template-go-agent/internal/usage"
	"github.com/rolfwessels/template-go-agent/internal/usage/usagetest"
)

type fakeDistillerModel struct {
	resp *schema.Message
}

func (f *fakeDistillerModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return f.resp, nil
}

func (f *fakeDistillerModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	panic("not used in distiller")
}

func TestLLMDistiller_WithTracker_RecordsDistillerComponent(t *testing.T) {
	// arrange
	tracker := &usagetest.StubTracker{}
	fakeResp := &schema.Message{
		Role:    schema.Assistant,
		Content: "[summary] a user who codes",
		ResponseMeta: &schema.ResponseMeta{
			Usage: &schema.TokenUsage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
		},
	}
	d := &llmDistiller{
		model:     &fakeDistillerModel{resp: fakeResp},
		modelName: "gpt-test",
		tracker:   tracker,
	}
	ctx := usage.WithContext(context.Background(), "user1", "sess1")

	// act
	_, _, err := d.DistillAndSummarize(ctx, "", []*schema.Message{schema.UserMessage("hi")})

	// assert
	require.NoError(t, err)
	components := tracker.Snapshot()
	require.Len(t, components, 1)
	assert.Equal(t, "distiller", components[0])
}

func TestLLMDistiller_WithTracker_NoRecordWhenNoUsage(t *testing.T) {
	// arrange
	tracker := &usagetest.StubTracker{}
	fakeResp := &schema.Message{Role: schema.Assistant, Content: "[summary] a user"}
	d := &llmDistiller{
		model:     &fakeDistillerModel{resp: fakeResp},
		modelName: "gpt-test",
		tracker:   tracker,
	}

	// act
	_, _, err := d.DistillAndSummarize(context.Background(), "", []*schema.Message{schema.UserMessage("hi")})

	// assert
	require.NoError(t, err)
	assert.Empty(t, tracker.Snapshot())
}

func TestParseFacts_ClassifiesGeneralAndUntaggedFacts(t *testing.T) {
	// arrange
	input := "[general] user prefers dark mode\nmeeting scheduled tomorrow"

	// act
	facts, summary := parseFacts(input)

	// assert
	require.Len(t, facts, 2)
	assert.Equal(t, "user prefers dark mode", facts[0].Content)
	assert.Equal(t, KindGeneral, facts[0].Kind)
	assert.Equal(t, "meeting scheduled tomorrow", facts[1].Content)
	assert.Equal(t, KindDaily, facts[1].Kind)
	assert.Empty(t, summary)
}

func TestParseFacts_CapturesSummaryLine(t *testing.T) {
	// arrange
	input := "[general] user prefers Go\n[summary] Go developer who prefers concise tools."

	// act
	facts, summary := parseFacts(input)

	// assert
	require.Len(t, facts, 1)
	assert.Equal(t, "Go developer who prefers concise tools.", summary)
}

func TestParseFacts_TakesLastSummaryWhenMultiple(t *testing.T) {
	// arrange
	input := "[summary] first summary\n[summary] last summary"

	// act
	_, summary := parseFacts(input)

	// assert
	assert.Equal(t, "last summary", summary)
}

func TestParseFacts_DefaultsToDaily(t *testing.T) {
	// arrange
	input := "some unclassified fact"

	// act
	facts, _ := parseFacts(input)

	// assert
	require.Len(t, facts, 1)
	assert.Equal(t, KindDaily, facts[0].Kind)
	assert.Equal(t, "some unclassified fact", facts[0].Content)
}

func TestParseFacts_EmptyInputReturnsNil(t *testing.T) {
	facts, summary := parseFacts("")
	assert.Empty(t, facts)
	assert.Empty(t, summary)
}

func TestParseFacts_SkipsBlankLines(t *testing.T) {
	input := "[general] pref one\n\nevent one\n\n"
	facts, _ := parseFacts(input)
	require.Len(t, facts, 2)
}

func TestParseFacts_StripsLeadingBulletMarkers(t *testing.T) {
	tests := []struct {
		name     string
		give     string
		wantKind FactKind
		wantText string
	}{
		{name: "dash prefix on daily fact", give: "- event happened", wantKind: KindDaily, wantText: "event happened"},
		{name: "dash prefix on general fact", give: "- [general] user prefers Go", wantKind: KindGeneral, wantText: "user prefers Go"},
		{name: "asterisk prefix", give: "* event happened", wantKind: KindDaily, wantText: "event happened"},
		{name: "double dash prefix", give: "- - event happened", wantKind: KindDaily, wantText: "event happened"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts, _ := parseFacts(tt.give)
			require.Len(t, facts, 1)
			assert.Equal(t, tt.wantKind, facts[0].Kind)
			assert.Equal(t, tt.wantText, facts[0].Content)
		})
	}
}

func TestParseFacts_StripsBulletBeforeSummary(t *testing.T) {
	facts, summary := parseFacts("- [summary] one sentence summary")
	assert.Empty(t, facts)
	assert.Equal(t, "one sentence summary", summary)
}
