package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
