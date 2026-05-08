package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestHTTPFetchTool() *httpFetchTool {
	return &httpFetchTool{client: &http.Client{}}
}

func TestHTTPFetchTool_Get(t *testing.T) {
	// arrange
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("X-Test", "hello")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response body"))
	}))
	defer srv.Close()
	tool := newTestHTTPFetchTool()

	// act
	raw, err := tool.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": srv.URL}))

	// assert
	require.NoError(t, err)
	var out httpFetchOutput
	require.NoError(t, json.Unmarshal([]byte(raw), &out))
	assert.Equal(t, 200, out.Status)
	assert.Equal(t, "response body", out.Body)
	assert.Equal(t, "hello", out.Headers["X-Test"])
	assert.Equal(t, srv.URL, out.FinalURL)
}

func TestHTTPFetchTool_PostWithStringBody(t *testing.T) {
	// arrange
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 512)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	tool := newTestHTTPFetchTool()

	// act
	raw, err := tool.InvokableRun(context.Background(), jsonArgs(map[string]any{
		"url":    srv.URL,
		"method": "POST",
		"body":   "hello server",
	}))

	// assert
	require.NoError(t, err)
	var out httpFetchOutput
	require.NoError(t, json.Unmarshal([]byte(raw), &out))
	assert.Equal(t, 201, out.Status)
	assert.Equal(t, "hello server", receivedBody)
}

func TestHTTPFetchTool_PostWithObjectBody(t *testing.T) {
	// arrange
	var receivedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 512)
		n, _ := r.Body.Read(buf)
		receivedBody = buf[:n]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	tool := newTestHTTPFetchTool()

	// act
	_, err := tool.InvokableRun(context.Background(), `{"url":"`+srv.URL+`","method":"POST","body":{"key":"value"}}`)

	// assert
	require.NoError(t, err)
	assert.JSONEq(t, `{"key":"value"}`, string(receivedBody))
}

func TestHTTPFetchTool_CustomHeaders(t *testing.T) {
	// arrange
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	tool := newTestHTTPFetchTool()

	// act
	_, err := tool.InvokableRun(context.Background(), jsonArgs(map[string]any{
		"url":     srv.URL,
		"headers": map[string]string{"Authorization": "Bearer secret"},
	}))

	// assert
	require.NoError(t, err)
	assert.Equal(t, "Bearer secret", receivedAuth)
}

func TestHTTPFetchTool_FollowsRedirect(t *testing.T) {
	// arrange
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("final"))
	}))
	defer srv.Close()
	tool := newTestHTTPFetchTool()

	// act
	raw, err := tool.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": srv.URL}))

	// assert
	require.NoError(t, err)
	var out httpFetchOutput
	require.NoError(t, json.Unmarshal([]byte(raw), &out))
	assert.Equal(t, "final", out.Body)
	assert.Contains(t, out.FinalURL, "/final")
}

func TestHTTPFetchTool_Timeout(t *testing.T) {
	// arrange
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// never respond
		<-r.Context().Done()
	}))
	defer srv.Close()
	tool := newTestHTTPFetchTool()

	// act
	_, err := tool.InvokableRun(context.Background(), jsonArgs(map[string]any{
		"url":        srv.URL,
		"timeout_ms": 50,
	}))

	// assert
	assert.ErrorContains(t, err, "executing request")
}

func TestHTTPFetchTool_Info(t *testing.T) {
	// arrange
	tool := newTestHTTPFetchTool()

	// act
	info, err := tool.Info(context.Background())

	// assert
	require.NoError(t, err)
	assert.Equal(t, "http_fetch", info.Name)
}

func jsonArgs(v map[string]any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
