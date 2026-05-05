package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionStore_CurrentSession_NoneExists(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)

	id, err := store.CurrentSession("user1")

	require.NoError(t, err)
	assert.Len(t, id, 19)
}

func TestSessionStore_CurrentSession_ReturnsLatest(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	sessDir := filepath.Join(dir, "user1", "sessions")
	require.NoError(t, os.MkdirAll(sessDir, 0750))
	require.NoError(t, os.WriteFile(filepath.Join(sessDir, "0000000001000000000.jsonl"), []byte{}, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(sessDir, "0000000002000000000.jsonl"), []byte{}, 0600))

	id, err := store.CurrentSession("user1")

	require.NoError(t, err)
	assert.Equal(t, "0000000002000000000", id)
}

func TestSessionStore_CurrentSession_StableBeforeFirstAppend(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)

	id1, err := store.CurrentSession("user1")
	require.NoError(t, err)

	require.NoError(t, store.Append("user1", id1, "user", "hello"))

	id2, err := store.CurrentSession("user1")
	require.NoError(t, err)

	assert.Equal(t, id1, id2)
}

func TestSessionStore_ReadLastMessages_ReturnsLastN(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	sessID := fmt.Sprintf("%019d", int64(1000000000))
	require.NoError(t, store.Append("user1", sessID, "user", "hello"))
	require.NoError(t, store.Append("user1", sessID, "assistant", "world"))
	require.NoError(t, store.Append("user1", sessID, "user", "bye"))

	msgs, err := store.ReadLastMessages("user1", sessID, 2)

	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, schema.Assistant, msgs[0].Role)
	assert.Equal(t, "world", msgs[0].Content)
	assert.Equal(t, schema.User, msgs[1].Role)
	assert.Equal(t, "bye", msgs[1].Content)
}

func TestSessionStore_ReadLastMessages_FewerThanWindow(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	sessID := fmt.Sprintf("%019d", int64(1000000000))
	require.NoError(t, store.Append("user1", sessID, "user", "hello"))

	msgs, err := store.ReadLastMessages("user1", sessID, 20)

	require.NoError(t, err)
	assert.Len(t, msgs, 1)
}

func TestSessionStore_ReadLastMessages_NoFile(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)

	msgs, err := store.ReadLastMessages("user1", "no-such-session", 20)

	require.NoError(t, err)
	assert.Empty(t, msgs)
}

func TestSessionStore_Append_ZeroPaddedFilename(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	sessID := fmt.Sprintf("%019d", int64(1746500000000000000))

	require.NoError(t, store.Append("user1", sessID, "user", "hello"))

	sessDir := filepath.Join(dir, "user1", "sessions")
	entries, err := os.ReadDir(sessDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, sessID+".jsonl", entries[0].Name())
}

func TestSessionStore_ReadCursor_ReturnsZeroWhenMissing(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)

	cursor, err := store.ReadCursor("user1", "sess-1")

	require.NoError(t, err)
	assert.Equal(t, 0, cursor)
}

func TestSessionStore_WriteCursor_PersistsCursor(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	sessDir := filepath.Join(dir, "user1", "sessions")
	require.NoError(t, os.MkdirAll(sessDir, 0750))

	require.NoError(t, store.WriteCursor("user1", "sess-1", 42))

	cursor, err := store.ReadCursor("user1", "sess-1")
	require.NoError(t, err)
	assert.Equal(t, 42, cursor)
}

func TestSessionStore_ReadFrom_ReturnsMessagesFromOffset(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	sessID := "sess-1"
	require.NoError(t, store.Append("user1", sessID, "user", "msg1"))
	require.NoError(t, store.Append("user1", sessID, "assistant", "msg2"))
	require.NoError(t, store.Append("user1", sessID, "user", "msg3"))

	msgs, total, err := store.ReadFrom("user1", sessID, 1)

	require.NoError(t, err)
	assert.Equal(t, 3, total)
	require.Len(t, msgs, 2)
	assert.Equal(t, "msg2", msgs[0].Content)
	assert.Equal(t, "msg3", msgs[1].Content)
}

func TestSessionStore_ReadFrom_ReturnsEmptyWhenCursorAtEOF(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	sessID := "sess-1"
	require.NoError(t, store.Append("user1", sessID, "user", "msg1"))
	require.NoError(t, store.Append("user1", sessID, "assistant", "msg2"))

	msgs, total, err := store.ReadFrom("user1", sessID, 2)

	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Empty(t, msgs)
}

func TestSessionStore_ReadFrom_NoFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)

	msgs, total, err := store.ReadFrom("user1", "no-such-session", 0)

	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, msgs)
}
