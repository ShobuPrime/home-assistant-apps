// Maps to: N/A — Go-only option helpers for the sonuntius yt-cast binary.
//
// The sonuntius yt-cast service consumes the same /data/options.json
// addon config that ma-bridge does (loaded via internal/config) and a
// small per-service slice of derived runtime config:
//
//   - the friendly DIAL name (from `friendly_name_youtube`)
//   - a stable receiver UUID (persisted under /data/sonuntius/)
//   - the addon data directory used by the lounge data store
//
// This file collects the loaders that wrap internal/config and produce
// those derived values.
package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"crypto/rand"

	"github.com/shobuprime/sonuntius/internal/config"
	"github.com/shobuprime/sonuntius/internal/ytcast/constants"
)

// runtimeOptions is the resolved configuration the yt-cast service
// runs with.
type runtimeOptions struct {
	// AddonOptions is the parsed /data/options.json (or its supervisor
	// fallback). Always populated, possibly with zero-valued fields if
	// the file is missing.
	AddonOptions config.Options
	// LogLevel is the slog level derived from AddonOptions.LogLevel.
	LogLevel slog.Level
	// FriendlyName is the DIAL friendly name. Defaults to "Sonuntius"
	// when AddonOptions.FriendlyNameYouTube is empty.
	FriendlyName string
	// DataDir is the persistent state directory the lounge data store
	// writes into.
	DataDir string
	// UUIDFile is the path of the stable receiver UUID.
	UUIDFile string
	// UUID is the stable receiver UUID (UUIDv4 hex with separators).
	UUID string
}

// dataDir returns the configured data directory ($SONUNTIUS_DATA_DIR
// when set, otherwise /data/sonuntius).
func dataDir() string {
	if v := os.Getenv("SONUNTIUS_DATA_DIR"); v != "" {
		return v
	}
	return "/data/sonuntius"
}

// loadRuntimeOptions builds the runtimeOptions struct used by main.
//
// The function never fails — every error path either falls back to a
// safe default or logs a warning. We absorb errors here because the
// service is supposed to stay alive even when the addon is misconfigured.
func loadRuntimeOptions(ctx context.Context, log *slog.Logger) runtimeOptions {
	opts, err := config.Load(ctx, log)
	if err != nil {
		log.Warn("yt-cast: config load returned error — using defaults", "err", err)
	}

	r := runtimeOptions{
		AddonOptions: opts,
		LogLevel:     config.ResolveLogLevel(opts.LogLevel),
		FriendlyName: opts.FriendlyNameYouTube,
		DataDir:      dataDir(),
	}
	if strings.TrimSpace(r.FriendlyName) == "" {
		r.FriendlyName = "Sonuntius"
	}
	r.UUIDFile = filepath.Join(r.DataDir, "yt-cast-uuid")
	uuid, err := loadOrCreateUUID(r.UUIDFile)
	if err != nil {
		log.Warn("yt-cast: failed to persist receiver UUID — using ephemeral value", "err", err)
		uuid = generateUUIDv4()
	}
	r.UUID = uuid
	return r
}

// loadOrCreateUUID reads the UUID from path, returning an existing
// value if present and well-formed. Otherwise a fresh UUIDv4 is
// generated and persisted.
func loadOrCreateUUID(path string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("yt-cast: ensure data dir: %w", err)
	}
	if data, err := os.ReadFile(path); err == nil {
		uuid := strings.TrimSpace(string(data))
		if isValidUUID(uuid) {
			return uuid, nil
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("yt-cast: read uuid file: %w", err)
	}
	uuid := generateUUIDv4()
	if err := writeAtomic(path, uuid); err != nil {
		return uuid, err
	}
	return uuid, nil
}

// generateUUIDv4 returns a v4 UUID using crypto/rand. We assemble the
// hex form manually so we don't need a third-party UUID package.
func generateUUIDv4() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// isValidUUID is a lenient sanity check — a stored UUID is acceptable
// if it's 36 chars (with dashes) or 32 chars (without).
func isValidUUID(s string) bool {
	switch len(s) {
	case 32, 36:
		return true
	}
	return false
}

// writeAtomic writes content to path via a tmpfile + rename so a power
// loss can't leave a half-written UUID file behind.
func writeAtomic(path, content string) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("yt-cast: create %s: %w", tmp, err)
	}
	if _, err := io.WriteString(f, content); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("yt-cast: write %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("yt-cast: close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("yt-cast: rename %s -> %s: %w", tmp, path, err)
	}
	return nil
}

// upstreamShort returns the 7-char short SHA of the pinned upstream
// commit — the smoke test greps for this to verify the commit pin is
// alive.
func upstreamShort() string {
	c := constants.UpstreamCommit
	if len(c) >= 7 {
		return c[:7]
	}
	return c
}
