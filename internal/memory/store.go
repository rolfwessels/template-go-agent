package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type FileStore struct {
	dir string
	mu  sync.Mutex // serialize read-modify-write operations within this store
}

func NewFileStore(dir string) *FileStore {
	return &FileStore{dir: dir}
}

func (s *FileStore) Save(_ context.Context, userID, date string, fact Fact) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	memDir := s.MemoryDir(userID)
	if err := os.MkdirAll(memDir, 0750); err != nil {
		return fmt.Errorf("creating memory dir: %w", err)
	}
	dailyDir := filepath.Join(memDir, "daily")
	if err := os.MkdirAll(dailyDir, 0750); err != nil {
		return fmt.Errorf("creating daily dir: %w", err)
	}
	// Persist the general copy first. Otherwise a failed general write leaves a
	// daily copy that tells the distiller to omit the fact on the next sweep.
	if fact.Kind == KindGeneral {
		if err := s.appendFact(filepath.Join(memDir, "general.md"), fact.Content); err != nil {
			return err
		}
	}
	return s.appendFact(filepath.Join(dailyDir, date+".md"), fact.Content)
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
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		name := strings.TrimSuffix(filepath.Base(path), ".md")
		data = []byte(fmt.Sprintf("# %s\n\n", name))
	} else if err != nil {
		return fmt.Errorf("reading fact file: %w", err)
	}
	// Identity is trimmed content within each destination file. Compare complete
	// bullet records, including their delimiters, rather than substrings of facts.
	record := "- " + strings.TrimSpace(content) + "\n"
	if strings.Contains("\n"+string(data), "\n"+record) {
		return nil
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	return writeFileAtomic(path, append(data, record...))
}

// UpdateIndex rebuilds MEMORY.md with clickable Markdown links.
// summaries contains new or updated one-line descriptions keyed by relative path
// (e.g. "daily/2026-04-18.md"). Existing descriptions in MEMORY.md are preserved
// for any file not present in summaries.
func (s *FileStore) UpdateIndex(userID string, summaries map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	memDir := s.MemoryDir(userID)
	if err := os.MkdirAll(memDir, 0750); err != nil {
		return fmt.Errorf("creating memory dir: %w", err)
	}
	merged, err := s.parseIndexSummaries(filepath.Join(memDir, "MEMORY.md"))
	if err != nil {
		return err
	}
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
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking general memory: %w", err)
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
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("listing daily memory: %w", err)
	}

	return writeFileAtomic(filepath.Join(memDir, "MEMORY.md"), []byte(sb.String()))
}

const indexSep = " — "

func (s *FileStore) parseIndexSummaries(path string) (map[string]string, error) {
	result := map[string]string{}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading memory index: %w", err)
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
	return result, nil
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
