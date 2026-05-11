// Maps to: N/A — Go-only castv2.Intent → events.PlayIntent /
//          TransportCommand / VolumeCommand mapper.
//
// The CASTV2 Server hands us a castv2.Intent whenever a parser claims a
// LOAD payload. This file translates that intent onto the existing IPC
// wire types in internal/events so the ma-bridge dispatcher can reuse
// the same translation table it already runs for yt-cast.
//
// Mapping table (intent → events.PlayIntent):
//
//   Provider="tidal", TrackID:X  → PlayIntent{Provider:"tidal", TrackID:X, Source:"cast-receiver"}
//   Provider="url",   URL:U      → PlayIntent{Provider:"url",   URL:U,    Source:"cast-receiver"}
//   Provider=X (other),...       → PlayIntent{Provider:X, TrackID, URL, Metadata copied through}
//
// Transport / volume command translation is *not* wired in this phase
// because Phase 3a's media handler does not yet surface those callbacks
// to the cmd binary. When a future phase exposes them (probably as a
// distinct Server method), this file will grow a counterpart helper.
package main

import (
	"context"
	"log/slog"
	"sync"

	"github.com/shobuprime/sonuntius/internal/castv2"
	"github.com/shobuprime/sonuntius/internal/events"
	"github.com/shobuprime/sonuntius/internal/ipc"
)

// intentSource is the label every event the cast-receiver emits carries
// in its `source` field. The dispatcher uses it to log routing decisions.
const intentSource = "cast-receiver"

// intentEmitter owns the IPC client used to push translated intents
// onto the ma-bridge bus. The CASTV2 Server invokes Emit from per-
// connection goroutines, so the writer is mutex-protected.
type intentEmitter struct {
	mu     sync.Mutex
	client *ipc.Client
	log    *slog.Logger
}

// newIntentEmitter constructs an emitter. The caller wires the client
// in via setIPCClient once the connector establishes a connection; Emit
// logs and drops intents until then.
func newIntentEmitter(log *slog.Logger) *intentEmitter {
	if log == nil {
		log = slog.Default()
	}
	return &intentEmitter{log: log}
}

// setIPCClient swaps the underlying writer. nil means "broker is
// offline" — Emit logs and drops in that case.
func (e *intentEmitter) setIPCClient(c *ipc.Client) {
	e.mu.Lock()
	e.client = c
	e.mu.Unlock()
}

// Emit translates a castv2.Intent into a PlayIntent and pushes it onto
// the IPC bus. The CASTV2 server calls this from a parser-claim
// callback; we hold the mutex for the duration of the send so concurrent
// emitters can't interleave bytes.
func (e *intentEmitter) Emit(_ context.Context, source string, intent castv2.Intent) {
	pi := translate(intent)
	if pi == nil {
		e.log.Debug("cast-receiver: dropping empty intent", "cast_source", source)
		return
	}
	e.mu.Lock()
	cli := e.client
	e.mu.Unlock()
	if cli == nil {
		e.log.Warn("cast-receiver: ma-bridge offline — dropping intent",
			"provider", pi.Provider, "track_id", pi.TrackID, "url", pi.URL)
		return
	}
	if err := cli.Send(pi); err != nil {
		e.log.Warn("cast-receiver: failed to send PlayIntent",
			"err", err, "provider", pi.Provider)
		return
	}
	e.log.Info("cast-receiver: PlayIntent emitted",
		"provider", pi.Provider, "track_id", pi.TrackID, "url", pi.URL,
		"cast_source", source)
}

// translate converts a castv2.Intent to a PlayIntent. Returns nil when
// the intent carries no actionable identifier (defensive — the CASTV2
// server never invokes the callback in that case, but keeping the guard
// makes the helper safe to reuse from tests and future callers).
func translate(intent castv2.Intent) *events.PlayIntent {
	provider := intent.Provider
	trackID := intent.TrackID
	url := intent.URL
	if provider == "" && trackID == "" && url == "" {
		return nil
	}
	return &events.PlayIntent{
		Provider: provider,
		TrackID:  trackID,
		URL:      url,
		Source:   intentSource,
		Metadata: intent.Metadata,
	}
}
