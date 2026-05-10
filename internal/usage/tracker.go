package usage

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type Tracker interface {
	Record(userID, sessionID, component, model string, usage TokenUsage)
}

type record struct {
	Timestamp        string  `json:"timestamp"`
	Component        string  `json:"component"`
	Model            string  `json:"model"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	SessionID        string  `json:"session_id"`
}

type FileTracker struct {
	storageRoot string
	counter     *Counter
}

func NewFileTracker(storageRoot string, counter *Counter) *FileTracker {
	return &FileTracker{storageRoot: storageRoot, counter: counter}
}

func (t *FileTracker) Record(userID, sessionID, component, model string, u TokenUsage) {
	cost := CostUSD(model, u.PromptTokens, u.CompletionTokens)
	r := record{
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
		Component:        component,
		Model:            model,
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
		CostUSD:          cost,
		SessionID:        sessionID,
	}
	slog.Info("usage",
		"component", component, "model", model,
		"prompt_tokens", u.PromptTokens, "completion_tokens", u.CompletionTokens,
		"total_tokens", u.TotalTokens, "cost_usd", cost,
		"session_id", sessionID, "user_id", userID,
	)
	if err := t.appendRecord(userID, r); err != nil {
		slog.Error("writing usage record", "error", err)
	}
	t.counter.add(u, cost)
}

func (t *FileTracker) appendRecord(userID string, r record) error {
	dir := filepath.Join(t.storageRoot, "user", userID, "metrics")
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("creating metrics dir: %w", err)
	}
	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshaling record: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "costs.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("opening cost ledger: %w", err)
	}
	_, err = f.Write(append(data, '\n'))
	if closeErr := f.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
}
