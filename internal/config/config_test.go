package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_promptsDir(t *testing.T) {
	// Keep dotenv files out of the unset-environment case.
	t.Chdir(t.TempDir())
	t.Setenv("OPENAI_API_KEY", "test-openai")
	t.Setenv("TAVILY_API_KEY", "test-tavily")
	for _, tt := range []struct {
		name  string
		value string
		unset bool
	}{
		{name: "unset", unset: true},
		{name: "empty"},
		{name: "custom directory", value: "/custom prompt templates"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PROMPTS_DIR", tt.value)
			if tt.unset {
				require.NoError(t, os.Unsetenv("PROMPTS_DIR"))
			}

			cfg, err := Load()

			require.NoError(t, err)
			assert.Equal(t, tt.value, cfg.PromptsDir)
		})
	}
}

func TestLoad_missingOpenAIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("TAVILY_API_KEY", "test-tavily")

	_, err := Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "OPENAI_API_KEY")
}

func TestLoad_missingTavilyKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-openai")
	t.Setenv("TAVILY_API_KEY", "")

	_, err := Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "TAVILY_API_KEY")
}

func TestLoad_defaults(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-openai")
	t.Setenv("TAVILY_API_KEY", "test-tavily")
	t.Setenv("OPENAI_MODEL", "")
	t.Setenv("SESSION_TIMEOUT_MINUTES", "")
	t.Setenv("CONVERSATION_HISTORY_WINDOW_SIZE", "")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "gpt-5.5", cfg.OpenAIModel)
	assert.Equal(t, "none", cfg.OpenAIReasoningEffort)
	assert.Equal(t, 30, cfg.SessionTimeoutMinutes)
	assert.Equal(t, 20, cfg.ConversationHistoryWindowSize)
}

func TestLoad_customValues(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("TAVILY_API_KEY", "tvly-test")
	t.Setenv("OPENAI_MODEL", "gpt-4o")
	t.Setenv("OPENAI_REASONING_EFFORT", "low")
	t.Setenv("SESSION_TIMEOUT_MINUTES", "60")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "sk-test", cfg.OpenAIAPIKey)
	assert.Equal(t, "tvly-test", cfg.TavilyAPIKey)
	assert.Equal(t, "gpt-4o", cfg.OpenAIModel)
	assert.Equal(t, "low", cfg.OpenAIReasoningEffort)
	assert.Equal(t, 60, cfg.SessionTimeoutMinutes)
}

func clearFetchEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"HTTP_FETCH_ENABLED", "HTTP_FETCH_ALLOWED_HOSTS", "HTTP_FETCH_DENIED_HOSTS", "HTTP_FETCH_ALLOWED_PORTS", "HTTP_FETCH_ALLOW_MUTATIONS", "HTTP_FETCH_MUTATION_ALLOWED_HOSTS", "HTTP_FETCH_MAX_TIMEOUT_MS", "HTTP_FETCH_MAX_RESPONSE_BYTES", "HTTP_FETCH_MAX_TEXT_BYTES", "HTTP_FETCH_MAX_REDIRECTS"} {
		t.Setenv(name, "")
		require.NoError(t, os.Unsetenv(name))
	}
}

func TestHTTPFetchPolicyDefaults(t *testing.T) {
	clearFetchEnv(t)
	p, err := loadHTTPFetchPolicy()
	require.NoError(t, err)
	assert.Equal(t, DefaultHTTPFetchPolicy(), p)
	t.Setenv("OPENAI_API_KEY", "test")
	t.Setenv("TAVILY_API_KEY", "test")
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, p, cfg.HTTPFetch)
}

func TestHTTPFetchPolicyCustom(t *testing.T) {
	clearFetchEnv(t)
	for name, value := range map[string]string{
		"HTTP_FETCH_ENABLED": "false", "HTTP_FETCH_ALLOWED_HOSTS": "Example.COM., *.EXAMPLE.org", "HTTP_FETCH_DENIED_HOSTS": "BAD.EXAMPLE.ORG", "HTTP_FETCH_ALLOWED_PORTS": "80,443,8443",
		"HTTP_FETCH_ALLOW_MUTATIONS": "true", "HTTP_FETCH_MUTATION_ALLOWED_HOSTS": "api.example.com", "HTTP_FETCH_MAX_TIMEOUT_MS": "1000", "HTTP_FETCH_MAX_RESPONSE_BYTES": "2048", "HTTP_FETCH_MAX_TEXT_BYTES": "1024", "HTTP_FETCH_MAX_REDIRECTS": "0",
	} {
		t.Setenv(name, value)
	}
	p, err := loadHTTPFetchPolicy()
	require.NoError(t, err)
	assert.False(t, p.Enabled)
	assert.True(t, p.AllowMutations)
	assert.Equal(t, []string{"example.com", "*.example.org"}, p.AllowedHosts)
	assert.Equal(t, []string{"bad.example.org"}, p.DeniedHosts)
	assert.Equal(t, []int{80, 443, 8443}, p.AllowedPorts)
	assert.Equal(t, 1000, p.MaxTimeoutMS)
	assert.Equal(t, 2048, p.MaxResponseBytes)
	assert.Equal(t, 1024, p.MaxTextBytes)
	assert.Zero(t, p.MaxRedirects)
}

func TestHTTPFetchPolicyInvalid(t *testing.T) {
	for name, values := range map[string][]string{
		"HTTP_FETCH_ENABLED": {"", "TRUE", "1", "yes"}, "HTTP_FETCH_ALLOW_MUTATIONS": {"yes", "true"},
		"HTTP_FETCH_ALLOWED_HOSTS": {"example.com,", "https://example.com", "foo.*.example.com", "*", "*.127.0.0.1", "bad_host.com", "example.com:443", "user@example.com"},
		"HTTP_FETCH_DENIED_HOSTS":  {"foo..example.com", "0x7f000001"}, "HTTP_FETCH_MUTATION_ALLOWED_HOSTS": {"*."},
		"HTTP_FETCH_ALLOWED_PORTS":      {"", "0", "65536", "80,", "-1", "443x", "080"},
		"HTTP_FETCH_MAX_TIMEOUT_MS":     {"", "0", "-1", "abc", "+30", "01", "9223372036854775807"},
		"HTTP_FETCH_MAX_RESPONSE_BYTES": {"0", "-1", "oops", "9223372036854775807"}, "HTTP_FETCH_MAX_TEXT_BYTES": {"0", "oops"}, "HTTP_FETCH_MAX_REDIRECTS": {"-1", "5.0", ""},
	} {
		for _, value := range values {
			t.Run(name+"="+value, func(t *testing.T) {
				clearFetchEnv(t)
				t.Setenv(name, value)
				_, err := loadHTTPFetchPolicy()
				require.Error(t, err)
				assert.Contains(t, err.Error(), "HTTP_FETCH_")
				t.Setenv("OPENAI_API_KEY", "test")
				t.Setenv("TAVILY_API_KEY", "test")
				_, err = Load()
				require.Error(t, err, "malformed policy must fail at startup")
			})
		}
	}
}
