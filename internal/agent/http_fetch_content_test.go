package agent

import (
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchReadableContent(t *testing.T) {
	base, err := url.Parse("https://example.com/articles/one")
	require.NoError(t, err)
	cases := []struct{ name, content, kind, body, title string }{
		{"HTML", `<html><head><title> Research &amp; results </title><style>secret style</style></head><body><header>header secret</header><nav>navigation secret</nav><script>secret script</script><form>secret form</form><aside>secret aside</aside><div class="cookie-banner">cookie secret</div><div hidden>hidden secret</div><main><h1>Heading</h1><p>Useful <b>facts</b>.</p><ul><li>First</li><li>Second</li></ul><a href="/source">Evidence</a><footer>footer secret</footer></main></body></html>`, "text/html; charset=utf-8", "Heading", "Research & results"},
		{"malformed HTML", `<title>Broken</title><p>Hello <b>world<p>Still useful<ul><li>One<li>Two`, "text/html; charset=utf-8", "Hello", "Broken"},
		{"Windows HTML", "<title>Caf\xe9</title><p>R\xe9sum\xe9", "text/html; charset=windows-1252", "Résumé", "Café"},
		{"meta charset HTML", "<meta charset=windows-1252><title>Caf\xe9</title><p>R\xe9sum\xe9", "text/html", "Résumé", "Café"},
		{"plain text", "hello\nworld", "text/plain; charset=utf-8", "hello\nworld", ""},
		{"legacy plain text", "caf\xe9", "text/plain; charset=iso-8859-1", "café", ""},
		{"JSON", `{"key":"value"}`, "application/json", `{"key":"value"}`, ""},
		{"JSON suffix", `{"key":"value"}`, "application/ld+json", `{"key":"value"}`, ""},
		{"sniff HTML", "<!DOCTYPE html><html><title>Sniffed</title><p>Visible</p></html>", "", "Visible", "Sniffed"},
		{"sniff plain", "plain", "", "plain", ""},
		{"XML", "<feed>content</feed>", "application/rss+xml", "<feed>content</feed>", ""},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			body, title, kind, cut, err := fetchReadableContent([]byte(tt.content), tt.kind, base, 524288, 65536)
			require.NoError(t, err)
			assert.Contains(t, body, tt.body)
			assert.Equal(t, tt.title, title)
			assert.NotEmpty(t, kind)
			assert.False(t, cut)
			if tt.name == "HTML" {
				assert.NotContains(t, body, "secret")
				assert.Contains(t, body, "- First")
				assert.Contains(t, body, "- Second")
				assert.Contains(t, body, "https://example.com/source")
			}
		})
	}
	for _, kind := range []string{"application/pdf", "image/png", "image/svg+xml", "application/octet-stream", "text/html; bad"} {
		body, _, _, _, err := fetchReadableContent([]byte("secret binary"), kind, base, 524288, 65536)
		require.NoError(t, err)
		assert.Contains(t, body, "Unsupported content type")
		assert.NotContains(t, body, "secret")
	}
	_, _, kind, _, err := fetchReadableContent([]byte{0, 1, 2, 3}, "", base, 524288, 65536)
	require.NoError(t, err)
	assert.Equal(t, "application/octet-stream", kind)
	_, _, _, _, err = fetchReadableContent([]byte("plain"), "text/plain; charset=not-a-real-charset", base, 524288, 65536)
	assert.ErrorContains(t, err, "unsupported_charset")
}

func TestFetchContentLimits(t *testing.T) {
	base, _ := url.Parse("https://example.com")
	for _, kind := range []string{"text/plain; charset=utf-8", "text/html; charset=utf-8"} {
		body, _, _, cut, err := fetchReadableContent([]byte(strings.Repeat("é", 100)), kind, base, 500, 11)
		require.NoError(t, err)
		assert.True(t, cut)
		assert.LessOrEqual(t, len(body), 11)
		assert.True(t, utf8.ValidString(body))
	}
	// Charset expansion has a separate limit; title is also bounded.
	body, _, _, cut, err := fetchReadableContent([]byte(strings.Repeat("\xe9", 32)), "text/plain; charset=windows-1252", base, 32, 64)
	require.NoError(t, err)
	assert.True(t, cut)
	assert.Len(t, body, 32)
	_, title, _, cut, err := fetchReadableContent([]byte("<title>"+strings.Repeat("a", 2000)+"</title><p>content"), "text/html", base, 5000, 5000)
	require.NoError(t, err)
	assert.True(t, cut)
	assert.Len(t, title, 1024)
}

func TestFetchReadableEmptyContent(t *testing.T) {
	base, err := url.Parse("https://example.com/")
	require.NoError(t, err)
	for _, tt := range []struct{ contentType, wantKind string }{
		{"text/html; charset=utf-8", "text/html"},
		{"text/html", "text/html"},
		{"text/plain; charset=not-a-real-charset", "text/plain"},
		{"application/pdf", "application/pdf"},
		{"text/html; bad", "unknown"},
		{"", ""},
	} {
		t.Run(tt.contentType, func(t *testing.T) {
			body, title, kind, truncated, err := fetchReadableContent(nil, tt.contentType, base, 524288, 65536)
			require.NoError(t, err)
			assert.Empty(t, body)
			assert.Empty(t, title)
			assert.Equal(t, tt.wantKind, kind)
			assert.False(t, truncated)
		})
	}
}
