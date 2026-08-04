// Maps to: src/lib/app/PlaylistRequestHandler.ts
//
// PlaylistRequestHandler is the abstract surface a Playlist consults when
// it needs to discover the previous / next videos around the current
// track. Upstream defines it as an abstract TypeScript class with a
// `getPreviousNextVideos` hook plus a wrapper `getPreviousNextVideosAbortable`
// that bakes in abort-signal handling. The Go port keeps the same shape
// using a context.Context for cancellation.
//
// Implementations override GetPreviousNextVideos (and optionally Reset);
// the Logger getter is satisfied by embedding *BaseHandler. The
// DefaultPlaylistRequestHandler in playlistreq_default.go is a minimal
// stub — see that file for justification.
package lounge

import (
	"context"
	"errors"

	"github.com/shobuprime/sonuntius/internal/ytcast/logger"
	"github.com/shobuprime/sonuntius/internal/ytcast/types"
)

// PlaylistPreviousNextVideos ports the upstream interface returned by
// `getPreviousNextVideos`. Either field may be nil meaning "no neighbor".
type PlaylistPreviousNextVideos struct {
	Previous *types.Video `json:"previous,omitempty"`
	Next     *types.Video `json:"next,omitempty"`
}

// PlaylistRequestHandler ports the abstract upstream class. Implementors
// embed *BaseHandler to inherit the SetLogger / Logger / Reset defaults.
type PlaylistRequestHandler interface {
	// SetLogger attaches a logger. Mirrors `setLogger(logger)` upstream.
	SetLogger(l logger.Logger)
	// Logger returns the previously attached logger.
	Logger() logger.Logger
	// GetPreviousNextVideos ports the abstract upstream method. The ctx
	// argument carries the abort signal from `getPreviousNextVideosAbortable`.
	GetPreviousNextVideos(ctx context.Context, target types.Video, playlist *Playlist) (PlaylistPreviousNextVideos, error)
	// Reset ports `reset()` — implementations override to clear caches.
	Reset()
}

// GetPreviousNextVideosAbortable ports the upstream wrapper that injects
// an abort check around the implementation call. The check before the
// call mirrors upstream verbatim; the check after the call is preserved
// so a late-arriving cancel still produces an AbortError instead of a
// stale result.
//
// Callers pass a context that gets cancelled when they want to abort —
// the function returns ctx.Err() wrapped in an "AbortError"-named error
// matching upstream's `error.name = 'AbortError'` convention.
func GetPreviousNextVideosAbortable(
	ctx context.Context,
	handler PlaylistRequestHandler,
	target types.Video,
	playlist *Playlist,
) (PlaylistPreviousNextVideos, error) {
	if err := checkAbort(ctx, target.ID, handler.Logger()); err != nil {
		return PlaylistPreviousNextVideos{}, err
	}
	result, err := handler.GetPreviousNextVideos(ctx, target, playlist)
	if err != nil {
		return PlaylistPreviousNextVideos{}, err
	}
	if err := checkAbort(ctx, target.ID, handler.Logger()); err != nil {
		return PlaylistPreviousNextVideos{}, err
	}
	return result, nil
}

// abortError is a sentinel type for ctx.Cancelled-derived errors. It
// reports true to errors.Is on either context.Canceled or the Go-only
// ErrAbort sentinel.
type abortError struct {
	msg string
}

func (e *abortError) Error() string { return e.msg }

// ErrPlaylistRequestAborted is the abort sentinel for callers that want
// to detect an abort without reaching for the typed error.
var ErrPlaylistRequestAborted = errors.New("AbortError: PlaylistRequestHandler operation aborted")

func (e *abortError) Is(target error) bool {
	return target == ErrPlaylistRequestAborted || target == context.Canceled
}

func checkAbort(ctx context.Context, videoID string, l logger.Logger) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		msg := "PlaylistRequestHandler.getPreviousNextVideos() aborted for video Id: " + videoID
		if l != nil {
			l.Debug("[yt-cast-receiver] " + msg + ".")
		}
		return &abortError{msg: msg}
	}
	return nil
}

// BaseHandler is an embeddable base implementing the boilerplate
// SetLogger/Logger/Reset surface. Custom handlers can embed this and
// override only GetPreviousNextVideos.
type BaseHandler struct {
	log logger.Logger
}

// SetLogger implements PlaylistRequestHandler.
func (h *BaseHandler) SetLogger(l logger.Logger) {
	h.log = l
}

// Logger implements PlaylistRequestHandler.
func (h *BaseHandler) Logger() logger.Logger {
	return h.log
}

// Reset is a no-op by default — upstream comment says "Do nothing".
func (h *BaseHandler) Reset() {}
