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
