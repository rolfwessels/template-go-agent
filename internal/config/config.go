package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	OpenAIAPIKey                  string
	OpenAIModel                   string
	TavilyAPIKey                  string
	OllamaBaseURL                 string
	DiscordToken                  string
	SessionTimeoutMinutes         int
	ConversationHistoryWindowSize int
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		OpenAIAPIKey:                  os.Getenv("OPENAI_API_KEY"),
		OpenAIModel:                   envOrDefault("OPENAI_MODEL", "gpt-5.5"),
		TavilyAPIKey:                  os.Getenv("TAVILY_API_KEY"),
		OllamaBaseURL:                 envOrDefault("OLLAMA_BASE_URL", "http://localhost:11434/api"),
		DiscordToken:                  os.Getenv("DISCORD_TOKEN"),
		SessionTimeoutMinutes:         envIntOrDefault("SESSION_TIMEOUT_MINUTES", 30),
		ConversationHistoryWindowSize: envIntOrDefault("CONVERSATION_HISTORY_WINDOW_SIZE", 20),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.OpenAIAPIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is required")
	}
	if c.TavilyAPIKey == "" {
		return fmt.Errorf("TAVILY_API_KEY is required")
	}
	return nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envIntOrDefault(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
