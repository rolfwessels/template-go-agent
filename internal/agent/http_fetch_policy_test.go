package agent

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"sync/atomic"
	"testing"

	"github.com/rolfwessels/template-go-agent/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicFetchIP(t *testing.T) {
	for _, value := range []string{"8.8.8.8", "1.1.1.1", "93.184.216.34", "2606:4700:4700::1111", "2001:4860:4860::8888", "::ffff:8.8.8.8"} {
		t.Run(value, func(t *testing.T) { assert.True(t, publicFetchIP(netip.MustParseAddr(value))) })
	}
	// Exercise every documented prefix plus address-class checks and zones.
	for _, prefix := range forbiddenPrefixes {
		t.Run(prefix.String(), func(t *testing.T) { assert.False(t, publicFetchIP(prefix.Addr())) })
	}
	for _, value := range []string{"::1", "::", "::ffff:127.0.0.1", "::ffff:10.0.0.1", "::ffff:100.100.100.200", "fe80::1%eth0", "255.255.255.255", "169.254.169.254", "168.63.129.16", "4000::1"} {
		assert.False(t, publicFetchIP(netip.MustParseAddr(value)), value)
	}
	assert.False(t, publicFetchIP(netip.Addr{}))
}

func TestValidateFetchURL(t *testing.T) {
	p := config.DefaultHTTPFetchPolicy()
	for _, value := range []string{"http://example.com", "https://Example.COM.:443/path?q=x#fragment", "http://8.8.8.8:80", "https://[2606:4700:4700::1111]", "http://[::ffff:8.8.8.8]"} {
		_, err := validateFetchURL(value, p)
		assert.NoError(t, err, value)
	}
	for _, value := range []string{
		"//example.com", "example.com", "file:///etc/passwd", "ftp://example.com", "http:example.com", "http://user:secret@example.com", "http://@example.com",
		"http://example.com:443", "https://example.com:80", "http://example.com:8080", "http://example.com:", "http://example.com:0", "http://example.com:65536", "http://example.com:+80", "http://example.com:080", "http://example.com:wat",
		"http://127.0.0.1", "http://[::ffff:127.0.0.1]", "http://[fe80::1%25eth0]", "http://2130706433", "http://0177.0.0.1", "http://0x7f000001", "http://127.1", "http://127.0.0.1.",
		"http://localhost.", "http://foo.localhost", "http://router.local", "http://foo.internal", "http://metadata.google.internal", "http://metadata.goog", "http://instance-data", "http://100.100.100.200", "http://168.63.129.16",
		"http://bad_host.com", "http://-bad.com", "http://example..com", "http://example.com..", "http://example.com%2f.evil", "http://éxample.com", "http://[example.com]", "http://2606:4700:4700::1111", "http://example.com\\@evil.com",
	} {
		_, err := validateFetchURL(value, p)
		assert.Error(t, err, value)
	}
	p.AllowedPorts = append(p.AllowedPorts, 8080)
	_, err := validateFetchURL("http://example.com:8080", p)
	require.NoError(t, err)
	p.AllowedHosts = []string{"*.example.com", "8.8.8.8"}
	p.DeniedHosts = []string{"bad.example.com"}
	for _, host := range []string{"a.example.com", "deep.a.example.com", "8.8.8.8"} {
		_, err := validateFetchURL("https://"+host, p)
		assert.NoError(t, err)
	}
	for _, host := range []string{"example.com", "bad.example.com", "a.example.com.evil.com", "fakeexample.com", "10.0.0.1"} {
		_, err := validateFetchURL("https://"+host, p)
		assert.Error(t, err)
	}
	p.AllowedHosts = []string{"localhost", "10.0.0.1", "metadata.goog"}
	for _, host := range p.AllowedHosts {
		_, err := validateFetchURL("http://"+host, p)
		assert.Error(t, err, "allowlist must not override mandatory denials")
	}
}

func TestGuardedDialRejectsAllAnswersBeforeDial(t *testing.T) {
	for _, answers := range [][]netip.Addr{
		{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("10.0.0.1")},
		{netip.MustParseAddr("::ffff:127.0.0.1"), netip.MustParseAddr("8.8.8.8")},
		{netip.MustParseAddr("2606:4700::1111"), netip.MustParseAddr("fe80::1")},
		nil,
	} {
		called := false
		tr := guardedFetchTransport(config.DefaultHTTPFetchPolicy(), fetchNetwork{
			lookup: func(context.Context, string) ([]netip.Addr, error) { return answers, nil },
			dial: func(context.Context, string, string) (net.Conn, error) {
				called = true
				return nil, errors.New("must not dial")
			},
		})
		_, err := tr.DialContext(context.Background(), "tcp", "example.com:80")
		require.Error(t, err)
		assert.False(t, called)
	}
}

func TestGuardedDialPinsIPAndRetriesOnlyApprovedLiterals(t *testing.T) {
	var lookups atomic.Int32
	var dialed []string
	tr := guardedFetchTransport(config.DefaultHTTPFetchPolicy(), fetchNetwork{
		lookup: func(context.Context, string) ([]netip.Addr, error) {
			if lookups.Add(1) > 1 {
				return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
			}
			return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("::ffff:1.1.1.1")}, nil
		},
		dial: func(_ context.Context, _ string, address string) (net.Conn, error) {
			dialed = append(dialed, address)
			return nil, errors.New("failure URL secret")
		},
	})
	_, err := tr.DialContext(context.Background(), "tcp", "example.com:443")
	assert.ErrorContains(t, err, "dial_failed")
	assert.NotContains(t, err.Error(), "secret")
	assert.Equal(t, int32(1), lookups.Load())
	assert.Equal(t, []string{"8.8.8.8:443", "1.1.1.1:443"}, dialed)
	_, err = tr.DialContext(context.Background(), "tcp", "example.com:443")
	assert.ErrorContains(t, err, "address_denied")
	assert.Len(t, dialed, 2)
	require.Nil(t, tr.Proxy)
	require.Nil(t, tr.DialTLSContext)
	assert.Equal(t, int64(64<<10), tr.MaxResponseHeaderBytes)
}

func TestGuardedDNSDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tr := guardedFetchTransport(config.DefaultHTTPFetchPolicy(), fetchNetwork{
		lookup: func(ctx context.Context, _ string) ([]netip.Addr, error) { <-ctx.Done(); return nil, ctx.Err() },
		dial:   func(context.Context, string, string) (net.Conn, error) { t.Fatal("unexpected dial"); return nil, nil },
	})
	_, err := tr.DialContext(ctx, "tcp", "example.com:80")
	require.Error(t, err)
	assert.ErrorIs(t, safeFetchError(ctx, err), context.Canceled)
}

func TestHTTPFetchRedirectPolicyChecks(t *testing.T) {
	h := newHTTPFetchTool(config.DefaultHTTPFetchPolicy())
	mk := func(value, method string) *http.Request {
		req, err := http.NewRequest(method, value, nil)
		require.NoError(t, err)
		return req
	}
	https := mk("https://example.com/start", "GET")
	err := h.client.CheckRedirect(mk("http://example.com/end", "GET"), []*http.Request{https})
	assert.ErrorContains(t, err, "downgrade")
	err = h.client.CheckRedirect(mk("https://other.example.com/end", "TRACE"), []*http.Request{https})
	assert.ErrorContains(t, err, "method_denied")
}
