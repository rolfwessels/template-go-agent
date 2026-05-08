package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalculatorTool_Addition(t *testing.T) {
	tool := &calculatorTool{}
	result, err := tool.InvokableRun(context.Background(), `{"expression":"1.1 + 2.2"}`)
	require.NoError(t, err)
	assert.Equal(t, "3.3", result)
}

func TestCalculatorTool_ExactDecimal(t *testing.T) {
	tool := &calculatorTool{}
	result, err := tool.InvokableRun(context.Background(), `{"expression":"0.1 + 0.2"}`)
	require.NoError(t, err)
	assert.Equal(t, "0.3", result)
}

func TestCalculatorTool_Precedence(t *testing.T) {
	tool := &calculatorTool{}
	result, err := tool.InvokableRun(context.Background(), `{"expression":"2 + 3 * 4"}`)
	require.NoError(t, err)
	assert.Equal(t, "14", result)
}

func TestCalculatorTool_Parentheses(t *testing.T) {
	tool := &calculatorTool{}
	result, err := tool.InvokableRun(context.Background(), `{"expression":"(2 + 3) * 4"}`)
	require.NoError(t, err)
	assert.Equal(t, "20", result)
}

func TestCalculatorTool_UnaryNegative(t *testing.T) {
	tool := &calculatorTool{}
	result, err := tool.InvokableRun(context.Background(), `{"expression":"-5 + 3"}`)
	require.NoError(t, err)
	assert.Equal(t, "-2", result)
}

func TestCalculatorTool_Division(t *testing.T) {
	tool := &calculatorTool{}
	result, err := tool.InvokableRun(context.Background(), `{"expression":"10 / 4"}`)
	require.NoError(t, err)
	assert.Equal(t, "2.5", result)
}

func TestCalculatorTool_DivisionByZero(t *testing.T) {
	tool := &calculatorTool{}
	_, err := tool.InvokableRun(context.Background(), `{"expression":"5 / 0"}`)
	assert.ErrorContains(t, err, "division by zero")
}

func TestCalculatorTool_InvalidExpression(t *testing.T) {
	tool := &calculatorTool{}
	_, err := tool.InvokableRun(context.Background(), `{"expression":"2 +"}`)
	assert.Error(t, err)
}

func TestCalculatorTool_MismatchedParens(t *testing.T) {
	tool := &calculatorTool{}
	_, err := tool.InvokableRun(context.Background(), `{"expression":"(2 + 3"}`)
	assert.ErrorContains(t, err, "parenthesis")
}

func TestCalculatorTool_Info(t *testing.T) {
	tool := &calculatorTool{}
	info, err := tool.Info(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "calculate", info.Name)
}
