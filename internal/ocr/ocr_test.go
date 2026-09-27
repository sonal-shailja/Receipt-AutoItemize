package ocr

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fixtures = "../../fixtures/task-a"

func TestStub_TextUploadReturnsFileContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rcpt_1.txt")
	require.NoError(t, os.WriteFile(path, []byte("TOTAL 1.00\n"), 0o600))

	text, err := NewStub(fixtures).Extract(context.Background(),
		File{Name: "mine.txt", ContentType: "text/plain", Path: path})

	require.NoError(t, err)
	assert.Equal(t, "TOTAL 1.00\n", text)
}

func TestStub_ImageMapsToFixtureByBaseName(t *testing.T) {
	text, err := NewStub(fixtures).Extract(context.Background(),
		File{Name: "receipt-clean.png", ContentType: "image/png", Path: "/does/not/matter"})

	require.NoError(t, err)
	assert.Contains(t, text, "Cafe Mitte")
}

func TestStub_UnknownImageHasNoText(t *testing.T) {
	_, err := NewStub(fixtures).Extract(context.Background(),
		File{Name: "holiday.jpg", ContentType: "image/jpeg", Path: "/x"})

	assert.ErrorIs(t, err, ErrNoText)
}

func TestStub_EmptyTextHasNoText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.txt")
	require.NoError(t, os.WriteFile(path, []byte("  \n"), 0o600))

	_, err := NewStub(fixtures).Extract(context.Background(),
		File{Name: "empty.txt", ContentType: "text/plain", Path: path})

	assert.ErrorIs(t, err, ErrNoText)
}

func TestStub_Name(t *testing.T) {
	assert.Equal(t, "stub", NewStub(fixtures).Name())
}
