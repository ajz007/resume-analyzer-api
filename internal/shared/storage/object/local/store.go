package local

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"resume-backend/internal/shared/storage/object"
	"resume-backend/internal/shared/util"
)

// Store implements ObjectStore using the local filesystem.
type Store struct {
	baseDir string
}

// New creates a new local object store rooted at baseDir.
func New(baseDir string) object.ObjectStore {
	return &Store{baseDir: baseDir}
}

// Save writes the reader to disk under the user's namespace with a random prefix.
func (s *Store) Save(ctx context.Context, userId string, fileName string, r io.Reader) (string, int64, string, error) {
	sanitizedName, err := util.SanitizeFileName(fileName)
	if err != nil {
		return "", 0, "", fmt.Errorf("sanitize file name: %w", err)
	}

	storageUserKey := util.HashUserKey(userId)

	if err := ctx.Err(); err != nil {
		return "", 0, "", err
	}

	prefix := randomID()
	finalName := fmt.Sprintf("%s_%s", prefix, sanitizedName)

	dirPath := filepath.Join(s.baseDir, storageUserKey)
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return "", 0, "", fmt.Errorf("mkdir: %w", err)
	}

	fullPath := filepath.Join(dirPath, finalName)
	f, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", 0, "", fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	var sniff [512]byte
	n, readErr := io.ReadFull(r, sniff[:])
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return "", 0, "", fmt.Errorf("read sniff: %w", readErr)
	}

	mimeType := http.DetectContentType(sniff[:n])

	size := int64(0)
	if n > 0 {
		if _, err := f.Write(sniff[:n]); err != nil {
			return "", 0, "", fmt.Errorf("write sniff: %w", err)
		}
		size += int64(n)
	}

	written, err := io.Copy(f, r)
	if err != nil {
		return "", 0, "", fmt.Errorf("write body: %w", err)
	}
	size += written

	relPath := filepath.Join(storageUserKey, finalName)
	return relPath, size, mimeType, nil
}

// Open opens a stored object for reading.
func (s *Store) Open(ctx context.Context, storageKey string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	clean := filepath.Clean(storageKey)
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return nil, fmt.Errorf("invalid storage key")
	}

	fullPath := filepath.Join(s.baseDir, clean)
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// SaveWithKey writes the reader to disk at a specific storage key.
func (s *Store) SaveWithKey(ctx context.Context, storageKey string, contentType string, r io.Reader) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	clean := filepath.Clean(storageKey)
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return 0, fmt.Errorf("invalid storage key")
	}

	fullPath := filepath.Join(s.baseDir, clean)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return 0, fmt.Errorf("mkdir: %w", err)
	}
	f, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	written, err := io.Copy(f, r)
	if err != nil {
		return 0, fmt.Errorf("write body: %w", err)
	}
	_ = contentType
	return written, nil
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
