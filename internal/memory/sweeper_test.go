package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubDistiller struct {
	facts []string
}

func (s *stubDistiller) Distill(_ context.Context, _ []*schema.Message) ([]string, error) {
	return s.facts, nil
}

func TestSweeper_WritesMarkdownFilesOnDestroy(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := NewFileStore(dir)
	distiller := &stubDistiller{facts: []string{"user prefers brevity", "user works in Go"}}
	sweeper := NewSweeper(store, nil, distiller)
	messages := []*schema.Message{
		schema.UserMessage("hello"),
		{Role: schema.Assistant, Content: "hi"},
	}

	// act
	sweeper.OnDestroy(context.Background(), "alice", "sess-1", messages)

	// assert
	entries, err := os.ReadDir(filepath.Join(dir, "alice"))
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestSweeper_CallsVectorStoreForEachFact(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := NewFileStore(dir)
	vs := &stubVectorStore{}
	distiller := &stubDistiller{facts: []string{"fact A", "fact B"}}
	sweeper := NewSweeper(store, vs, distiller)
	messages := []*schema.Message{schema.UserMessage("msg")}

	// act
	sweeper.OnDestroy(context.Background(), "bob", "sess-1", messages)

	// assert
	assert.Len(t, vs.added, 2)
}

func TestSweeper_SkipsEmptyHistory(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := NewFileStore(dir)
	distiller := &stubDistiller{facts: []string{"should not appear"}}
	sweeper := NewSweeper(store, nil, distiller)

	// act
	sweeper.OnDestroy(context.Background(), "carol", "sess-1", nil)

	// assert — no files written
	_, err := os.ReadDir(filepath.Join(dir, "carol"))
	assert.True(t, os.IsNotExist(err))
}

func TestSweeper_SkipsVectorWhenNil(t *testing.T) {
	// arrange
	dir := t.TempDir()
	store := NewFileStore(dir)
	distiller := &stubDistiller{facts: []string{"a fact"}}
	sweeper := NewSweeper(store, nil, distiller) // nil vector store
	messages := []*schema.Message{schema.UserMessage("msg")}

	// act — should not panic
	assert.NotPanics(t, func() {
		sweeper.OnDestroy(context.Background(), "dave", "sess-1", messages)
	})
}
