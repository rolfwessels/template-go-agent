package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubVectorStore struct {
	added    []Entry
	toReturn []Entry
}

func (s *stubVectorStore) Add(_ context.Context, e Entry) error {
	s.added = append(s.added, e)
	return nil
}

func (s *stubVectorStore) Search(_ context.Context, _, _ string, _ int) ([]Entry, error) {
	return s.toReturn, nil
}

func TestVectorStore_InterfaceContract(t *testing.T) {
	// arrange
	var _ VectorStore = &stubVectorStore{}
	var _ VectorStore = (*ChromemStore)(nil)

	stub := &stubVectorStore{toReturn: []Entry{{ID: "e1", Content: "test fact"}}}
	ctx := context.Background()

	// act
	err := stub.Add(ctx, Entry{ID: "e1", Content: "test fact"})
	results, searchErr := stub.Search(ctx, "user1", "query", 5)

	// assert
	require.NoError(t, err)
	require.NoError(t, searchErr)
	assert.Len(t, stub.added, 1)
	assert.Len(t, results, 1)
	assert.Equal(t, "test fact", results[0].Content)
}
