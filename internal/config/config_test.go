package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	t.Setenv("OLLAMA_BASE_URL", "")
	t.Setenv("SESSION_TIMEOUT_MINUTES", "")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "gpt-5.5", cfg.OpenAIModel)
	assert.Equal(t, "http://localhost:11434", cfg.OllamaBaseURL)
	assert.Equal(t, 30, cfg.SessionTimeoutMinutes)
}

func TestLoad_customValues(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("TAVILY_API_KEY", "tvly-test")
	t.Setenv("OPENAI_MODEL", "gpt-4o")
	t.Setenv("OLLAMA_BASE_URL", "http://ollama:11434")
	t.Setenv("SESSION_TIMEOUT_MINUTES", "60")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "sk-test", cfg.OpenAIAPIKey)
	assert.Equal(t, "tvly-test", cfg.TavilyAPIKey)
	assert.Equal(t, "gpt-4o", cfg.OpenAIModel)
	assert.Equal(t, "http://ollama:11434", cfg.OllamaBaseURL)
	assert.Equal(t, 60, cfg.SessionTimeoutMinutes)
}
