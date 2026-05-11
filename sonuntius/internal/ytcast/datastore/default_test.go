// Maps to: N/A — Go-only tests for the FileStore port.
package datastore

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestFileStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewFileStore(filepath.Join(dir, "store"))
	ctx := context.Background()

	if err := s.Set(ctx, "alpha", map[string]any{"hello": "world", "n": 42}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	raw, err := s.Get(ctx, "alpha")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["hello"] != "world" {
		t.Fatalf("expected hello=world, got %v", got)
	}
}

func TestFileStoreMissingKeyReturnsNilNil(t *testing.T) {
	s := NewFileStore(t.TempDir())
	raw, err := s.Get(context.Background(), "absent")
	if err != nil {
		t.Fatalf("expected nil error for missing key, got %v", err)
	}
	if raw != nil {
		t.Fatalf("expected nil RawMessage for missing key, got %s", raw)
	}
}

func TestFileStoreRemoveAndClear(t *testing.T) {
	s := NewFileStore(t.TempDir())
	ctx := context.Background()

	for _, k := range []string{"a", "b", "c"} {
		if err := s.Set(ctx, k, k); err != nil {
			t.Fatalf("Set %s: %v", k, err)
		}
	}
	if err := s.Remove(ctx, "b"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	raw, err := s.Get(ctx, "b")
	if err != nil || raw != nil {
		t.Fatalf("expected b absent after Remove, got raw=%s err=%v", raw, err)
	}
	// Removing a missing key is a no-op.
	if err := s.Remove(ctx, "ghost"); err != nil {
		t.Fatalf("Remove(ghost): %v", err)
	}

	if err := s.Clear(ctx); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	raw, err = s.Get(ctx, "a")
	if err != nil || raw != nil {
		t.Fatalf("expected a absent after Clear, got raw=%s err=%v", raw, err)
	}
}

func TestFileStoreKeysWithSlash(t *testing.T) {
	// Keys with '/' must not escape the store directory.
	s := NewFileStore(t.TempDir())
	ctx := context.Background()
	if err := s.Set(ctx, "lounge/screens/abc", "x"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	raw, err := s.Get(ctx, "lounge/screens/abc")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(raw) != `"x"` {
		t.Fatalf("unexpected payload %s", raw)
	}
}
