// Maps to: N/A — Go-only options loader for the sonuntius cast-receiver binary.
//
// Mirrors cmd/yt-cast/options.go for the CASTV2 receiver: read
// /data/options.json via internal/config, derive a stable receiver UUID,
// and resolve the cert / signature / intermediates paths Phase 3a's
// AirReceiverResponder consumes. None of this code crashes the process
// on a missing file — the responder degrades to an empty AuthResponse
// and the cmd binary suppresses the TLS listener accordingly.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/shobuprime/sonuntius/internal/config"
)

// runtimeOptions is the resolved configuration the cast-receiver service
// runs with. Mirrors the shape of the yt-cast equivalent so future
// reviewers can diff the two side-by-side.
type runtimeOptions struct {
	// AddonOptions is the parsed /data/options.json (or its supervisor
	// fallback). Always populated.
	AddonOptions config.Options
	// LogLevel is the slog level derived from AddonOptions.LogLevel.
	LogLevel slog.Level
	// FriendlyName is the receiver's display name surfaced over mDNS as
	// the Cast picker label.
	FriendlyName string
	// DataDir is the persistent state directory.
	DataDir string
	// UUIDFile is the path of the stable receiver UUID.
	UUIDFile string
	// UUID is the stable receiver UUID (UUIDv4 hex with separators).
	UUID string

	// Cert artifact paths. CertPath / KeyPath come from addon options;
	// SignaturePath / IntermediatesPath are derived siblings.
	CertPath          string
	KeyPath           string
	SignaturePath     string
	IntermediatesPath string
}

// dataDir returns the configured data directory ($SONUNTIUS_DATA_DIR
// when set, otherwise /data/sonuntius). Mirrors cmd/yt-cast/options.go
// so the two services share a state directory.
func dataDir() string {
	if v := os.Getenv("SONUNTIUS_DATA_DIR"); v != "" {
		return v
	}
	return "/data/sonuntius"
}

// loadRuntimeOptions builds the runtimeOptions struct used by main. The
// function never fails — every error path either falls back to a safe
// default or logs a warning so the service stays alive under S6.
func loadRuntimeOptions(ctx context.Context, log *slog.Logger) runtimeOptions {
	opts, err := config.Load(ctx, log)
	if err != nil {
		log.Warn("cast-receiver: config load returned error — using defaults", "err", err)
	}

	r := runtimeOptions{
		AddonOptions: opts,
		LogLevel:     config.ResolveLogLevel(opts.LogLevel),
		FriendlyName: opts.FriendlyNameTidal,
		DataDir:      dataDir(),
	}
	if strings.TrimSpace(r.FriendlyName) == "" {
		r.FriendlyName = "Sonuntius (Tidal)"
	}

	r.CertPath = strings.TrimSpace(opts.CastCertPath)
	if r.CertPath == "" {
		r.CertPath = "/share/sonuntius/airreceiver_cert.pem"
	}
	r.KeyPath = strings.TrimSpace(opts.CastKeyPath)
	if r.KeyPath == "" {
		r.KeyPath = "/share/sonuntius/airreceiver_key.pem"
	}
	r.SignaturePath = deriveCompanionPath(r.CertPath, ".signature")
	r.IntermediatesPath = deriveCompanionPath(r.CertPath, ".intermediates.pem")

	r.UUIDFile = filepath.Join(r.DataDir, "cast-receiver-uuid")
	uuid, err := loadOrCreateUUID(r.UUIDFile)
	if err != nil {
		log.Warn("cast-receiver: failed to persist receiver UUID — using ephemeral value", "err", err)
		uuid = generateUUIDv4()
	}
	r.UUID = uuid
	return r
}

// deriveCompanionPath builds a sibling path for the cert (e.g. cert.pem
// → cert.pem.signature). We append rather than replace the extension so
// "airreceiver_cert.pem.signature" is the natural pairing.
func deriveCompanionPath(certPath, suffix string) string {
	if strings.TrimSpace(certPath) == "" {
		return ""
	}
	return certPath + suffix
}

// loadOrCreateUUID reads the UUID from path, returning an existing
// value if present and well-formed. Otherwise a fresh UUIDv4 is
// generated and persisted.
func loadOrCreateUUID(path string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("cast-receiver: ensure data dir: %w", err)
	}
	if data, err := os.ReadFile(path); err == nil {
		uuid := strings.TrimSpace(string(data))
		if isValidUUID(uuid) {
			return uuid, nil
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("cast-receiver: read uuid file: %w", err)
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

// isValidUUID accepts a stored UUID if it's 36 chars (with dashes) or
// 32 chars (without).
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
		return fmt.Errorf("cast-receiver: create %s: %w", tmp, err)
	}
	if _, err := io.WriteString(f, content); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("cast-receiver: write %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("cast-receiver: close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("cast-receiver: rename %s -> %s: %w", tmp, path, err)
	}
	return nil
}
