// Maps to: src/lib/app/DefaultPlaylistRequestHandler.ts (stubbed)
//
// Upstream's DefaultPlaylistRequestHandler uses `youtubei.js` to talk to
// Innertube and fetch full metadata (title, duration, thumbnails) plus
// autoplay neighbor videos. Importing youtubei.js (or any Innertube
// client) into the Go port would violate the stdlib-only constraint and
// pull in an enormous Node-flavoured surface area, and sonuntius does
// not need rich metadata — it needs just the video id to forward to
// Music Assistant.
//
// This stub implements the PlaylistRequestHandler contract minimally:
//
//   - GetPreviousNextVideos returns a zero PlaylistPreviousNextVideos
//     (no previous, no next). The Playlist treats that as "the current
//     video is the last one in the queue with no autoplay candidate",
//     which is the safe default until a real implementation is wired up.
//   - Reset is a no-op.
//
// Hosts that want richer behavior (e.g. autoplay) supply their own
// PlaylistRequestHandler — the interface itself preserves the upstream
// API exactly.
package lounge

import (
	"context"

	"github.com/shobuprime/sonuntius/internal/ytcast/logger"
	"github.com/shobuprime/sonuntius/internal/ytcast/types"
)

// SenderProvider mirrors the upstream `getConnectedSendersFn` constructor
// hook. The default handler doesn't actually consult it (it has no
// Innertube to inject the connected-devices list into) but we accept the
// hook so the interface stays compatible with upstream and any future
// stdlib-based implementation can use it.
type SenderProvider func() []*types.Sender

// DefaultPlaylistRequestHandler is the stdlib-friendly stub. Initialise
// with NewDefaultPlaylistRequestHandler so future enrichment can hook
// into the constructor.
type DefaultPlaylistRequestHandler struct {
	BaseHandler
	getConnectedSenders SenderProvider
}

// NewDefaultPlaylistRequestHandler matches upstream's
// `new DefaultPlaylistRequestHandler({getConnectedSendersFn})`.
func NewDefaultPlaylistRequestHandler(getConnectedSenders SenderProvider) *DefaultPlaylistRequestHandler {
	return &DefaultPlaylistRequestHandler{
		getConnectedSenders: getConnectedSenders,
	}
}

// GetPreviousNextVideos returns no neighbors and logs a debug-level
// breadcrumb so it's obvious the stub is active.
//
// TODO(yt-cast Phase 2.1): replace stub with stdlib HTTP scrape or
// expose Player-provided metadata hook so autoplay can work without
// pulling youtubei.js in.
func (h *DefaultPlaylistRequestHandler) GetPreviousNextVideos(
	_ context.Context,
	target types.Video,
	_ *Playlist,
) (PlaylistPreviousNextVideos, error) {
	if l := h.Logger(); l != nil {
		l.Debug("[yt-cast-receiver] DefaultPlaylistRequestHandler stub: no neighbors for video", target.ID)
	}
	return PlaylistPreviousNextVideos{}, nil
}

// Reset clears the (currently-empty) cache. No-op for the stub.
func (h *DefaultPlaylistRequestHandler) Reset() {
	// no-op
}

// connectedSenders exposes the injected provider, mainly for tests and
// future implementations. Currently unused inside the stub.
//
//nolint:unused // referenced by future stdlib implementation; keep for
// API parity with upstream's getConnectedSendersFn hook.
func (h *DefaultPlaylistRequestHandler) connectedSenders() []*types.Sender {
	if h.getConnectedSenders == nil {
		return nil
	}
	return h.getConnectedSenders()
}

// Compile-time assertion that the stub satisfies the interface.
var _ PlaylistRequestHandler = (*DefaultPlaylistRequestHandler)(nil)

// SetLogger overrides the embedded BaseHandler so callers can SetLogger
// before any logger init (mostly to avoid logger nil-checks; the
// upstream getter type is non-optional).
func (h *DefaultPlaylistRequestHandler) SetLogger(l logger.Logger) {
	h.BaseHandler.SetLogger(l)
}
