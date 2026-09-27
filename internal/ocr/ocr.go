// Package ocr turns an uploaded receipt file into raw text. Providers sit
// behind one interface so a real vendor (a vision model, Veryfi, ...) can
// replace the stub without touching the service; the provider name is recorded
// on every OCR result.
package ocr

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoText means the provider ran but found nothing to read.
var ErrNoText = errors.New("ocr: no text could be extracted")

// File is the stored upload a provider reads.
type File struct {
	// Name is the original file name from the upload.
	Name        string
	ContentType string
	// Path is where the upload is stored on disk.
	Path string
}

// Provider extracts raw text from a receipt file.
type Provider interface {
	Name() string
	Extract(ctx context.Context, file File) (string, error)
}

// StubName identifies the stub in config and on OCR results.
const StubName = "stub"

// Stub stands in for a real OCR engine. A text upload is treated as already
// OCR'd and returned as-is; an image or PDF is matched by base name to a
// fixture, so receipt-clean.png reads fixtures/task-a/receipt-clean.txt.
type Stub struct {
	fixturesDir string
}

// NewStub builds a stub that looks up non-text uploads in fixturesDir.
func NewStub(fixturesDir string) *Stub {
	return &Stub{fixturesDir: fixturesDir}
}

// Name implements Provider.
func (s *Stub) Name() string { return StubName }

// Extract implements Provider.
func (s *Stub) Extract(_ context.Context, file File) (string, error) {
	path := file.Path
	if !isText(file) {
		// filepath.Base strips any directory from the client-supplied name, so
		// the lookup cannot escape fixturesDir.
		base := strings.TrimSuffix(filepath.Base(file.Name), filepath.Ext(file.Name))
		path = filepath.Join(s.fixturesDir, base+".txt")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("%w: no fixture text for %q", ErrNoText, file.Name)
		}
		return "", fmt.Errorf("ocr: reading %s: %w", path, err)
	}

	text := string(content)
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("%w: %q is empty", ErrNoText, file.Name)
	}
	return text, nil
}

func isText(file File) bool {
	return strings.HasPrefix(file.ContentType, "text/") ||
		strings.EqualFold(filepath.Ext(file.Name), ".txt")
}
