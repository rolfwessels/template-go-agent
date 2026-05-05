package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Entry struct {
	ID        string
	UserID    string
	SessionID string
	Content   string
}

type FileStore struct {
	dir string
}

func NewFileStore(dir string) *FileStore {
	return &FileStore{dir: dir}
}

func (s *FileStore) Save(_ context.Context, e Entry) error {
	userDir := filepath.Join(s.dir, e.UserID)
	if err := os.MkdirAll(userDir, 0750); err != nil {
		return fmt.Errorf("creating memory dir: %w", err)
	}
	content := fmt.Sprintf("---\nuser_id: %s\nsession_id: %s\n---\n\n%s\n", e.UserID, e.SessionID, e.Content)
	if err := os.WriteFile(filepath.Join(userDir, e.ID+".md"), []byte(content), 0600); err != nil {
		return fmt.Errorf("writing memory file: %w", err)
	}
	return nil
}

func (s *FileStore) All(_ context.Context, userID string) ([]Entry, error) {
	userDir := filepath.Join(s.dir, userID)
	des, err := os.ReadDir(userDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading memory dir: %w", err)
	}
	var result []Entry
	for _, de := range des {
		if !strings.HasSuffix(de.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(userDir, de.Name()))
		if err != nil {
			continue
		}
		result = append(result, Entry{
			ID:      strings.TrimSuffix(de.Name(), ".md"),
			UserID:  userID,
			Content: extractContent(string(data)),
		})
	}
	return result, nil
}

func (s *FileStore) AllAsContext(ctx context.Context, userID string) (string, error) {
	entries, err := s.All(ctx, userID)
	if err != nil || len(entries) == 0 {
		return "", err
	}
	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString("- ")
		sb.WriteString(e.Content)
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func extractContent(raw string) string {
	parts := strings.SplitN(raw, "---", 3)
	if len(parts) == 3 {
		return strings.TrimSpace(parts[2])
	}
	return strings.TrimSpace(raw)
}
