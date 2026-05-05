package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type SessionStore struct {
	dir string
}

type sessionLine struct {
	Timestamp time.Time `json:"timestamp"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
}

func NewSessionStore(dir string) *SessionStore {
	return &SessionStore{dir: dir}
}

func (s *SessionStore) Append(userID, sessionID, role, content string) error {
	sessDir := filepath.Join(s.dir, userID, "sessions")
	if err := os.MkdirAll(sessDir, 0750); err != nil {
		return fmt.Errorf("creating session dir: %w", err)
	}
	data, err := json.Marshal(sessionLine{Timestamp: time.Now().UTC(), Role: role, Content: content})
	if err != nil {
		return fmt.Errorf("marshalling session line: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(sessDir, sessionID+".jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("opening session file: %w", err)
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}
