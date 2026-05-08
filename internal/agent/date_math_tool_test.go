package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDateMathTool() *dateMathTool {
	return &dateMathTool{}
}

func TestDateMathTool_Add_Days(t *testing.T) {
	// arrange
	tool := newTestDateMathTool()

	// act
	result, err := tool.InvokableRun(context.Background(), `{"operation":"add","date":"2026-05-08T10:00:00Z","amount":5,"unit":"days"}`)

	// assert
	require.NoError(t, err)
	assert.Equal(t, "2026-05-13T10:00:00Z", result)
}

func TestDateMathTool_Add_Negative(t *testing.T) {
	// arrange
	tool := newTestDateMathTool()

	// act
	result, err := tool.InvokableRun(context.Background(), `{"operation":"add","date":"2026-05-08T10:00:00Z","amount":-3,"unit":"hours"}`)

	// assert
	require.NoError(t, err)
	assert.Equal(t, "2026-05-08T07:00:00Z", result)
}

func TestDateMathTool_Add_Months(t *testing.T) {
	// arrange
	tool := newTestDateMathTool()

	// act
	result, err := tool.InvokableRun(context.Background(), `{"operation":"add","date":"2026-01-31T00:00:00Z","amount":1,"unit":"months"}`)

	// assert
	require.NoError(t, err)
	assert.Equal(t, "2026-03-03T00:00:00Z", result)
}

func TestDateMathTool_Add_DateOnly(t *testing.T) {
	// arrange
	tool := newTestDateMathTool()

	// act
	result, err := tool.InvokableRun(context.Background(), `{"operation":"add","date":"2026-05-08","amount":1,"unit":"weeks"}`)

	// assert
	require.NoError(t, err)
	assert.Equal(t, "2026-05-15T00:00:00Z", result)
}

func TestDateMathTool_Add_UnknownUnit(t *testing.T) {
	// arrange
	tool := newTestDateMathTool()

	// act
	_, err := tool.InvokableRun(context.Background(), `{"operation":"add","date":"2026-05-08T10:00:00Z","amount":1,"unit":"fortnights"}`)

	// assert
	assert.ErrorContains(t, err, "unknown unit")
}

func TestDateMathTool_Diff(t *testing.T) {
	// arrange
	tool := newTestDateMathTool()

	// act
	result, err := tool.InvokableRun(context.Background(), `{"operation":"diff","from":"2026-05-01T00:00:00Z","to":"2026-05-08T03:30:00Z"}`)

	// assert
	require.NoError(t, err)
	var out diffOutput
	require.NoError(t, json.Unmarshal([]byte(result), &out))
	assert.Equal(t, 7, out.Days)
	assert.Equal(t, 3, out.Hours)
	assert.Equal(t, 30, out.Minutes)
	assert.Equal(t, 0, out.Seconds)
	assert.Contains(t, out.Human, "7 days")
}

func TestDateMathTool_Diff_Negative(t *testing.T) {
	// arrange
	tool := newTestDateMathTool()

	// act
	result, err := tool.InvokableRun(context.Background(), `{"operation":"diff","from":"2026-05-08T00:00:00Z","to":"2026-05-07T00:00:00Z"}`)

	// assert
	require.NoError(t, err)
	var out diffOutput
	require.NoError(t, json.Unmarshal([]byte(result), &out))
	assert.Equal(t, 1, out.Days)
	assert.Contains(t, out.Human, "ago")
}

func TestDateMathTool_UnknownOperation(t *testing.T) {
	// arrange
	tool := newTestDateMathTool()

	// act
	_, err := tool.InvokableRun(context.Background(), `{"operation":"multiply"}`)

	// assert
	assert.ErrorContains(t, err, "unknown operation")
}

func TestDateMathTool_Info(t *testing.T) {
	// arrange
	tool := newTestDateMathTool()

	// act
	info, err := tool.Info(context.Background())

	// assert
	require.NoError(t, err)
	assert.Equal(t, "date_math", info.Name)
}
