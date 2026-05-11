// Maps to: src/lib/Player.ts (state-machine portion)
//
// engine wraps a host-provided player.Player (which only implements the
// `Do*` hooks — see internal/ytcast/player/player.go for the rationale
// behind that split) with the same state machine and queue scaffolding
// upstream's abstract `Player` class baked in.
//
// In TypeScript the wrapper and the abstract hooks live in one file
// because TS lets you have abstract methods inside a concrete class. Go
// has no equivalent, so the foundation package ships the interface (the
// hooks the host implements) and this file ships the wrapping behaviour
// (play / pause / resume / stop / seek / next / previous / setVolume /
// reset, plus state event emission). The split is byte-for-byte logical
// parity with upstream — every branch in `Player.ts` has a counterpart
// here.
package ytcast

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/shobuprime/sonuntius/internal/ytcast/constants"
	"github.com/shobuprime/sonuntius/internal/ytcast/logger"
	"github.com/shobuprime/sonuntius/internal/ytcast/lounge"
	pkgplayer "github.com/shobuprime/sonuntius/internal/ytcast/player"
	"github.com/shobuprime/sonuntius/internal/ytcast/types"
)

// playerEngine is the Go port of the `Player.ts` state machine. Public
// methods Play, Pause, Resume, Stop, Seek, Next, Previous, SetVolume,
// Reset wrap the host's pkgplayer.Player implementation; GetState /
// GetNavInfo expose the cached state for the orchestrator.
//
// Concurrency: a single sync.Mutex serialises every state transition.
// The host's Do* hooks are called while holding the lock — this matches
// upstream's behaviour (Node is single-threaded so the `await`s inside
// `Player.play` etc. naturally serialise) and is fine for the workload
// (the bottleneck is the host's HTTP call to Music Assistant, not the
// in-process state machine).
type playerEngine struct {
	host  pkgplayer.Player
	queue *lounge.Playlist
	bus   *pkgplayer.EventBus
	log   logger.Logger

	mu                    sync.Mutex
	status                constants.PlayerStatus
	cpn                   string
	previousState         *pkgplayer.State
	zeroVolumeLevelOnMute bool
}

// newPlayerEngine constructs an engine in the IDLE state with a fresh
// playlist queue and a stable CPN (the upstream `cpn` field is a
// 16-character hex string).
func newPlayerEngine(host pkgplayer.Player, log logger.Logger) *playerEngine {
	return &playerEngine{
		host:   host,
		queue:  lounge.NewPlaylist(),
		bus:    pkgplayer.NewEventBus(),
		log:    log,
		status: constants.PlayerStatusIdle,
		cpn:    generateCPN(),
	}
}

// generateCPN ports the upstream
//   `uuidv4().replace(/-/g, '').substring(0, 16)` idiom.
// We use crypto/rand directly — the wire only needs 16 hex chars of
// per-session uniqueness, not RFC4122 compliance.
func generateCPN() string {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}

// SetLogger replaces the engine's logger and forwards to the playlist.
func (e *playerEngine) SetLogger(l logger.Logger) {
	e.mu.Lock()
	e.log = l
	e.mu.Unlock()
	if e.queue != nil {
		e.queue.SetLogger(l)
	}
}

// Bus returns the StateEvent fan-out used by the orchestrator.
func (e *playerEngine) Bus() *pkgplayer.EventBus { return e.bus }

// Queue exposes the playlist (upstream `player.queue`).
func (e *playerEngine) Queue() *lounge.Playlist { return e.queue }

// Status returns the current PlayerStatus.
func (e *playerEngine) Status() constants.PlayerStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.status
}

// CPN returns the current cpn (16 hex chars).
func (e *playerEngine) CPN() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cpn
}

// AutoplayMode mirrors `player.autoplayMode` — sourced from the queue.
func (e *playerEngine) AutoplayMode() constants.AutoplayMode {
	if e.queue == nil {
		return constants.AutoplayModeUnsupported
	}
	return e.queue.AutoplayMode()
}

// ZeroVolumeLevelOnMute mirrors `player.zeroVolumeLevelOnMute`.
func (e *playerEngine) ZeroVolumeLevelOnMute() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.zeroVolumeLevelOnMute
}

// SetZeroVolumeLevelOnMute ports `setZeroVolumeLevelOnMute` — toggles
// the flag and, if turning ON while muted at non-zero volume, calls
// SetVolume to enforce the policy.
func (e *playerEngine) SetZeroVolumeLevelOnMute(ctx context.Context, value bool, aid *int) error {
	e.mu.Lock()
	e.zeroVolumeLevelOnMute = value
	e.mu.Unlock()

	cur, err := e.getVolume(ctx)
	if err != nil {
		return err
	}
	if value && cur.Muted && cur.Level > 0 {
		if l := e.logger(); l != nil {
			l.Debug(fmt.Sprintf("[yt-cast-receiver] Calling setVolume() to enforce 'zeroVolumeLevelOnMute: %v'...", value))
		}
		if _, err := e.SetVolume(ctx, cur, aid); err != nil {
			return err
		}
	}
	return nil
}

// GetState ports `getState()`. Position / duration / volume are pulled
// from the host on every call (matching upstream's `await
// doGetPosition()` etc.). Queue is the lounge.PlaylistState value the
// orchestrator can type-assert if it wants the full structure.
func (e *playerEngine) GetState(ctx context.Context) (pkgplayer.State, error) {
	pos, err := e.host.DoGetPosition(ctx)
	if err != nil {
		return pkgplayer.State{}, err
	}
	dur, err := e.host.DoGetDuration(ctx)
	if err != nil {
		return pkgplayer.State{}, err
	}
	vol, err := e.getVolume(ctx)
	if err != nil {
		return pkgplayer.State{}, err
	}
	e.mu.Lock()
	status := e.status
	cpn := e.cpn
	e.mu.Unlock()
	queueState := e.queue.GetState()
	return pkgplayer.State{
		Status:   status,
		Queue:    queueState,
		Position: pos,
		Duration: dur,
		Volume:   vol,
		CPN:      cpn,
	}, nil
}

// NavInfo carries the result of `getNavInfo()` — upstream wraps the
// three fields in a `PlayerNavInfo` interface. We mirror that locally
// rather than adding a foundation type because only the orchestrator
// reads it.
type NavInfo struct {
	HasPrevious  bool                   `json:"hasPrevious"`
	HasNext      bool                   `json:"hasNext"`
	AutoplayMode constants.AutoplayMode `json:"autoplayMode"`
}

// NavInfo ports `getNavInfo()`.
func (e *playerEngine) NavInfo() NavInfo {
	return NavInfo{
		HasPrevious:  e.queue.HasPrevious(),
		HasNext:      e.queue.HasNext(),
		AutoplayMode: e.AutoplayMode(),
	}
}

// Play ports `play(video, position, AID)`.
func (e *playerEngine) Play(ctx context.Context, video types.Video, position float64, aid *int) (bool, error) {
	if e.Status() == constants.PlayerStatusPlaying {
		if _, err := e.Stop(ctx, nil); err != nil {
			return false, err
		}
	}
	if l := e.logger(); l != nil {
		l.Info(fmt.Sprintf("[yt-cast-receiver] Player.play(): %s @ %vs", video.ID, position))
	}
	e.queue.SetAsCurrent(video)
	if err := e.setStatusAndEmit(ctx, constants.PlayerStatusLoading, aid); err != nil {
		return false, err
	}
	if err := e.host.DoPlay(ctx, video, position); err != nil {
		// Failure path — surface stopped state so senders drop the
		// loading spinner, matching upstream's `else` branch.
		_ = e.setStatusAndEmit(ctx, constants.PlayerStatusStopped, aid)
		return false, err
	}
	if err := e.setStatusAndEmit(ctx, constants.PlayerStatusPlaying, aid); err != nil {
		return true, err
	}
	return true, nil
}

// Pause ports `pause(AID)`.
func (e *playerEngine) Pause(ctx context.Context, aid *int) (bool, error) {
	if l := e.logger(); l != nil {
		l.Info("[yt-cast-receiver] Player.pause()")
	}
	if e.Status() != constants.PlayerStatusPlaying {
		return false, nil
	}
	if err := e.host.DoPause(ctx); err != nil {
		return false, err
	}
	if err := e.setStatusAndEmit(ctx, constants.PlayerStatusPaused, aid); err != nil {
		return true, err
	}
	return true, nil
}

// Resume ports `resume(AID)`.
func (e *playerEngine) Resume(ctx context.Context, aid *int) (bool, error) {
	if l := e.logger(); l != nil {
		l.Info("[yt-cast-receiver] Player.resume()")
	}
	switch e.Status() {
	case constants.PlayerStatusPlaying:
		_ = e.setStatusAndEmit(ctx, constants.PlayerStatusPlaying, aid)
		return false, nil
	case constants.PlayerStatusPaused:
		if err := e.host.DoResume(ctx); err != nil {
			return false, err
		}
		if err := e.setStatusAndEmit(ctx, constants.PlayerStatusPlaying, aid); err != nil {
			return true, err
		}
		return true, nil
	}
	if cur := e.queue.Current(); cur != nil {
		return e.Play(ctx, *cur, 0, aid)
	}
	return false, nil
}

// Stop ports `stop(AID)`.
func (e *playerEngine) Stop(ctx context.Context, aid *int) (bool, error) {
	if s := e.Status(); s == constants.PlayerStatusStopped || s == constants.PlayerStatusIdle {
		return true, nil
	}
	if l := e.logger(); l != nil {
		l.Info("[yt-cast-receiver] Player.stop()")
	}
	if err := e.host.DoStop(ctx); err != nil {
		return false, err
	}
	if err := e.setStatusAndEmit(ctx, constants.PlayerStatusStopped, aid); err != nil {
		return true, err
	}
	return true, nil
}

// Seek ports `seek(position, AID)`. The `fakeState` step in upstream is
// preserved verbatim — emit a synthetic loading-at-position state, do
// the seek, then either resume from paused or transition to playing.
func (e *playerEngine) Seek(ctx context.Context, position float64, aid *int) (bool, error) {
	if s := e.Status(); s != constants.PlayerStatusPlaying && s != constants.PlayerStatusPaused {
		return false, nil
	}
	if l := e.logger(); l != nil {
		l.Info(fmt.Sprintf("[yt-cast-receiver] Player.seek(): %vs", position))
	}
	prev, err := e.GetState(ctx)
	if err != nil {
		return false, err
	}
	fake := prev
	fake.Position = position
	fake.Status = constants.PlayerStatusLoading
	prevCopy := prev
	e.bus.Publish(pkgplayer.StateEvent{AID: aid, Current: fake, Previous: &prevCopy})

	if err := e.host.DoSeek(ctx, position); err != nil {
		return false, err
	}
	e.mu.Lock()
	fakeCopy := fake
	e.previousState = &fakeCopy
	wasPaused := prev.Status == constants.PlayerStatusPaused
	if wasPaused {
		e.status = constants.PlayerStatusPaused
	}
	e.mu.Unlock()
	if wasPaused {
		return e.Resume(ctx, aid)
	}
	if err := e.setStatusAndEmit(ctx, constants.PlayerStatusPlaying, aid); err != nil {
		return true, err
	}
	return true, nil
}

// Next ports `next(AID)`. Returns true on successful playback.
func (e *playerEngine) Next(ctx context.Context, aid *int) (bool, error) {
	if e.queue.IsUpdating() {
		if l := e.logger(); l != nil {
			l.Debug("[yt-cast-receiver] Player.next() ignored: queue is updating.")
		}
		return false, nil
	}
	if l := e.logger(); l != nil {
		l.Info("[yt-cast-receiver] Player.next()")
	}
	nxt, err := e.queue.Next(ctx)
	if err != nil {
		return false, err
	}
	if nxt == nil {
		if l := e.logger(); l != nil {
			l.Info("[yt-cast-receiver] No next video in queue.")
		}
		if e.AutoplayMode() == constants.AutoplayModeEnabled {
			if auto := e.queue.Autoplay(); auto != nil {
				if l := e.logger(); l != nil {
					l.Info(fmt.Sprintf("[yt-cast-receiver] Play autoplay video: %s.", auto.ID))
				}
				return e.Play(ctx, *auto, 0, aid)
			}
			if l := e.logger(); l != nil {
				l.Info("[yt-cast-receiver] No autoplay video available.")
			}
		} else if l := e.logger(); l != nil {
			l.Info("[yt-cast-receiver] No autoplay video - autoplay is disabled.")
		}
		_, _ = e.Stop(ctx, aid)
		return false, nil
	}
	return e.Play(ctx, *nxt, 0, nil)
}

// Previous ports `previous(AID)`.
func (e *playerEngine) Previous(ctx context.Context, aid *int) (bool, error) {
	if e.queue.IsUpdating() {
		if l := e.logger(); l != nil {
			l.Debug("[yt-cast-receiver] Player.previous() ignored: queue is updating.")
		}
		return false, nil
	}
	if l := e.logger(); l != nil {
		l.Info("[yt-cast-receiver] Player.previous()")
	}
	prv, err := e.queue.Previous(ctx)
	if err != nil {
		return false, err
	}
	if prv == nil {
		if l := e.logger(); l != nil {
			l.Info("[yt-cast-receiver] No previous video in queue.")
		}
		_, _ = e.Stop(ctx, aid)
		return false, nil
	}
	return e.Play(ctx, *prv, 0, aid)
}

// SetVolume ports `setVolume(volume, AID)`.
func (e *playerEngine) SetVolume(ctx context.Context, vol pkgplayer.Volume, aid *int) (bool, error) {
	v := vol
	e.mu.Lock()
	zero := e.zeroVolumeLevelOnMute
	e.mu.Unlock()
	if zero && v.Muted && v.Level > 0 {
		if l := e.logger(); l != nil {
			l.Debug("[yt-cast-receiver] Enforcing 'zeroVolumeLevelOnMute: true'...")
		}
		v.Level = 0
	}
	if l := e.logger(); l != nil {
		l.Info(fmt.Sprintf("[yt-cast-receiver] Player.setVolume(): %+v", v))
	}
	prev, err := e.GetState(ctx)
	if err != nil {
		return false, err
	}
	if err := e.host.DoSetVolume(ctx, v); err != nil {
		return false, err
	}
	cur, err := e.GetState(ctx)
	if err != nil {
		return true, err
	}
	prevCopy := prev
	e.bus.Publish(pkgplayer.StateEvent{AID: aid, Current: cur, Previous: &prevCopy})
	return true, nil
}

// Reset ports `reset(AID)`.
func (e *playerEngine) Reset(ctx context.Context, aid *int) {
	if l := e.logger(); l != nil {
		l.Info("[yt-cast-receiver] Player.reset()")
	}
	e.queue.Reset()
	_, _ = e.Stop(ctx, aid)
	e.mu.Lock()
	e.previousState = nil
	e.mu.Unlock()
	if err := e.setStatusAndEmit(ctx, constants.PlayerStatusIdle, aid); err != nil {
		if l := e.logger(); l != nil {
			l.Error("[yt-cast-receiver] Caught error emitting status after player reset:", err)
		}
	}
}

// NotifyExternalStateChange ports `notifyExternalStateChange(newStatus?)`.
func (e *playerEngine) NotifyExternalStateChange(ctx context.Context, newStatus *constants.PlayerStatus) error {
	if newStatus == nil {
		return e.emitState(ctx, nil)
	}
	return e.setStatusAndEmit(ctx, *newStatus, nil)
}

// setStatusAndEmit ports `#setStatusAndEmit(status?, AID?)`.
func (e *playerEngine) setStatusAndEmit(ctx context.Context, status constants.PlayerStatus, aid *int) error {
	e.mu.Lock()
	e.status = status
	e.mu.Unlock()
	return e.emitState(ctx, aid)
}

// emitState publishes a state event with the cached previous frame.
func (e *playerEngine) emitState(ctx context.Context, aid *int) error {
	cur, err := e.GetState(ctx)
	if err != nil {
		return err
	}
	e.mu.Lock()
	prev := e.previousState
	curCopy := cur
	e.previousState = &curCopy
	e.mu.Unlock()
	e.bus.Publish(pkgplayer.StateEvent{AID: aid, Current: cur, Previous: prev})
	return nil
}

// getVolume ports `getVolume()` — clamps level into [0, 100].
func (e *playerEngine) getVolume(ctx context.Context) (pkgplayer.Volume, error) {
	v, err := e.host.DoGetVolume(ctx)
	if err != nil {
		return pkgplayer.Volume{}, err
	}
	if v.Level < 0 {
		v.Level = 0
	} else if v.Level > 100 {
		v.Level = 100
	}
	return v, nil
}

func (e *playerEngine) logger() logger.Logger {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.log
}
