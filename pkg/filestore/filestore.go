// Package filestore saves uploaded files. Production storage is out of scope;
// Store is the seam where an object store would go.
package filestore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Stored describes a saved file.
type Stored struct {
	Path      string
	SizeBytes int64
	SHA256    string
}

// Store saves a file under name.
type Store interface {
	Save(ctx context.Context, name string, r io.Reader) (Stored, error)
}

// Local stores files in a directory on local disk.
type Local struct {
	dir string
}

// NewLocal creates dir if needed.
func NewLocal(dir string) (*Local, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("filestore: creating %s: %w", dir, err)
	}
	return &Local{dir: dir}, nil
}

var _ Store = (*Local)(nil)

// Save writes r to dir/name, hashing as it goes. It never overwrites, and
// filepath.Base keeps a hostile name inside dir.
func (l *Local) Save(_ context.Context, name string, r io.Reader) (Stored, error) {
	path := filepath.Join(l.dir, filepath.Base(name))

	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return Stored{}, fmt.Errorf("filestore: creating %s: %w", path, err)
	}

	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, hash), r)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(path)
		if copyErr != nil {
			return Stored{}, fmt.Errorf("filestore: writing %s: %w", path, copyErr)
		}
		return Stored{}, fmt.Errorf("filestore: closing %s: %w", path, closeErr)
	}

	return Stored{Path: path, SizeBytes: size, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}
