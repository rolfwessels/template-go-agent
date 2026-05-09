package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type FileStore struct {
	dir string
}

func NewFileStore(dir string) *FileStore {
	return &FileStore{dir: dir}
}

func (s *FileStore) Save(_ context.Context, userID, date string, fact Fact) error {
	memDir := s.MemoryDir(userID)
	if err := os.MkdirAll(memDir, 0750); err != nil {
		return fmt.Errorf("creating memory dir: %w", err)
	}
	dailyDir := filepath.Join(memDir, "daily")
	if err := os.MkdirAll(dailyDir, 0750); err != nil {
		return fmt.Errorf("creating daily dir: %w", err)
	}
	if err := s.appendFact(filepath.Join(dailyDir, date+".md"), fact.Content); err != nil {
		return err
	}
	if fact.Kind == KindGeneral {
		return s.appendFact(filepath.Join(memDir, "general.md"), fact.Content)
	}
	return nil
}

func (s *FileStore) ReadDailyFile(userID, date string) (string, error) {
	path := filepath.Join(s.MemoryDir(userID), "daily", date+".md")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading daily file: %w", err)
	}
	return string(data), nil
}

func (s *FileStore) appendFact(path, content string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		name := strings.TrimSuffix(filepath.Base(path), ".md")
		header := fmt.Sprintf("# %s\n\n", name)
		if err := os.WriteFile(path, []byte(header), 0600); err != nil {
			return fmt.Errorf("creating file: %w", err)
		}
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "- %s\n", strings.TrimSpace(content))
	return err
}

// UpdateIndex rebuilds MEMORY.md with clickable Markdown links.
// summaries contains new or updated one-line descriptions keyed by relative path
// (e.g. "daily/2026-04-18.md"). Existing descriptions in MEMORY.md are preserved
// for any file not present in summaries.
func (s *FileStore) UpdateIndex(userID string, summaries map[string]string) error {
	memDir := s.MemoryDir(userID)
	if err := os.MkdirAll(memDir, 0750); err != nil {
		return fmt.Errorf("creating memory dir: %w", err)
	}
	merged := s.parseIndexSummaries(filepath.Join(memDir, "MEMORY.md"))
	for k, v := range summaries {
		merged[k] = v
	}
	var sb strings.Builder
	sb.WriteString("# Long-term Memory Index\n\n")

	if _, err := os.Stat(filepath.Join(memDir, "general.md")); err == nil {
		desc := merged["general.md"]
		if desc == "" {
			desc = "stable facts and preferences"
		}
		sb.WriteString(fmt.Sprintf("- [general.md](general.md)"+indexSep+"%s\n", desc))
	}

	dailyDir := filepath.Join(memDir, "daily")
	if des, err := os.ReadDir(dailyDir); err == nil {
		var names []string
		for _, de := range des {
			if strings.HasSuffix(de.Name(), ".md") {
				names = append(names, de.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			rel := "daily/" + name
			date := strings.TrimSuffix(name, ".md")
			desc := merged[rel]
			if desc == "" {
				desc = "facts from " + date
			}
			sb.WriteString(fmt.Sprintf("- [%s](%s)"+indexSep+"%s\n", rel, rel, desc))
		}
	}

	return os.WriteFile(filepath.Join(memDir, "MEMORY.md"), []byte(sb.String()), 0600)
}

const indexSep = " — "

func (s *FileStore) parseIndexSummaries(path string) map[string]string {
	result := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "- [") {
			continue
		}
		closeBracket := strings.Index(line, "]")
		if closeBracket < 3 {
			continue
		}
		rel := line[3:closeBracket]
		dashIdx := strings.Index(line, indexSep)
		if dashIdx < 0 {
			continue
		}
		if desc := strings.TrimSpace(line[dashIdx+len(indexSep):]); desc != "" {
			result[rel] = desc
		}
	}
	return result
}

func (s *FileStore) AllAsContext(_ context.Context, userID string) (string, error) {
	memDir := s.MemoryDir(userID)
	var sb strings.Builder
	for _, name := range []string{"MEMORY.md", "general.md"} {
		data, err := os.ReadFile(filepath.Join(memDir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", name, err)
		}
		sb.Write(data)
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func (s *FileStore) MemoryDir(userID string) string {
	return filepath.Join(s.dir, "user", userID, "memory")
}
