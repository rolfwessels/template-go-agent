package memory

import (
	"context"
	"fmt"
	"log"

	chromem "github.com/philippgille/chromem-go"
)

type VectorStore interface {
	Add(ctx context.Context, entry Entry) error
	Search(ctx context.Context, userID, query string, k int) ([]Entry, error)
}

type ChromemStore struct {
	db    *chromem.DB
	embFn chromem.EmbeddingFunc
}

func NewChromemStore(ctx context.Context, ollamaBaseURL, dataDir string) (*ChromemStore, error) {
	embFn := chromem.NewEmbeddingFuncOllama("nomic-embed-text", ollamaBaseURL)
	if _, err := embFn(ctx, "test"); err != nil {
		return nil, fmt.Errorf("ollama not reachable at %s: %w", ollamaBaseURL, err)
	}
	db, err := chromem.NewPersistentDB(dataDir, false)
	if err != nil {
		return nil, fmt.Errorf("creating vector db: %w", err)
	}
	return &ChromemStore{db: db, embFn: embFn}, nil
}

func NewChromemStoreOrWarn(ctx context.Context, ollamaBaseURL, dataDir string) *ChromemStore {
	cs, err := NewChromemStore(ctx, ollamaBaseURL, dataDir)
	if err != nil {
		log.Printf("warning: vector index unavailable, memory sweep will use Markdown-only: %v", err)
		return nil
	}
	return cs
}

func (s *ChromemStore) Add(ctx context.Context, e Entry) error {
	col, err := s.db.GetOrCreateCollection(e.UserID, nil, s.embFn)
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
	col, err := s.db.GetOrCreateCollection(userID, nil, s.embFn)
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
