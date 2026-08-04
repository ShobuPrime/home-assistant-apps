// Maps to: N/A — Go-only Phase 6b persistence helper for cast-receiver.
//
// Phase 6 polish (plan §6) calls for persisting "AirReceiver cert
// fingerprint" across restarts so we can log when the user replaces it.
// We compute a SHA-256 over the raw cert file bytes and store the
// hex-encoded digest at <data_dir>/airreceiver_cert.fingerprint. On
// next boot we compare and log if it changed — useful for catching
// silent cert rotations that might break senders.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const fingerprintFilename = "airreceiver_cert.fingerprint"

// trackCertFingerprint computes the cert's SHA-256, compares with the
// last-persisted digest under dataDir, logs the change (if any), and
// writes the new digest. Failures are non-fatal — the binary continues.
func trackCertFingerprint(certPath, dataDir string, log *slog.Logger) {
	if certPath == "" || dataDir == "" {
		return
	}
	certBytes, err := os.ReadFile(certPath)
	if err != nil {
		log.Debug("persist: cert fingerprint skipped (cert unreadable)", "err", err)
		return
	}
	sum := sha256.Sum256(certBytes)
	digest := hex.EncodeToString(sum[:])

	fpPath := filepath.Join(dataDir, fingerprintFilename)
	prev, err := os.ReadFile(fpPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Debug("persist: previous fingerprint unreadable", "err", err, "path", fpPath)
	}
	prevDigest := strings.TrimSpace(string(prev))

	switch {
	case prevDigest == "":
		log.Info("persist: AirReceiver cert fingerprint recorded",
			"sha256", digest[:16]+"…", "path", fpPath)
	case prevDigest != digest:
		log.Warn("persist: AirReceiver cert CHANGED since last boot",
			"old_sha256", prevDigest[:16]+"…",
			"new_sha256", digest[:16]+"…",
			"path", fpPath)
	default:
		log.Debug("persist: AirReceiver cert fingerprint unchanged", "sha256", digest[:16]+"…")
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Debug("persist: data dir mkdir failed", "err", err, "dir", dataDir)
		return
	}
	tmp := fpPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(digest+"\n"), 0o644); err != nil {
		log.Debug("persist: fingerprint write failed", "err", err, "path", tmp)
		return
	}
	if err := os.Rename(tmp, fpPath); err != nil {
		log.Debug("persist: fingerprint rename failed", "err", err, "path", fpPath)
	}
}
