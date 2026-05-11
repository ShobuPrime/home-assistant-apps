// Maps to: N/A — Go-only sonuntius Player adapter.
//
// The adapter satisfies internal/ytcast/player.Player (the do-only host
// interface). Its job is to translate every Do* call into a sonuntius
// IPC event the existing ma-bridge can dispatch to Music Assistant,
// and to surface the latest PlayerState frames received over IPC back
// to the receiver as cached volume / position / duration values.
//
// Provider mapping (DoPlay):
//
//   - Video.Client.Theme == "m"  → "ytmusic" (YouTube Music app)
//   - Video.Client.Theme == "cl" → "youtube" (regular YouTube app)
//
// The dispatcher in internal/dispatcher rejects unknown providers — for
// the YouTube classic surface there is no MA-native provider, so we
// still emit a PlayIntent (with provider="youtube") to keep the
// integration observable; the dispatcher logs and drops it. Hooking up
// a real YouTube provider in MA is a Phase 2.1 follow-up.
package main

import (
	"context"
	"errors"
	"sync"

	"github.com/shobuprime/sonuntius/internal/events"
	"github.com/shobuprime/sonuntius/internal/ipc"
	pkgplayer "github.com/shobuprime/sonuntius/internal/ytcast/player"
	"github.com/shobuprime/sonuntius/internal/ytcast/types"
)

// adapter is the live Player implementation. It is safe for concurrent
// use; mu guards the cached state + the IPC writer.
type adapter struct {
	mu sync.Mutex

	// ipcClient is the persistent connection to the ma-bridge broker.
	// Reset (set to nil + replaced) whenever the broker goes away.
	ipcClient *ipc.Client

	// cachedState holds the latest PlayerState frame we received from
	// the IPC broker. DoGetVolume / DoGetPosition / DoGetDuration read
	// from here so the receiver can answer sender queries without
	// blocking on a round-trip to MA.
	cachedState events.PlayerState

	// source is the "originating receiver" label attached to every
	// outgoing event. Always "yt-cast" for this adapter.
	source string
}

// newAdapter constructs an adapter that emits events with source =
// "yt-cast". Pass a connected ipc.Client; the adapter doesn't dial
// itself because the wrapper main keeps connection lifecycle.
func newAdapter(client *ipc.Client) *adapter {
	return &adapter{
		ipcClient: client,
		source:    "yt-cast",
	}
}

// setIPCClient swaps the underlying writer. nil means "broker is
// offline" — Do* methods return an error in that case so the receiver
// can surface a sensible failure to senders.
func (a *adapter) setIPCClient(c *ipc.Client) {
	a.mu.Lock()
	a.ipcClient = c
	a.mu.Unlock()
}

// updateCachedState replaces the cached PlayerState frame.
func (a *adapter) updateCachedState(ps events.PlayerState) {
	a.mu.Lock()
	a.cachedState = ps
	a.mu.Unlock()
}

// snapshotState returns a copy of the cached state.
func (a *adapter) snapshotState() events.PlayerState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cachedState
}

// send is the single send path. Holds the mutex for the duration of
// the write so concurrent emitters can't interleave bytes.
func (a *adapter) send(ev events.Event) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ipcClient == nil {
		return errIPCOffline
	}
	return a.ipcClient.Send(ev)
}

// errIPCOffline is the sentinel returned when no IPC client is
// available. Callers (the receiver) treat it as a transient failure;
// no automatic retry — the wrapper main reconnects on its own.
var errIPCOffline = errors.New("yt-cast: ma-bridge IPC unavailable")

// providerForClient maps a Cast Client surface onto the dispatcher's
// provider tag. See package doc comment for rationale.
func providerForClient(c types.Client) string {
	switch c.Theme {
	case "m":
		return "ytmusic"
	case "cl":
		return "youtube"
	default:
		// Unknown surface — let the dispatcher's "unknown provider" log
		// surface the issue rather than silently dropping.
		return c.Theme
	}
}

// DoPlay emits a PlayIntent.
func (a *adapter) DoPlay(_ context.Context, video types.Video, _ float64) error {
	return a.send(&events.PlayIntent{
		Provider: providerForClient(video.Client),
		TrackID:  video.ID,
		Source:   a.source,
	})
}

// DoPause emits a pause TransportCommand.
func (a *adapter) DoPause(_ context.Context) error {
	return a.send(&events.TransportCommand{Command: "pause", Source: a.source})
}

// DoResume emits a play TransportCommand.
func (a *adapter) DoResume(_ context.Context) error {
	return a.send(&events.TransportCommand{Command: "play", Source: a.source})
}

// DoStop emits a stop TransportCommand.
func (a *adapter) DoStop(_ context.Context) error {
	return a.send(&events.TransportCommand{Command: "stop", Source: a.source})
}

// DoSeek emits a seek TransportCommand with the requested position.
func (a *adapter) DoSeek(_ context.Context, position float64) error {
	p := position
	return a.send(&events.TransportCommand{
		Command:  "seek",
		Position: &p,
		Source:   a.source,
	})
}

// DoSetVolume emits a VolumeCommand. The upstream wire range for
// `level` is 0-100; the dispatcher expects 0.0-1.0, so we rescale.
func (a *adapter) DoSetVolume(_ context.Context, volume pkgplayer.Volume) error {
	level := float64(volume.Level) / 100.0
	muted := volume.Muted
	return a.send(&events.VolumeCommand{
		Level:  &level,
		Muted:  &muted,
		Source: a.source,
	})
}

// DoGetVolume returns the cached volume + muted state, rescaled from
// the dispatcher's 0.0-1.0 wire range back to the receiver's 0-100.
func (a *adapter) DoGetVolume(_ context.Context) (pkgplayer.Volume, error) {
	st := a.snapshotState()
	out := pkgplayer.Volume{}
	if st.Volume != nil {
		out.Level = int(*st.Volume * 100)
	}
	if st.Muted != nil {
		out.Muted = *st.Muted
	}
	return out, nil
}

// DoGetPosition returns the cached position in seconds.
func (a *adapter) DoGetPosition(_ context.Context) (float64, error) {
	st := a.snapshotState()
	if st.Position != nil {
		return *st.Position, nil
	}
	return 0, nil
}

// DoGetDuration returns the cached duration in seconds.
func (a *adapter) DoGetDuration(_ context.Context) (float64, error) {
	st := a.snapshotState()
	if st.Duration != nil {
		return *st.Duration, nil
	}
	return 0, nil
}

// Compile-time assertion that adapter satisfies the Player contract.
var _ pkgplayer.Player = (*adapter)(nil)
