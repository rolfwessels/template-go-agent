package memory

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
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

func (s *SessionStore) CurrentSession(userID string) (string, error) {
	sessDir := filepath.Join(s.dir, userID, "sessions")
	entries, err := os.ReadDir(sessDir)
	if err != nil {
		if os.IsNotExist(err) {
			return newSessionID(), nil
		}
		return "", fmt.Errorf("reading session dir: %w", err)
	}
	names := jsonlNames(entries)
	if len(names) == 0 {
		return newSessionID(), nil
	}
	sort.Strings(names)
	return names[len(names)-1], nil
}

func (s *SessionStore) ReadLastMessages(userID, sessionID string, n int) ([]*schema.Message, error) {
	path := filepath.Join(s.dir, userID, "sessions", sessionID+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("opening session file: %w", err)
	}
	defer f.Close()

	var lines []sessionLine
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var line sessionLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning session file: %w", err)
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	msgs := make([]*schema.Message, len(lines))
	for i, l := range lines {
		msgs[i] = toSchemaMessage(l)
	}
	return msgs, nil
}

func (s *SessionStore) LoadSession(userID string, windowSize int) (string, []*schema.Message, error) {
	sessionID, err := s.CurrentSession(userID)
	if err != nil {
		return "", nil, err
	}
	msgs, err := s.ReadLastMessages(userID, sessionID, windowSize)
	if err != nil {
		return "", nil, err
	}
	return sessionID, msgs, nil
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

func newSessionID() string {
	return fmt.Sprintf("%019d", time.Now().UnixNano())
}

func jsonlNames(entries []os.DirEntry) []string {
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			names = append(names, strings.TrimSuffix(e.Name(), ".jsonl"))
		}
	}
	return names
}

func toSchemaMessage(l sessionLine) *schema.Message {
	switch l.Role {
	case "user":
		return schema.UserMessage(l.Content)
	default:
		return &schema.Message{Role: schema.RoleType(l.Role), Content: l.Content}
	}
}
