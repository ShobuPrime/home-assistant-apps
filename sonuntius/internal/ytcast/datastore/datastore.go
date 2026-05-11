// Maps to: src/lib/utils/DataStore.ts
//
// Package datastore defines the abstract DataStore interface upstream uses to
// persist receiver state (lounge tokens, screen IDs, paired devices, etc.)
// plus the JSON-file-backed default in default.go. Callers can swap in their
// own implementation by satisfying the interface.
package datastore

import (
	"context"
	"encoding/json"

	"github.com/shobuprime/sonuntius/internal/ytcast/logger"
)

// DataStore ports the upstream `abstract class DataStore` in DataStore.ts.
//
// Upstream's TypeScript signature is `set<T>(key, value)` and `get<T>(key)`;
// Go has no method-level generics (and the upstream impl serializes via JSON
// internally), so we standardize on raw JSON at the interface boundary:
//
//   - Set accepts any `value` and the implementation is responsible for JSON
//     serialization.
//   - Get returns json.RawMessage; callers unmarshal into the type they want.
//   - Missing keys return (nil, nil) — matches upstream's `... || null`.
//
// Logger is exposed so implementations can emit warnings/errors via the
// receiver's configured logger, mirroring the `logger` getter upstream.
type DataStore interface {
	// SetLogger attaches a logger. Mirrors `setLogger(logger)` upstream.
	SetLogger(l logger.Logger)
	// Logger returns the previously attached logger (may be nil before SetLogger).
	Logger() logger.Logger
	// Set persists `value` under `key`. Implementations should JSON-encode.
	Set(ctx context.Context, key string, value any) error
	// Get fetches the raw JSON previously stored under `key`. Returns
	// `(nil, nil)` if the key is absent.
	Get(ctx context.Context, key string) (json.RawMessage, error)
	// Remove deletes the entry under `key`. Removing an absent key is a
	// no-op (no error). The upstream `node-persist` library has the same
	// shape — added here so the lounge layer can clear stale tokens.
	Remove(ctx context.Context, key string) error
	// Clear removes every key in the store. Mirrors `clear()` on
	// `DefaultDataStore`.
	Clear(ctx context.Context) error
}
