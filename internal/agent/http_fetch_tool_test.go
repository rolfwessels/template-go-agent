package agent

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rolfwessels/template-go-agent/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Production URL, DNS and IP policy is exercised before this explicit test
// dialer maps approved public literals to a local httptest listener.
func testFetchForServer(t *testing.T, srv *httptest.Server, p config.HTTPFetchPolicy) (*httpFetchTool, string) {
	t.Helper()
	h := newHTTPFetchToolWithNetwork(p, fetchNetwork{
		lookup: func(_ context.Context, host string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("2606:4700::1111")}, nil
		},
		dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			if !publicFetchIP(netip.MustParseAddr(host)) {
				t.Errorf("forbidden dial: %s", address)
			}
			return dialFetchTestServer(ctx, srv)
		},
	})
	scheme := "http"
	if srv.TLS != nil {
		scheme = "https"
		roots := x509.NewCertPool()
		roots.AddCert(srv.Certificate())
		h.client.Transport.(*http.Transport).TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	}
	t.Cleanup(h.client.CloseIdleConnections)
	return h, scheme + "://example.com"
}

func newFetchServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := newUnstartedFetchServer(t, handler)
	srv.Start()
	return srv
}

func jsonArgs(v map[string]any) string { b, _ := json.Marshal(v); return string(b) }

func runFetch(t *testing.T, h *httpFetchTool, input map[string]any) httpFetchOutput {
	t.Helper()
	raw, err := h.InvokableRun(context.Background(), jsonArgs(input))
	require.NoError(t, err)
	var out httpFetchOutput
	require.NoError(t, json.Unmarshal([]byte(raw), &out))
	return out
}

func TestHTTPFetchPublicReadAndHeaders(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:1")
	srv := newFetchServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "example.com", r.Host)
		assert.Equal(t, fetchUserAgent, r.UserAgent())
		assert.Empty(t, r.Header.Get("Cookie"))
		assert.Equal(t, "fr", r.Header.Get("Accept-Language"))
		assert.Equal(t, "text/plain", r.Header.Get("Accept"))
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Last-Modified", "Wed, 07 Oct 2026 00:00:00 GMT")
		w.Header().Set("Set-Cookie", "session=secret")
		w.Header().Set("X-Secret", "secret")
		w.Header().Set("Location", "https://example.com/?secret=token")
		if r.Method != "HEAD" {
			_, _ = io.WriteString(w, "response body")
		}
	})
	h, url := testFetchForServer(t, srv, config.DefaultHTTPFetchPolicy())
	for _, method := range []string{"GET", "HEAD"} {
		out := runFetch(t, h, map[string]any{"url": url, "method": method, "headers": map[string]string{"aCcEpT": "text/plain", "accept-language": "fr"}})
		assert.Equal(t, 200, out.Status)
		assert.Equal(t, url, out.FinalURL)
		assert.Equal(t, "text/plain", out.ContentType)
		assert.False(t, out.Truncated)
		if method == "GET" {
			assert.Equal(t, "response body", out.Body)
		} else {
			assert.Empty(t, out.Body)
		}
		assert.Len(t, out.Headers, 2)
		assert.NotEmpty(t, out.Headers["Last-Modified"])
	}
	assert.Nil(t, h.client.Jar)
}

func TestHTTPFetchEmptyResponses(t *testing.T) {
	for _, tt := range []struct {
		name, method string
		status       int
		contentType  string
	}{
		{"HEAD HTML page", http.MethodHead, http.StatusOK, "text/html; charset=utf-8"},
		{"204 HTML", http.MethodGet, http.StatusNoContent, "text/html; charset=utf-8"},
		{"304", http.MethodGet, http.StatusNotModified, ""},
		{"zero-length 200 HTML", http.MethodGet, http.StatusOK, "text/html; charset=utf-8"},
		{"zero-length 200 binary", http.MethodGet, http.StatusOK, "application/pdf"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := newUnstartedFetchServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tt.method, r.Method)
				if tt.contentType != "" {
					w.Header().Set("Content-Type", tt.contentType)
				}
				w.Header().Set("ETag", `"empty-response"`)
				w.Header().Set("Set-Cookie", "session=secret")
				w.Header().Set("X-Secret", "secret")
				w.WriteHeader(tt.status)
				if r.Method == http.MethodHead {
					// The server suppresses these bytes on HEAD, just as a
					// public HTML page does while retaining its metadata.
					_, _ = io.WriteString(w, "<title>Public page</title><p>Research content</p>")
				}
			}))
			srv.StartTLS()
			h, baseURL := testFetchForServer(t, srv, config.DefaultHTTPFetchPolicy())
			out := runFetch(t, h, map[string]any{"url": baseURL + "/", "method": tt.method})
			assert.Equal(t, tt.status, out.Status)
			assert.Equal(t, baseURL+"/", out.FinalURL)
			wantHeaders := map[string]string{"ETag": `"empty-response"`}
			wantKind := ""
			if tt.contentType != "" {
				wantHeaders["Content-Type"] = tt.contentType
				wantKind = strings.Split(tt.contentType, ";")[0]
			}
			assert.Equal(t, wantKind, out.ContentType)
			assert.Equal(t, wantHeaders, out.Headers)
			assert.Empty(t, out.Body)
			assert.Empty(t, out.Title)
			assert.False(t, out.Truncated)
		})
	}
}

func TestHTTPFetchHTTPSGuardAndCertificateHost(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	srv := newUnstartedFetchServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "example.com", r.TLS.ServerName)
		assert.Equal(t, "example.com", r.Host)
		if r.URL.Path == "/downgrade" {
			http.Redirect(w, r, "http://example.com/final", 302)
			return
		}
		_, _ = io.WriteString(w, "TLS page")
	}))
	srv.StartTLS()
	h, url := testFetchForServer(t, srv, config.DefaultHTTPFetchPolicy())
	out := runFetch(t, h, map[string]any{"url": url})
	assert.Equal(t, "TLS page", out.Body)
	_, err := h.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": url + "/downgrade"}))
	assert.ErrorContains(t, err, "redirect_downgrade_denied")
	// A private DNS answer fails before TLS/dialing as well.
	h = newHTTPFetchToolWithNetwork(config.DefaultHTTPFetchPolicy(), fetchNetwork{
		lookup: func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("127.0.0.1")}, nil
		},
		dial: func(context.Context, string, string) (net.Conn, error) {
			t.Fatal("private DNS must not dial")
			return nil, nil
		},
	})
	_, err = h.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": url}))
	assert.ErrorContains(t, err, "address_denied")
}

func TestHTTPFetchRejectsUnsafeInputsByDefault(t *testing.T) {
	h := newHTTPFetchTool(config.DefaultHTTPFetchPolicy())
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE", "CONNECT", "TRACE", "PROPFIND", "GET secret"} {
		_, err := h.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": "https://example.com", "method": method}))
		assert.Error(t, err, method)
	}
	for _, header := range []string{"Authorization", "aUtHoRiZaTiOn", "Cookie", "X-Api-Key", "Host", "User-Agent", "Content-Type", "Proxy-Authorization", "X-Forwarded-For", "Metadata-Flavor", "X-aws-ec2-metadata-token", "Connection", "Accept-Encoding", "Referer"} {
		_, err := h.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": "https://example.com", "headers": map[string]string{header: "secret"}}))
		assert.ErrorContains(t, err, "header_denied", header)
		assert.NotContains(t, err.Error(), "secret")
	}
	for _, method := range []string{"GET", "HEAD"} {
		for _, body := range []any{"", "secret", map[string]any{}, nil} {
			_, err := h.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": "https://example.com", "method": method, "body": body}))
			assert.ErrorContains(t, err, "read_body_denied")
		}
	}
	_, err := h.InvokableRun(context.Background(), `{"url":`)
	assert.ErrorContains(t, err, "invalid_arguments")
	_, err = h.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": "https://example.com", "timeout_ms": -1}))
	assert.ErrorContains(t, err, "invalid_timeout")
	_, err = h.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": "https://example.com", "headers": map[string]string{"Accept": "text/plain\r\nAuthorization: secret"}}))
	assert.ErrorContains(t, err, "invalid_header")
	p := config.DefaultHTTPFetchPolicy()
	p.Enabled = false
	h = newHTTPFetchTool(p)
	_, err = h.InvokableRun(context.Background(), `{}`)
	assert.ErrorContains(t, err, "disabled")
}

func TestHTTPFetchExplicitMutations(t *testing.T) {
	var hits atomic.Int32
	srv := newFetchServer(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body, _ := io.ReadAll(r.Body)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Empty(t, r.Header.Get("Authorization"))
		if string(body) != "hello" {
			assert.JSONEq(t, `{"key":"value"}`, string(body))
		}
		w.WriteHeader(201)
	})
	p := config.DefaultHTTPFetchPolicy()
	p.AllowMutations = true
	p.MutationAllowedHosts = []string{"example.com"}
	h, url := testFetchForServer(t, srv, p)
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		for _, body := range []any{"hello", map[string]string{"key": "value"}} {
			out := runFetch(t, h, map[string]any{"url": url, "method": method, "body": body, "headers": map[string]string{"content-type": "application/json"}})
			assert.Equal(t, 201, out.Status)
		}
	}
	_, err := h.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": "http://other.example.com", "method": "POST"}))
	assert.ErrorContains(t, err, "mutation_host_denied")
	_, err = h.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": url, "method": "POST", "headers": map[string]string{"Authorization": "secret"}}))
	assert.ErrorContains(t, err, "header_denied")
	for _, method := range []string{"CONNECT", "TRACE", "CUSTOM"} {
		_, err = h.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": url, "method": method}))
		assert.ErrorContains(t, err, "method_denied")
	}
	_, err = h.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": url, "method": "POST", "body": strings.Repeat("x", maxMutationBodyBytes+1)}))
	assert.ErrorContains(t, err, "mutation_body_limit")
	assert.Equal(t, int32(8), hits.Load())
	p.MutationAllowedHosts = nil
	h = newHTTPFetchTool(p)
	_, err = h.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": url, "method": "POST"}))
	assert.ErrorContains(t, err, "invalid_policy")
}

func TestHTTPFetchRedirects(t *testing.T) {
	var finalHits atomic.Int32
	srv := newFetchServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cross":
			http.Redirect(w, r, "http://other.example.com/intermediate", 302)
		case "/intermediate":
			http.Redirect(w, r, "/final", 302)
		case "/private":
			http.Redirect(w, r, "http://127.0.0.1/final", 302)
		case "/metadata":
			http.Redirect(w, r, "http://metadata.google.internal/final", 302)
		case "/dns-private":
			http.Redirect(w, r, "http://private.example.com/final", 302)
		case "/loop":
			http.Redirect(w, r, "/loop", 302)
		case "/one":
			http.Redirect(w, r, "/two", 302)
		case "/two":
			http.Redirect(w, r, "/final", 302)
		case "/mutation":
			http.Redirect(w, r, "/final", 307)
		case "/mutation303":
			http.Redirect(w, r, "/final", 303)
		default:
			finalHits.Add(1)
			if r.Host == "other.example.com" {
				assert.Empty(t, r.Header.Get("Accept-Language"))
				assert.Equal(t, safeFetchHeaders().Get("Accept"), r.Header.Get("Accept"))
				assert.Empty(t, r.Referer())
			}
			_, _ = io.WriteString(w, "final")
		}
	})
	p := config.DefaultHTTPFetchPolicy()
	h, url := testFetchForServer(t, srv, p)
	out := runFetch(t, h, map[string]any{"url": url + "/cross", "headers": map[string]string{"Accept-Language": "secret", "Accept": "custom"}})
	assert.Equal(t, "final", out.Body)
	assert.Equal(t, "http://other.example.com/final", out.FinalURL)
	out = runFetch(t, h, map[string]any{"url": url + "/one"})
	assert.Equal(t, "final", out.Body)
	for _, path := range []string{"/private", "/metadata", "/loop"} {
		_, err := h.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": url + path}))
		require.Error(t, err)
	}
	p.MaxRedirects = 1
	h, _ = testFetchForServer(t, srv, p)
	_, err := h.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": url + "/one"}))
	assert.ErrorContains(t, err, "redirect_limit")
	p.MaxRedirects = 0
	h, _ = testFetchForServer(t, srv, p)
	_, err = h.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": url + "/cross"}))
	assert.ErrorContains(t, err, "redirect_limit")
	p = config.DefaultHTTPFetchPolicy()
	p.AllowMutations = true
	p.MutationAllowedHosts = []string{"example.com"}
	h, _ = testFetchForServer(t, srv, p)
	for _, path := range []string{"/mutation", "/mutation303"} {
		_, err = h.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": url + path, "method": "POST", "body": "x"}))
		assert.ErrorContains(t, err, "mutation_redirect_denied")
	}
	assert.Equal(t, int32(2), finalHits.Load())
	// A redirect's syntactically public hostname is rechecked through DNS.
	h = newHTTPFetchToolWithNetwork(p, fetchNetwork{
		lookup: func(_ context.Context, host string) ([]netip.Addr, error) {
			if host == "private.example.com" {
				return []netip.Addr{netip.MustParseAddr("10.0.0.1")}, nil
			}
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		},
		dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			require.Equal(t, "8.8.8.8:80", address)
			return dialFetchTestServer(ctx, srv)
		},
	})
	t.Cleanup(h.client.CloseIdleConnections)
	_, err = h.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": url + "/dns-private"}))
	assert.ErrorContains(t, err, "address_denied")
}

func TestHTTPFetchResourceLimits(t *testing.T) {
	srv := newFetchServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		switch r.URL.Path {
		case "/huge-header":
			w.Header().Set("X-Huge", strings.Repeat("x", 65<<10))
		case "/gzip":
			w.Header().Set("Content-Encoding", "gzip")
			writer := gzip.NewWriter(w)
			_, _ = io.WriteString(writer, strings.Repeat("x", 600<<10))
			_ = writer.Close()
			return
		case "/large":
			_, _ = io.WriteString(w, strings.Repeat("x", 600<<10))
			return
		case "/exact":
			_, _ = io.WriteString(w, strings.Repeat("x", 32))
			return
		case "/body-timeout":
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		case "/header-timeout":
			<-r.Context().Done()
			return
		}
		_, _ = io.WriteString(w, "ok")
	})
	p := config.DefaultHTTPFetchPolicy()
	h, url := testFetchForServer(t, srv, p)
	for _, path := range []string{"/large", "/gzip"} {
		out := runFetch(t, h, map[string]any{"url": url + path})
		assert.True(t, out.Truncated)
		assert.Len(t, out.Body, p.MaxTextBytes)
	}
	_, err := h.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": url + "/huge-header"}))
	require.Error(t, err)
	p.MaxResponseBytes = 32
	p.MaxTextBytes = 32
	h, _ = testFetchForServer(t, srv, p)
	out := runFetch(t, h, map[string]any{"url": url + "/exact"})
	assert.False(t, out.Truncated)
	assert.Len(t, out.Body, 32)
	out = runFetch(t, h, map[string]any{"url": url + "/large"})
	assert.True(t, out.Truncated)
	assert.Len(t, out.Body, 32)
	p.MaxTimeoutMS = 60
	h, _ = testFetchForServer(t, srv, p)
	for _, path := range []string{"/header-timeout", "/body-timeout"} {
		start := time.Now()
		_, err = h.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": url + path, "timeout_ms": 600000}))
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Less(t, time.Since(start), time.Second)
	}
	start := time.Now()
	_, err = h.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": url + "/header-timeout", "timeout_ms": 10}))
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = h.InvokableRun(ctx, jsonArgs(map[string]any{"url": url}))
	assert.ErrorIs(t, err, context.Canceled)
}

func TestHTTPFetchOriginalContextBoundsDNS(t *testing.T) {
	for _, cancelEarly := range []bool{false, true} {
		t.Run(map[bool]string{false: "deadline", true: "cancel"}[cancelEarly], func(t *testing.T) {
			entered := make(chan struct{})
			stopped := make(chan struct{})
			p := config.DefaultHTTPFetchPolicy()
			p.MaxTimeoutMS = 40
			h := newHTTPFetchToolWithNetwork(p, fetchNetwork{
				lookup: func(ctx context.Context, _ string) ([]netip.Addr, error) {
					close(entered)
					deadline, ok := ctx.Deadline()
					assert.True(t, ok)
					assert.LessOrEqual(t, time.Until(deadline), 40*time.Millisecond)
					<-ctx.Done()
					close(stopped)
					return nil, ctx.Err()
				},
				dial: func(context.Context, string, string) (net.Conn, error) { t.Error("unexpected dial"); return nil, nil },
			})
			t.Cleanup(h.client.CloseIdleConnections)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() { _, err := h.InvokableRun(ctx, `{"url":"https://example.com"}`); result <- err }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("resolver never entered")
			}
			if cancelEarly {
				cancel()
			}
			select {
			case err := <-result:
				if cancelEarly {
					assert.ErrorIs(t, err, context.Canceled)
				} else {
					assert.ErrorIs(t, err, context.DeadlineExceeded)
				}
			case <-time.After(time.Second):
				t.Fatal("request was not canceled")
			}
			select {
			case <-stopped:
			case <-time.After(time.Second):
				t.Fatal("transport detached DNS from the request context")
			}
		})
	}
}

func TestHTTPFetchDeadlineAcrossRedirects(t *testing.T) {
	var hits atomic.Int32
	srv := newFetchServer(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		select {
		case <-r.Context().Done():
			return
		case <-time.After(25 * time.Millisecond):
		}
		http.Redirect(w, r, "/again", 302)
	})
	p := config.DefaultHTTPFetchPolicy()
	p.MaxTimeoutMS = 40
	h, url := testFetchForServer(t, srv, p)
	_, err := h.InvokableRun(context.Background(), jsonArgs(map[string]any{"url": url}))
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.LessOrEqual(t, hits.Load(), int32(2))
}
