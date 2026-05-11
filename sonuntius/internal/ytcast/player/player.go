// Maps to: src/lib/Player.ts
//
// Package player exposes the abstract Player surface implementors must
// satisfy. Upstream's `Player` is a TypeScript abstract class that mixes the
// `do*` hooks implementors override with the state-machine and EventEmitter
// scaffolding. We split that here:
//
//   - Player (this file) is a Go interface containing only the `do*` hooks.
//     This is the contract host applications implement to plug a real
//     playback engine into the receiver.
//   - StateMachine wiring (queue, status tracking, event fan-out) lives with
//     the Phase 3 orchestrator that ports `YouTubeApp` — keeping it out of
//     the interface lets implementors stay focused on playback primitives.
//   - Event types live alongside in events.go so the Phase 3 orchestrator
//     can broadcast state changes via a Go-idiomatic channel rather than the
//     Node EventEmitter.
package player

import (
	"context"

	"github.com/shobuprime/sonuntius/internal/ytcast/types"
)

// Volume ports the upstream `interface Volume`.
//
// Level is a 0-100 integer (the upstream wire range); the Phase 3
// state machine clamps to that range on read, matching the upstream
// `getVolume()` post-processing.
type Volume struct {
	Level int  `json:"level"`
	Muted bool `json:"muted"`
}

// Player is the abstract surface upstream `Player.ts` defines via `protected
// abstract` methods. Hosts implement this interface; the Phase 3 orchestrator
// owns the public play / pause / seek / queue management methods that wrap
// these.
//
// Each method takes a context.Context so cancellation flows through to the
// playback engine. Methods return `error` only — upstream uses
// `Promise<boolean>` where `false` means "could not perform the operation";
// in Go we treat that as a non-nil error so callers can attach context.
type Player interface {
	// DoPlay starts playback of `video` from `position` seconds.
	DoPlay(ctx context.Context, video types.Video, position float64) error

	// DoPause pauses current playback.
	DoPause(ctx context.Context) error

	// DoResume resumes paused playback.
	DoResume(ctx context.Context) error

	// DoStop stops current playback or cancels any pending playback.
	DoStop(ctx context.Context) error

	// DoSeek seeks to `position` seconds.
	DoSeek(ctx context.Context, position float64) error

	// DoSetVolume sets the player volume to `volume`.
	DoSetVolume(ctx context.Context, volume Volume) error

	// DoGetVolume returns the current volume level and muted state.
	DoGetVolume(ctx context.Context) (Volume, error)

	// DoGetPosition returns the current playback position in seconds.
	DoGetPosition(ctx context.Context) (float64, error)

	// DoGetDuration returns the current video's duration in seconds.
	DoGetDuration(ctx context.Context) (float64, error)
}
