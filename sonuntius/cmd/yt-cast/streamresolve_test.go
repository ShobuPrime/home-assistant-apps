// Maps to: N/A — Go-only tests for the yt-dlp stream URL resolver.
package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestStreamResolver_EmptyVideoID(t *testing.T) {
	t.Parallel()
	r := newStreamResolver()
	if _, err := r.Resolve(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty video id")
	}
}

func TestStreamResolver_MissingBinary(t *testing.T) {
	t.Parallel()
	r := &streamResolver{Binary: "this-binary-does-not-exist-anywhere", Timeout: time.Second}
	_, err := r.Resolve(context.Background(), "bp4_7T9J6Fg")
	if err == nil {
		t.Fatal("expected error when binary is missing")
	}
	if !strings.Contains(err.Error(), "yt-dlp failed") {
		t.Errorf("error did not surface yt-dlp failure: %v", err)
	}
}

func TestStreamResolver_TimeoutPropagated(t *testing.T) {
	t.Parallel()
	// Use a binary that exists but will run longer than the timeout.
	// `sleep` is universally available on Alpine + Linux test runners.
	r := &streamResolver{Binary: "sleep", Timeout: 50 * time.Millisecond}
	start := time.Now()
	_, err := r.Resolve(context.Background(), "bp4_7T9J6Fg")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed > 2*time.Second {
		t.Errorf("resolver took %v; timeout should have killed it well before 2s", elapsed)
	}
}
