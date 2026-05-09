package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// --- read_memory_file ---

type readMemoryFileTool struct {
	memDir string
}

func newReadMemoryFileTool(memDir string) tool.InvokableTool {
	return &readMemoryFileTool{memDir: memDir}
}

func (t *readMemoryFileTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "read_memory_file",
		Desc: "Read a file from your long-term memory. Use the MEMORY.md index to discover available files. Filename is relative to memory/ (e.g. daily/2026-05-09.md).",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"filename": {
				Type:     schema.String,
				Desc:     "Relative path under memory/ (e.g. daily/2026-05-09.md).",
				Required: true,
			},
		}),
	}, nil
}

type readMemoryFileInput struct {
	Filename string `json:"filename"`
}

func (t *readMemoryFileTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var in readMemoryFileInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &in); err != nil {
		return "error: invalid arguments", nil
	}
	cleaned := filepath.Clean(in.Filename)
	if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
		return "error: invalid filename", nil
	}
	data, err := os.ReadFile(filepath.Join(t.memDir, cleaned))
	if err != nil {
		return fmt.Sprintf("error: %s", err.Error()), nil
	}
	return string(data), nil
}

// --- search_memory ---

type searchMemoryTool struct {
	memDir string
}

func newSearchMemoryTool(memDir string) tool.InvokableTool {
	return &searchMemoryTool{memDir: memDir}
}

func (t *searchMemoryTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "search_memory",
		Desc: "Case-insensitive substring search across all long-term memory files. Returns matching lines grouped by filename.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"queries": {
				Type:     schema.Array,
				Desc:     "List of search terms to find across memory files.",
				Required: true,
				ElemInfo: &schema.ParameterInfo{Type: schema.String},
			},
		}),
	}, nil
}

type searchMemoryInput struct {
	Queries []string `json:"queries"`
}

func (t *searchMemoryTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var in searchMemoryInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &in); err != nil {
		return "error: invalid arguments", nil
	}
	results := t.searchFiles(in.Queries)
	b, err := json.Marshal(results)
	if err != nil {
		return "error: encoding results", nil
	}
	return string(b), nil
}

func (t *searchMemoryTool) searchFiles(queries []string) map[string][]string {
	results := map[string][]string{}
	_ = filepath.Walk(t.memDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(t.memDir, path)
		for _, line := range strings.Split(string(data), "\n") {
			for _, q := range queries {
				if strings.Contains(strings.ToLower(line), strings.ToLower(q)) {
					results[rel] = append(results[rel], line)
					break
				}
			}
		}
		return nil
	})
	return results
}
