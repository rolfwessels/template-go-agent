package agent

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type newSessionTool struct {
	reset func(ctx context.Context) error
}

func newNewSessionTool(reset func(ctx context.Context) error) tool.InvokableTool {
	return &newSessionTool{reset: reset}
}

func (t *newSessionTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name:        "new_session",
		Desc:        "Start a fresh conversation. Sweeps the current session into long-term memory and begins with an empty conversation history.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{}),
	}, nil
}

func (t *newSessionTool) InvokableRun(ctx context.Context, _ string, _ ...tool.Option) (string, error) {
	if err := t.reset(ctx); err != nil {
		return "", fmt.Errorf("resetting session: %w", err)
	}
	return "Session saved. Starting fresh with an empty conversation history.", nil
}
