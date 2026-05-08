package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const (
	defaultTimeoutMS  = 30_000
	maxResponseBodyKB = 512
)

type httpFetchTool struct {
	client *http.Client
}

func newHTTPFetchTool() tool.InvokableTool {
	return &httpFetchTool{client: &http.Client{}}
}

type httpFetchInput struct {
	URL       string            `json:"url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	Body      json.RawMessage   `json:"body"`
	TimeoutMS int               `json:"timeout_ms"`
}

type httpFetchOutput struct {
	Status   int               `json:"status"`
	Headers  map[string]string `json:"headers"`
	Body     string            `json:"body"`
	FinalURL string            `json:"final_url"`
}

func (h *httpFetchTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "http_fetch",
		Desc: "Make an HTTP request to any URL. Use for calling APIs, fetching web pages, or interacting with HTTP endpoints.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"url": {
				Type:     schema.String,
				Desc:     "The URL to request.",
				Required: true,
			},
			"method": {
				Type: schema.String,
				Desc: `HTTP method: GET, POST, PUT, PATCH, DELETE. Defaults to GET.`,
			},
			"headers": {
				Type: schema.Object,
				Desc: `Request headers as key-value pairs, e.g. {"Authorization": "Bearer token"}.`,
			},
			"body": {
				Type: schema.String,
				Desc: `Request body. Pass a plain string or a JSON-encoded object. For JSON APIs, set Content-Type: application/json in headers.`,
			},
			"timeout_ms": {
				Type: schema.Integer,
				Desc: fmt.Sprintf("Request timeout in milliseconds. Defaults to %d.", defaultTimeoutMS),
			},
		}),
	}, nil
}

func (h *httpFetchTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var input httpFetchInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return "", fmt.Errorf("parsing arguments: %w", err)
	}

	method := strings.ToUpper(input.Method)
	if method == "" {
		method = http.MethodGet
	}

	bodyReader, err := buildRequestBody(input.Body)
	if err != nil {
		return "", err
	}

	timeoutMS := input.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = defaultTimeoutMS
	}

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, input.URL, bodyReader)
	if err != nil {
		return "", fmt.Errorf("building request: %w", err)
	}
	for k, v := range input.Headers {
		req.Header.Set(k, v)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxResponseBodyKB*1024)
	rawBody, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("reading response body: %w", err)
	}

	out := httpFetchOutput{
		Status:   resp.StatusCode,
		Headers:  flattenHeaders(resp.Header),
		Body:     string(rawBody),
		FinalURL: resp.Request.URL.String(),
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("encoding result: %w", err)
	}
	return string(b), nil
}

func buildRequestBody(raw json.RawMessage) (io.Reader, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("parsing body string: %w", err)
		}
		return strings.NewReader(s), nil
	}
	return bytes.NewReader(raw), nil
}

func flattenHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, vals := range h {
		out[k] = strings.Join(vals, ", ")
	}
	return out
}
