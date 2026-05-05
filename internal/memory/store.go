package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	date := time.Now().UTC().Format("2006-01-02")
	path := filepath.Join(userDir, fmt.Sprintf("%s-%s.md", e.SessionID, date))

	if _, err := os.Stat(path); os.IsNotExist(err) {
		header := fmt.Sprintf("# %s\n\n", date)
		if err := os.WriteFile(path, []byte(header), 0600); err != nil {
			return fmt.Errorf("creating memory file: %w", err)
		}
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("opening memory file: %w", err)
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "- %s\n", strings.TrimSpace(e.Content))
	return err
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
		sb.WriteString(strings.TrimSpace(e.Content))
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func extractContent(raw string) string {
	// strip the `# date\n\n` header and return the bullet list body
	idx := strings.Index(raw, "\n\n")
	if idx >= 0 {
		return strings.TrimSpace(raw[idx+2:])
	}
	return strings.TrimSpace(raw)
}
