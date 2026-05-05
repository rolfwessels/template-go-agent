package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type tavilyTool struct {
	apiKey string
}

type tavilyRequest struct {
	APIKey          string `json:"api_key"`
	Query           string `json:"query"`
	SearchDepth     string `json:"search_depth"`
	MaxResults      int    `json:"max_results"`
	IncludeAnswer   bool   `json:"include_answer"`
}

type tavilyResponse struct {
	Answer  string         `json:"answer"`
	Results []tavilyResult `json:"results"`
}

type tavilyResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

type tavilyInput struct {
	Query string `json:"query"`
}

func newTavilyTool(apiKey string) tool.InvokableTool {
	return &tavilyTool{apiKey: apiKey}
}

func (t *tavilyTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "web_search",
		Desc: "Search the web for current information. Use this to answer questions about recent events, facts, or anything that requires up-to-date knowledge.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "The search query",
				Required: true,
			},
		}),
	}, nil
}

func (t *tavilyTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var input tavilyInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return "", fmt.Errorf("parsing tool arguments: %w", err)
	}

	result, err := t.search(input.Query)
	if err != nil {
		return "", fmt.Errorf("tavily search: %w", err)
	}

	return result, nil
}

func (t *tavilyTool) search(query string) (string, error) {
	req := tavilyRequest{
		APIKey:        t.apiKey,
		Query:         query,
		SearchDepth:   "basic",
		MaxResults:    5,
		IncludeAnswer: true,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("encoding request: %w", err)
	}

	resp, err := http.Post("https://api.tavily.com/search", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("calling tavily API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tavily API returned status %d", resp.StatusCode)
	}

	var tavilyResp tavilyResponse
	if err := json.NewDecoder(resp.Body).Decode(&tavilyResp); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	return formatResults(tavilyResp), nil
}

func formatResults(resp tavilyResponse) string {
	var buf bytes.Buffer
	if resp.Answer != "" {
		fmt.Fprintf(&buf, "Answer: %s\n\n", resp.Answer)
	}
	for _, r := range resp.Results {
		fmt.Fprintf(&buf, "- %s (%s)\n  %s\n\n", r.Title, r.URL, r.Content)
	}
	return buf.String()
}
