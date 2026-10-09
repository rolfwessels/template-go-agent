package agent

import (
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tavilyTestClient(t *testing.T, handler http.HandlerFunc) *tavilyTool {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	certTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{"api.tavily.com"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, certTemplate, certTemplate, &key.PublicKey, key)
	require.NoError(t, err)
	srv := newUnstartedFetchServer(t, handler)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	srv.StartTLS()
	tool := newTavilyTool("SECRET_API_KEY").(*tavilyTool)
	tr := tool.client.Transport.(*http.Transport)
	// Only tests replace networking; production retains the standard cloned
	// transport, normal DNS, and http.ProxyFromEnvironment.
	tr.Proxy = nil
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		assert.Equal(t, "api.tavily.com:443", address)
		return dialFetchTestServer(ctx, srv)
	}
	roots := x509.NewCertPool()
	roots.AddCert(srv.Certificate())
	tr.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	t.Cleanup(tool.client.CloseIdleConnections)
	return tool
}

func TestTavilyFixedEndpointAndNoRedirects(t *testing.T) {
	var hits atomic.Int32
	redirects := map[string]int{"redirect301": 301, "redirect302": 302, "redirect303": 303, "redirect307": 307, "redirect308": 308}
	tool := tavilyTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		assert.Equal(t, "api.tavily.com", r.Host)
		assert.Equal(t, "/search", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		assert.Empty(t, r.URL.RawQuery)
		assert.Equal(t, "api.tavily.com", r.TLS.ServerName)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, tavilyUserAgent, r.UserAgent())
		assert.NotContains(t, r.Header, "Authorization")
		for _, values := range r.Header {
			for _, value := range values {
				assert.NotContains(t, value, "SECRET_API_KEY")
			}
		}
		var request tavilyRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		assert.Equal(t, "SECRET_API_KEY", request.APIKey)
		if status, ok := redirects[request.Query]; ok {
			http.Redirect(w, r, "https://other.example.com/leak?token=SECRET_API_KEY", status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if request.Query == "accepted" {
			w.WriteHeader(http.StatusAccepted)
		}
		_, _ = io.WriteString(w, `{"answer":"answer","results":[{"title":"Title","url":"https://example.com","content":"Snippet"}]}`)
	})
	result, err := tool.InvokableRun(context.Background(), `{"query":"research","url":"https://other.example.com/leak","method":"GET","api_key":"UNTRUSTED_KEY"}`)
	require.NoError(t, err)
	assert.Contains(t, result, "Untrusted")
	assert.Contains(t, result, "Snippet")
	_, err = tool.InvokableRun(context.Background(), `{"query":"accepted"}`)
	require.NoError(t, err)
	for query, status := range redirects {
		_, err = tool.InvokableRun(context.Background(), jsonArgs(map[string]any{"query": query}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), fmt.Sprint(status))
		assert.NotContains(t, err.Error(), "SECRET")
	}
	assert.Equal(t, int32(2+len(redirects)), hits.Load())
	assert.Nil(t, tool.client.Jar)
	assert.Equal(t, 30*time.Second, tool.client.Timeout)
}

func TestTavilyProductionTransport(t *testing.T) {
	tool := newTavilyTool("test-key").(*tavilyTool)
	t.Cleanup(tool.client.CloseIdleConnections)
	tr, ok := tool.client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.NotSame(t, http.DefaultTransport, tr)
	require.NotNil(t, tr.Proxy)
	assert.Equal(t, reflect.ValueOf(http.ProxyFromEnvironment).Pointer(), reflect.ValueOf(tr.Proxy).Pointer())
	req, err := http.NewRequest(http.MethodPost, tavilyEndpoint, nil)
	require.NoError(t, err)
	assert.Equal(t, "https://api.tavily.com/search", req.URL.String())
	expected, expectedErr := http.ProxyFromEnvironment(req)
	actual, actualErr := tr.Proxy(req)
	assert.Equal(t, expected, actual)
	assert.Equal(t, expectedErr, actualErr)
	assert.Equal(t, 10*time.Second, tr.TLSHandshakeTimeout)
	assert.Equal(t, 10*time.Second, tr.ResponseHeaderTimeout)
	assert.Equal(t, int64(64<<10), tr.MaxResponseHeaderBytes)
	assert.Equal(t, 30*time.Second, tool.client.Timeout)
}

func TestTavilyLimitsAndCancellation(t *testing.T) {
	tool := tavilyTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var input tavilyRequest
		_ = json.NewDecoder(r.Body).Decode(&input)
		switch input.Query {
		case "big":
			_, _ = io.WriteString(w, strings.Repeat("x", (512<<10)+1))
		case "gzip":
			w.Header().Set("Content-Encoding", "gzip")
			writer := gzip.NewWriter(w)
			_, _ = io.WriteString(writer, strings.Repeat("x", 600<<10))
			_ = writer.Close()
		case "headers":
			w.Header().Set("X-Huge", strings.Repeat("x", 65<<10))
		case "wait":
			<-r.Context().Done()
		case "wait-body":
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		case "forbidden":
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, "SECRET_API_KEY")
		default:
			_, _ = io.WriteString(w, "invalid json SECRET_API_KEY")
		}
	})
	for _, query := range []string{"big", "gzip"} {
		_, err := tool.InvokableRun(context.Background(), jsonArgs(map[string]any{"query": query}))
		assert.ErrorContains(t, err, "response_limit")
	}
	for _, query := range []string{"headers", "invalid", "forbidden"} {
		_, err := tool.InvokableRun(context.Background(), jsonArgs(map[string]any{"query": query}))
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "SECRET")
	}
	for _, query := range []string{"wait", "wait-body"} {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		_, err := tool.InvokableRun(ctx, jsonArgs(map[string]any{"query": query}))
		cancel()
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := tool.InvokableRun(ctx, `{"query":"wait"}`)
	assert.ErrorIs(t, err, context.Canceled)
	_, err = tool.InvokableRun(context.Background(), jsonArgs(map[string]any{"query": strings.Repeat("x", (64<<10)+1)}))
	assert.ErrorContains(t, err, "query_limit")
	// The client's own timeout also bounds reads without a caller deadline.
	tool.client.Timeout = 30 * time.Millisecond
	for _, query := range []string{"wait", "wait-body"} {
		_, err = tool.InvokableRun(context.Background(), jsonArgs(map[string]any{"query": query}))
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	}
}

type tavilyRoundTripFunc func(*http.Request) (*http.Response, error)

func (f tavilyRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type tavilyErrorReader struct{}

func (tavilyErrorReader) Read([]byte) (int, error) {
	return 0, errors.New("body read failed: SECRET_API_KEY")
}

func TestTavilyCallerContextAndErrorRedaction(t *testing.T) {
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "caller-value")
	for _, bodyError := range []bool{false, true} {
		t.Run(fmt.Sprintf("body_error=%t", bodyError), func(t *testing.T) {
			tool := newTavilyTool("SECRET_API_KEY").(*tavilyTool)
			tool.client.Transport = tavilyRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "caller-value", req.Context().Value(contextKey{}))
				assert.Equal(t, tavilyEndpoint, req.URL.String())
				assert.Equal(t, http.MethodPost, req.Method)
				assert.Nil(t, req.URL.User)
				assert.Empty(t, req.URL.RawQuery)
				var input tavilyRequest
				require.NoError(t, json.NewDecoder(req.Body).Decode(&input))
				assert.Equal(t, "SECRET_API_KEY", input.APIKey)
				if bodyError {
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(tavilyErrorReader{})}, nil
				}
				return nil, errors.New("transport failed: SECRET_API_KEY")
			})
			_, err := tool.InvokableRun(ctx, `{"query":"research"}`)
			require.Error(t, err)
			assert.ErrorContains(t, err, "request_failed")
			assert.NotContains(t, err.Error(), "SECRET_API_KEY")
			assert.NotContains(t, err.Error(), "http_fetch")
		})
	}
}

func TestTavilyContextErrorRedaction(t *testing.T) {
	for _, stage := range []string{"transport", "body", "response_context_error", "response_context_eof"} {
		for _, contextErr := range []error{context.DeadlineExceeded, context.Canceled} {
			t.Run(fmt.Sprintf("%s/%s", stage, contextErr), func(t *testing.T) {
				callerCtx := context.Background()
				tool := newTavilyTool("SECRET_API_KEY").(*tavilyTool)
				tool.client.Transport = tavilyRoundTripFunc(func(req *http.Request) (*http.Response, error) {
					assert.NoError(t, req.Context().Err())
					failure := fmt.Errorf("SECRET_API_KEY transport/body detail: %w", contextErr)
					if stage == "transport" {
						return nil, failure
					}
					if strings.HasPrefix(stage, "response_context_") {
						// Only the response's context has completed; the caller's
						// timer need not have fired when a raw body error arrives.
						var responseCtx context.Context
						var cancel context.CancelFunc
						if contextErr == context.DeadlineExceeded {
							responseCtx, cancel = context.WithDeadline(callerCtx, time.Now().Add(-time.Hour))
						} else {
							responseCtx, cancel = context.WithCancel(callerCtx)
							cancel()
						}
						defer cancel()
						req = req.WithContext(responseCtx)
						failure = errors.New("body read failed: SECRET_API_KEY")
						if stage == "response_context_eof" {
							failure = io.EOF
						}
					}
					return &http.Response{StatusCode: http.StatusOK, Request: req, Body: io.NopCloser(fetchErrorReader{failure})}, nil
				})
				_, err := tool.InvokableRun(callerCtx, `{"query":"research"}`)
				require.Error(t, err)
				assert.NoError(t, callerCtx.Err())
				assert.ErrorIs(t, err, contextErr)
				assert.NotContains(t, err.Error(), "SECRET")
				assert.NotContains(t, err.Error(), "http_fetch")
			})
		}
	}
}
