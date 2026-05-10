package usage_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rolfwessels/template-go-agent/internal/usage"
)

func TestCostUSD_KnownModels(t *testing.T) {
	tests := []struct {
		model            string
		prompt, complete int
		want             float64
	}{
		{"gpt-5.5", 1_000_000, 0, 5.00},
		{"gpt-5.5", 0, 1_000_000, 30.00},
		{"gpt-5.4-mini", 1_000_000, 0, 0.75},
		{"gpt-5.4-nano", 1_000_000, 0, 0.20},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			assert.InDelta(t, tt.want, usage.CostUSD(tt.model, tt.prompt, tt.complete), 1e-9)
		})
	}
}

func TestCostUSD_UnknownModel(t *testing.T) {
	assert.Equal(t, 0.0, usage.CostUSD("unknown-model-xyz-unique", 1000, 1000))
}

func TestFileTracker_SingleEntry(t *testing.T) {
	// arrange
	dir := t.TempDir()
	tracker := usage.NewFileTracker(dir, usage.NewCounter())

	// act
	tracker.Record("user1", "sess1", "agent", "gpt-5.5", usage.TokenUsage{
		PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150,
	})

	// assert
	ledger := filepath.Join(dir, "user", "user1", "metrics", "costs.jsonl")
	require.FileExists(t, ledger)
	lines := readLines(t, ledger)
	require.Len(t, lines, 1)

	var r map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &r))
	assert.Equal(t, "agent", r["component"])
	assert.Equal(t, "gpt-5.5", r["model"])
	assert.Equal(t, "sess1", r["session_id"])
	assert.NotEmpty(t, r["timestamp"])
	assert.Equal(t, float64(100), r["prompt_tokens"])
	assert.Equal(t, float64(50), r["completion_tokens"])
	assert.Equal(t, float64(150), r["total_tokens"])
	assert.Greater(t, r["cost_usd"], 0.0)
}

func TestFileTracker_MultipleEntries(t *testing.T) {
	// arrange
	dir := t.TempDir()
	counter := usage.NewCounter()
	tracker := usage.NewFileTracker(dir, counter)

	// act
	tracker.Record("user1", "sess1", "agent", "gpt-5.5", usage.TokenUsage{
		PromptTokens: 1000, CompletionTokens: 500, TotalTokens: 1500,
	})
	tracker.Record("user1", "sess1", "distiller", "gpt-5-mini", usage.TokenUsage{
		PromptTokens: 2000, CompletionTokens: 1000, TotalTokens: 3000,
	})

	// assert
	ledger := filepath.Join(dir, "user", "user1", "metrics", "costs.jsonl")
	require.Len(t, readLines(t, ledger), 2)
	assert.Equal(t, "Tokens: 4,500 (3,000 in / 1,500 out) | Cost: $0.0225", counter.StatusLine())
}

func TestCounter_StatusLine_Format(t *testing.T) {
	// arrange
	dir := t.TempDir()
	counter := usage.NewCounter()
	tracker := usage.NewFileTracker(dir, counter)

	// act — 9,800 prompt + 2,650 completion = 12,450 total
	tracker.Record("u", "s", "agent", "gpt-5-mini", usage.TokenUsage{
		PromptTokens: 9800, CompletionTokens: 2650, TotalTokens: 12450,
	})

	// assert — verify comma-formatting in the status line
	line := counter.StatusLine()
	assert.Contains(t, line, "Tokens: 12,450 (9,800 in / 2,650 out)")
	assert.Contains(t, line, "| Cost: $")
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		if line := scanner.Text(); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
