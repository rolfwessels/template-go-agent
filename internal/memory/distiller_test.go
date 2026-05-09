package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsMemoryMarker(t *testing.T) {
	tests := []struct {
		name string
		give string
		want bool
	}{
		{name: "i'll remember lowercase", give: "i'll remember that you prefer Go", want: true},
		{name: "I'll remember mixed case", give: "I'll remember that.", want: true},
		{name: "i will remember", give: "i will remember your preference", want: true},
		{name: "I will remember mixed case", give: "I Will Remember this.", want: true},
		{name: "regular assistant reply", give: "That sounds great! Go is an excellent choice.", want: false},
		{name: "partial match not marker", give: "You'll remember to do that.", want: false},
		{name: "empty string", give: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isMemoryMarker(tt.give))
		})
	}
}

func TestParseFacts_ClassifiesGeneralAndDailyFacts(t *testing.T) {
	// arrange
	input := "[general] user prefers dark mode\n[daily] meeting scheduled tomorrow"

	// act
	facts := parseFacts(input)

	// assert
	require.Len(t, facts, 2)
	assert.Equal(t, "user prefers dark mode", facts[0].Content)
	assert.Equal(t, KindGeneral, facts[0].Kind)
	assert.Equal(t, "meeting scheduled tomorrow", facts[1].Content)
	assert.Equal(t, KindDaily, facts[1].Kind)
}

func TestParseFacts_ExtractsDateFromDailyTag(t *testing.T) {
	// arrange
	input := "[daily:2026-04-18] user added series to watchlist\n[general] user prefers Go"

	// act
	facts := parseFacts(input)

	// assert
	require.Len(t, facts, 2)
	assert.Equal(t, KindDaily, facts[0].Kind)
	assert.Equal(t, "2026-04-18", facts[0].Date)
	assert.Equal(t, "user added series to watchlist", facts[0].Content)
	assert.Equal(t, KindGeneral, facts[1].Kind)
	assert.Empty(t, facts[1].Date)
}

func TestParseFacts_DefaultsToDaily(t *testing.T) {
	// arrange
	input := "some unclassified fact"

	// act
	facts := parseFacts(input)

	// assert
	require.Len(t, facts, 1)
	assert.Equal(t, KindDaily, facts[0].Kind)
	assert.Equal(t, "some unclassified fact", facts[0].Content)
}

func TestParseFacts_EmptyInputReturnsNil(t *testing.T) {
	assert.Empty(t, parseFacts(""))
}

func TestParseFacts_SkipsBlankLines(t *testing.T) {
	input := "[general] pref one\n\n[daily] event one\n\n"
	facts := parseFacts(input)
	require.Len(t, facts, 2)
}
