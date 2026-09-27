package filestore

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocal_SaveWritesFileAndHashes(t *testing.T) {
	store, err := NewLocal(t.TempDir())
	require.NoError(t, err)

	stored, err := store.Save(context.Background(), "rcpt_1.txt", strings.NewReader("hello"))
	require.NoError(t, err)

	content, err := os.ReadFile(stored.Path)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(content))
	assert.Equal(t, int64(5), stored.SizeBytes)
	assert.Equal(t, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", stored.SHA256)
}

func TestLocal_SaveRefusesToOverwrite(t *testing.T) {
	store, err := NewLocal(t.TempDir())
	require.NoError(t, err)

	_, err = store.Save(context.Background(), "same.txt", strings.NewReader("a"))
	require.NoError(t, err)
	_, err = store.Save(context.Background(), "same.txt", strings.NewReader("b"))
	assert.Error(t, err)
}

func TestLocal_SaveStripsDirectories(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocal(dir)
	require.NoError(t, err)

	stored, err := store.Save(context.Background(), "../../escape.txt", strings.NewReader("x"))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(stored.Path, dir))
}

func TestLocal_RemoveDeletesSavedFile(t *testing.T) {
	store, err := NewLocal(t.TempDir())
	require.NoError(t, err)
	stored, err := store.Save(context.Background(), "gone.txt", strings.NewReader("x"))
	require.NoError(t, err)

	require.NoError(t, store.Remove(context.Background(), stored.Path))

	_, err = os.Stat(stored.Path)
	assert.True(t, os.IsNotExist(err))
}
