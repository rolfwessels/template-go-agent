package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"testing"

	"github.com/rolfwessels/template-go-agent/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResearchToolPolicyWiringAndSchema(t *testing.T) {
	cfg := &config.Config{HTTPFetch: config.DefaultHTTPFetchPolicy()}
	tools, err := researchTools(cfg)
	require.NoError(t, err)
	require.Len(t, tools, 2)
	h, ok := tools[1].(*httpFetchTool)
	require.True(t, ok)
	info, err := h.Info(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "http_fetch", info.Name)
	sc, err := info.ParamsOneOf.ToJSONSchema()
	require.NoError(t, err)
	method, ok := sc.Properties.Get("method")
	require.True(t, ok)
	assert.Equal(t, []any{"GET", "HEAD"}, method.Enum)
	_, ok = sc.Properties.Get("body")
	assert.False(t, ok)
	cfg.HTTPFetch.AllowMutations = true
	cfg.HTTPFetch.MutationAllowedHosts = []string{"api.example.com"}
	cfg.HTTPFetch.MaxTimeoutMS = 42
	tools, err = researchTools(cfg)
	require.NoError(t, err)
	h = tools[1].(*httpFetchTool)
	assert.Equal(t, cfg.HTTPFetch, h.policy)
	info, err = h.Info(context.Background())
	require.NoError(t, err)
	sc, err = info.ParamsOneOf.ToJSONSchema()
	require.NoError(t, err)
	method, _ = sc.Properties.Get("method")
	assert.Equal(t, []any{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE"}, method.Enum)
	_, ok = sc.Properties.Get("body")
	assert.True(t, ok)
	cfg.HTTPFetch.Enabled = false
	tools, err = researchTools(cfg)
	require.NoError(t, err)
	assert.Len(t, tools, 1)
	cfg.HTTPFetch.MutationAllowedHosts = nil
	_, err = researchTools(cfg)
	require.Error(t, err)
}

func TestHTTPFetchErrorAndLogRedaction(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	h := newHTTPFetchToolWithNetwork(config.DefaultHTTPFetchPolicy(), fetchNetwork{
		lookup: func(context.Context, string) ([]netip.Addr, error) {
			return nil, errors.New("https://example.com/SECRET_PATH?token=SECRET_QUERY")
		},
		dial: func(context.Context, string, string) (net.Conn, error) { t.Fatal("must not dial"); return nil, nil },
	})
	for _, input := range []map[string]any{
		{"url": "https://example.com/SECRET_PATH?token=SECRET_QUERY"},
		{"url": "https://example.com/SECRET_PATH", "headers": map[string]string{"Authorization": "SECRET_HEADER"}},
		{"url": "https://SECRET_USER:SECRET_PASSWORD@example.com/SECRET_PATH"},
		{"url": "https://example.com", "method": "POST", "body": "SECRET_BODY"},
	} {
		_, err := h.InvokableRun(context.Background(), jsonArgs(input))
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "SECRET")
	}
	assert.NotContains(t, logs.String(), "SECRET")
	assert.Contains(t, logs.String(), `"host":"example.com"`)
	assert.Contains(t, logs.String(), `"code":"dns_failed"`)
	assert.Contains(t, logs.String(), `"duration_ms":`)
	assert.Contains(t, logs.String(), `"status":`)
	assert.NotContains(t, logs.String(), `"args":`)
}

type fetchRoundTripFunc func(*http.Request) (*http.Response, error)

func (f fetchRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type fetchErrorReader struct{ err error }

func (r fetchErrorReader) Read([]byte) (int, error) { return 0, r.err }

func TestHTTPFetchContextErrorLogs(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	for _, stage := range []string{"dns", "dial", "body"} {
		for _, contextErr := range []error{context.DeadlineExceeded, context.Canceled} {
			t.Run(fmt.Sprintf("%s/%s", stage, contextErr), func(t *testing.T) {
				logs.Reset()
				failure := fmt.Errorf("SECRET_HOST 10.0.0.1 SECRET_DETAIL: %w", contextErr)
				h := newHTTPFetchToolWithNetwork(config.DefaultHTTPFetchPolicy(), fetchNetwork{
					lookup: func(ctx context.Context, _ string) ([]netip.Addr, error) {
						assert.NoError(t, ctx.Err())
						if stage == "dns" {
							return nil, failure
						}
						return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
					},
					dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
						assert.NoError(t, ctx.Err())
						return nil, failure
					},
				})
				t.Cleanup(h.client.CloseIdleConnections)
				if stage == "body" {
					h.client.Transport = fetchRoundTripFunc(func(req *http.Request) (*http.Response, error) {
						assert.NoError(t, req.Context().Err())
						return &http.Response{StatusCode: http.StatusOK, Request: req, Body: io.NopCloser(fetchErrorReader{failure})}, nil
					})
				}
				_, err := h.InvokableRun(context.Background(), `{"url":"https://example.com/SECRET_PATH"}`)
				require.Error(t, err)
				assert.ErrorIs(t, err, contextErr)
				assert.NotContains(t, err.Error(), "SECRET")
				assert.NotContains(t, err.Error(), "10.0.0.1")
				code := "timeout"
				if contextErr == context.Canceled {
					code = "canceled"
				}
				assert.Contains(t, logs.String(), `"code":"`+code+`"`)
				assert.NotContains(t, logs.String(), "SECRET")
				assert.NotContains(t, logs.String(), "10.0.0.1")
			})
		}
	}
}
