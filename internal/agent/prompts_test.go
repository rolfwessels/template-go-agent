package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rolfwessels/template-go-agent/internal/config"
	"github.com/rolfwessels/template-go-agent/internal/memory"
	"github.com/rolfwessels/template-go-agent/prompts"
)

func TestLoadPrompts_EmbeddedDefaults(t *testing.T) {
	for _, tt := range []struct {
		name string
		dir  string
	}{
		{name: "no override"},
		{name: "empty directory", dir: t.TempDir()},
		{name: "missing directory", dir: filepath.Join(t.TempDir(), "missing")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			prompt, err := loadPrompts(tt.dir)

			require.NoError(t, err)
			assert.Equal(t, prompts.Defaults(), prompt)
			assert.Contains(t, prompt, "# Agent Identity")
			assert.Contains(t, prompt, "# Agent Instructions")
		})
	}
}

func TestLoadPrompts_OverridesBothFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "soul.md"), []byte("custom soul"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "instructions.md"), []byte("custom instructions"), 0600))

	prompt, err := loadPrompts(dir)

	require.NoError(t, err)
	assert.Equal(t, "custom soul\n\ncustom instructions", prompt)
}

func TestLoadPrompts_RejectsPartialOverrides(t *testing.T) {
	for _, present := range []string{"soul.md", "instructions.md"} {
		t.Run(present, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, present), []byte("custom"), 0600))

			prompt, err := loadPrompts(dir)

			require.Error(t, err)
			assert.Empty(t, prompt)
			assert.Contains(t, err.Error(), "PROMPTS_DIR")
			assert.Contains(t, err.Error(), dir)
			assert.Contains(t, err.Error(), "both soul.md and instructions.md are required")
			assert.ErrorIs(t, err, os.ErrNotExist)

			a, constructionErr := New(context.Background(), &config.Config{PromptsDir: dir})
			require.Error(t, constructionErr)
			assert.Nil(t, a)
			assert.Equal(t, err.Error(), constructionErr.Error())
		})
	}
}

func TestLoadPrompts_RejectsUnreadableOverrides(t *testing.T) {
	for _, unreadable := range []string{"soul.md", "instructions.md"} {
		t.Run(unreadable, func(t *testing.T) {
			dir := t.TempDir()
			// A directory at the file path fails ReadFile even when tests run as root.
			require.NoError(t, os.Mkdir(filepath.Join(dir, unreadable), 0700))

			prompt, err := loadPrompts(dir)

			require.Error(t, err)
			assert.Empty(t, prompt)
			assert.Contains(t, err.Error(), "PROMPTS_DIR")
			assert.Contains(t, err.Error(), unreadable)
		})
	}
}

func TestNew_WithoutWorkingDirectoryPrompts(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("OPENAI_API_KEY", "test-openai")
	t.Setenv("TAVILY_API_KEY", "test-tavily")
	t.Setenv("PROMPTS_DIR", "")
	_, err := os.Stat("prompts")
	require.ErrorIs(t, err, os.ErrNotExist)
	cfg, err := config.Load()
	require.NoError(t, err)

	// New constructs the model locally; no API calls or real keys are needed.
	a, err := New(context.Background(), cfg, WithExtraInstructions("extra instructions"))
	require.NoError(t, err)
	want := prompts.Defaults() + "\n\n" + memory.Instructions() + "\n\nextra instructions"
	assert.Equal(t, want, a.systemPrompt)

	// Exercise first-message handling with a stub so it cannot call the network.
	a.react = &fakeGenerator{
		msgs: []*schema.Message{schema.AssistantMessage("ok", nil)},
		onGenerate: func(msgs []*schema.Message) {
			require.Len(t, msgs, 2)
			assert.Equal(t, schema.SystemMessage(want), msgs[0])
			assert.Equal(t, schema.UserMessage("hello"), msgs[1])
		},
	}
	reply, err := a.Generate(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, "ok", reply)
}

func TestNew_UsesConfiguredOverrides(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "soul.md"), []byte("custom soul"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "instructions.md"), []byte("custom instructions"), 0600))

	a, err := New(context.Background(), &config.Config{
		HTTPFetch:    config.DefaultHTTPFetchPolicy(),
		OpenAIAPIKey: "test-openai",
		OpenAIModel:  "test-model",
		TavilyAPIKey: "test-tavily",
		PromptsDir:   dir,
	})

	require.NoError(t, err)
	assert.Equal(t, "custom soul\n\ncustom instructions\n\n"+memory.Instructions(), a.systemPrompt)
}
