package memory

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sync"

	chromem "github.com/philippgille/chromem-go"
)

type VectorStore interface {
	Add(ctx context.Context, entry Entry) error
	Search(ctx context.Context, userID, query string, k int) ([]Entry, error)
}

type ChromemStore struct {
	baseDir string
	embFn   chromem.EmbeddingFunc
	mu      sync.Mutex
	dbs     map[string]*chromem.DB
}

func NewChromemStore(ctx context.Context, ollamaBaseURL, baseDir string) (*ChromemStore, error) {
	embFn := chromem.NewEmbeddingFuncOllama("nomic-embed-text", ollamaBaseURL)
	if _, err := embFn(ctx, "test"); err != nil {
		return nil, fmt.Errorf("ollama not reachable at %s: %w", ollamaBaseURL, err)
	}
	return &ChromemStore{baseDir: baseDir, embFn: embFn, dbs: make(map[string]*chromem.DB)}, nil
}

func NewChromemStoreOrWarn(ctx context.Context, ollamaBaseURL, baseDir string) *ChromemStore {
	cs, err := NewChromemStore(ctx, ollamaBaseURL, baseDir)
	if err != nil {
		log.Printf("warning: vector index unavailable, memory sweep will use Markdown-only: %v", err)
		return nil
	}
	return cs
}

func (s *ChromemStore) dbFor(userID string) (*chromem.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if db, ok := s.dbs[userID]; ok {
		return db, nil
	}
	dir := filepath.Join(s.baseDir, "user", userID, "memory")
	db, err := chromem.NewPersistentDB(dir, false)
	if err != nil {
		return nil, fmt.Errorf("creating vector db for %s: %w", userID, err)
	}
	s.dbs[userID] = db
	return db, nil
}

func (s *ChromemStore) Add(ctx context.Context, e Entry) error {
	db, err := s.dbFor(e.UserID)
	if err != nil {
		return err
	}
	col, err := db.GetOrCreateCollection(e.UserID, nil, s.embFn)
	if err != nil {
		return fmt.Errorf("getting collection: %w", err)
	}
	return col.AddDocument(ctx, chromem.Document{
		ID:       e.ID,
		Content:  e.Content,
		Metadata: map[string]string{"user_id": e.UserID},
	})
}

func (s *ChromemStore) Search(ctx context.Context, userID, query string, k int) ([]Entry, error) {
	db, err := s.dbFor(userID)
	if err != nil {
		return nil, err
	}
	col, err := db.GetOrCreateCollection(userID, nil, s.embFn)
	if err != nil {
		return nil, fmt.Errorf("getting collection: %w", err)
	}
	if col.Count() == 0 {
		return nil, nil
	}
	results, err := col.Query(ctx, query, k, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("querying vector store: %w", err)
	}
	entries := make([]Entry, len(results))
	for i, r := range results {
		entries[i] = Entry{
			ID:      r.ID,
			UserID:  userID,
			Content: r.Content,
		}
	}
	return entries, nil
}
