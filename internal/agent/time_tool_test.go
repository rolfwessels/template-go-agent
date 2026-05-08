package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var fixedTime = time.Date(2026, 5, 8, 10, 59, 23, 0, time.UTC)

func newFixedTimeTool() *currentTimeTool {
	return &currentTimeTool{now: func() time.Time { return fixedTime }}
}

func TestCurrentTimeTool_UTC(t *testing.T) {
	// arrange
	tool := newFixedTimeTool()

	// act
	result, err := tool.InvokableRun(context.Background(), `{}`)

	// assert
	require.NoError(t, err)
	var out timeOutput
	require.NoError(t, json.Unmarshal([]byte(result), &out))
	assert.Equal(t, "2026-05-08T10:59:23Z", out.ISO)
	assert.Equal(t, fixedTime.UnixMilli(), out.UnixMS)
	assert.Equal(t, "UTC", out.Timezone)
	assert.Equal(t, "+00:00", out.Offset)
}

func TestCurrentTimeTool_WithTimezone(t *testing.T) {
	// arrange
	tool := newFixedTimeTool()

	// act
	result, err := tool.InvokableRun(context.Background(), `{"timezone":"Africa/Johannesburg"}`)

	// assert
	require.NoError(t, err)
	var out timeOutput
	require.NoError(t, json.Unmarshal([]byte(result), &out))
	assert.Equal(t, "Africa/Johannesburg", out.Timezone)
	assert.Equal(t, "+02:00", out.Offset)
	assert.Contains(t, out.ISO, "2026-05-08T12:59:23")
}

func TestCurrentTimeTool_InvalidTimezone(t *testing.T) {
	// arrange
	tool := newFixedTimeTool()

	// act
	_, err := tool.InvokableRun(context.Background(), `{"timezone":"Not/Real"}`)

	// assert
	assert.ErrorContains(t, err, "unknown timezone")
}

func TestCurrentTimeTool_Info(t *testing.T) {
	// arrange
	tool := newFixedTimeTool()

	// act
	info, err := tool.Info(context.Background())

	// assert
	require.NoError(t, err)
	assert.Equal(t, "get_current_time", info.Name)
}
