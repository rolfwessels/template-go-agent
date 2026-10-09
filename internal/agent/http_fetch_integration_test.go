package agent

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
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
