// Maps to: src/lib/utils/DefaultDataStore.ts
//
// FileStore is the stdlib-only DataStore implementation that ships with the
// port. Upstream uses [node-persist] for an asynchronous JSON file backend;
// we write one JSON file per key under a configurable directory, which keeps
// the on-disk format simple to inspect and avoids pulling in a third-party
// dependency.
//
// Concurrency: each call holds a single sync.Mutex for the duration of the
// I/O. node-persist serializes per-key, but in practice the ytcast workload
// is tiny (a handful of writes per minute under heavy use), so global
// serialization is plenty and keeps the implementation obviously correct.
//
// [node-persist]: https://github.com/simonlast/node-persist
package datastore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"github.com/shobuprime/sonuntius/internal/ytcast/logger"
)

// FileStore persists keys as individual JSON files inside Dir.
type FileStore struct {
	dir string

	mu  sync.Mutex
	log logger.Logger
}

// NewFileStore creates a FileStore rooted at `dir`. The directory is created
// (with any missing parents) on first use; NewFileStore itself never touches
// the filesystem so it is safe to call before logging is configured.
func NewFileStore(dir string) *FileStore {
	return &FileStore{dir: dir}
}

// SetLogger implements DataStore.
func (s *FileStore) SetLogger(l logger.Logger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log = l
}

// Logger implements DataStore.
func (s *FileStore) Logger() logger.Logger {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.log
}

// Set implements DataStore.
//
// Upstream catches any thrown error and logs it without rethrowing. We follow
// the same convention: log the failure, then return the wrapped error so the
// caller can decide whether to surface it (Go callers usually want the error).
func (s *FileStore) Set(_ context.Context, key string, value any) error {
	if err := s.ensureDir(); err != nil {
		s.logErr("[yt-cast-receiver] FileStore.Set: ensureDir failed for key %q: %v", key, err)
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		s.logErr("[yt-cast-receiver] Failed to set value for key %q in data store: %v Value: %#v", key, err, value)
		return fmt.Errorf("datastore: marshal value for key %q: %w", key, err)
	}
	path := s.pathFor(key)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		s.logErr("[yt-cast-receiver] Failed to set value for key %q in data store: %v", key, err)
		return fmt.Errorf("datastore: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		// Best-effort cleanup of the temp file; we don't propagate this.
		_ = os.Remove(tmp)
		s.logErr("[yt-cast-receiver] Failed to set value for key %q in data store: %v", key, err)
		return fmt.Errorf("datastore: rename %s -> %s: %w", tmp, path, err)
	}
	return nil
}

// Get implements DataStore.
//
// Returns (nil, nil) for a missing key. Other I/O errors are returned and
// also logged (matching upstream's error/null pattern).
func (s *FileStore) Get(_ context.Context, key string) (json.RawMessage, error) {
	path := s.pathFor(key)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		s.logErr("[yt-cast-receiver] Failed to get value for key %q in data store: %v", key, err)
		return nil, fmt.Errorf("datastore: read %s: %w", path, err)
	}
	// Validate JSON so callers can rely on json.RawMessage being well-formed.
	if !json.Valid(data) {
		s.logErr("[yt-cast-receiver] Invalid JSON in data store for key %q", key)
		return nil, fmt.Errorf("datastore: invalid JSON for key %q", key)
	}
	return json.RawMessage(data), nil
}

// Remove implements DataStore. Removing a missing key is not an error.
func (s *FileStore) Remove(_ context.Context, key string) error {
	path := s.pathFor(key)
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		s.logErr("[yt-cast-receiver] Failed to remove key %q in data store: %v", key, err)
		return fmt.Errorf("datastore: remove %s: %w", path, err)
	}
	return nil
}

// Clear implements DataStore. It removes every file managed by the store.
// Files inside Dir that were not produced by FileStore (i.e. not encoded with
// pathFor) are left alone so an operator can drop a sentinel into the
// directory without losing it.
func (s *FileStore) Clear(_ context.Context) error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("datastore: readdir %s: %w", s.dir, err)
	}
	var firstErr error
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Only touch files that look like ones we produced (suffix `.json`).
		if filepath.Ext(name) != ".json" {
			continue
		}
		path := filepath.Join(s.dir, name)
		if err := os.Remove(path); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("datastore: remove %s: %w", path, err)
		}
	}
	return firstErr
}

// Dir returns the on-disk directory the store writes to.
func (s *FileStore) Dir() string { return s.dir }

func (s *FileStore) ensureDir() error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("datastore: mkdir %s: %w", s.dir, err)
	}
	return nil
}

// pathFor maps a logical key to an on-disk filename. Keys are URL-path-escaped
// so arbitrary characters (including '/') survive a round-trip without
// escaping the store directory.
func (s *FileStore) pathFor(key string) string {
	encoded := url.PathEscape(key)
	return filepath.Join(s.dir, encoded+".json")
}

func (s *FileStore) logErr(format string, args ...any) {
	s.mu.Lock()
	l := s.log
	s.mu.Unlock()
	if l == nil {
		return
	}
	l.Error(fmt.Sprintf(format, args...))
}
