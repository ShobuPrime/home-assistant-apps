// Maps to: N/A — Go-only smoke tests for the orchestrator port.
//
// The full session lifecycle requires talking to a live lounge HTTP
// server (see lounge/session.go), so these tests focus on the seams
// the orchestrator owns: Launch's payload validation, the engine's
// state-machine wrapping of the do-only Player, and event emission.
package ytcast

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/shobuprime/sonuntius/internal/ytcast/constants"
	pkgplayer "github.com/shobuprime/sonuntius/internal/ytcast/player"
	"github.com/shobuprime/sonuntius/internal/ytcast/types"
	"github.com/shobuprime/sonuntius/internal/ytcast/yterrors"
)

// stubPlayer is a do-only Player that records each call.
type stubPlayer struct {
	mu      sync.Mutex
	calls   []string
	volume  pkgplayer.Volume
	pos     float64
	dur     float64
	failPlay bool
}

func (s *stubPlayer) record(name string) {
	s.mu.Lock()
	s.calls = append(s.calls, name)
	s.mu.Unlock()
}
func (s *stubPlayer) Calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.calls))
	copy(out, s.calls)
	return out
}
func (s *stubPlayer) DoPlay(_ context.Context, v types.Video, _ float64) error {
	s.record("play:" + v.ID)
	if s.failPlay {
		return errors.New("play failed")
	}
	return nil
}
func (s *stubPlayer) DoPause(context.Context) error  { s.record("pause"); return nil }
func (s *stubPlayer) DoResume(context.Context) error { s.record("resume"); return nil }
func (s *stubPlayer) DoStop(context.Context) error   { s.record("stop"); return nil }
func (s *stubPlayer) DoSeek(_ context.Context, p float64) error {
	s.record("seek")
	s.pos = p
	return nil
}
func (s *stubPlayer) DoSetVolume(_ context.Context, v pkgplayer.Volume) error {
	s.record("setVolume")
	s.volume = v
	return nil
}
func (s *stubPlayer) DoGetVolume(context.Context) (pkgplayer.Volume, error) {
	return s.volume, nil
}
func (s *stubPlayer) DoGetPosition(context.Context) (float64, error) { return s.pos, nil }
func (s *stubPlayer) DoGetDuration(context.Context) (float64, error) { return s.dur, nil }

func TestEnginePlayPauseResumeStop(t *testing.T) {
	sp := &stubPlayer{}
	eng := newPlayerEngine(sp, nil)
	ctx := context.Background()

	video := types.Video{ID: "abc", Client: types.Clients[types.ClientKeyYTMusic]}
	if _, err := eng.Play(ctx, video, 0, nil); err != nil {
		t.Fatalf("Play: %v", err)
	}
	if eng.Status() != constants.PlayerStatusPlaying {
		t.Fatalf("status after Play = %v want PLAYING", eng.Status())
	}
	if _, err := eng.Pause(ctx, nil); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if eng.Status() != constants.PlayerStatusPaused {
		t.Fatalf("status after Pause = %v want PAUSED", eng.Status())
	}
	if _, err := eng.Resume(ctx, nil); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if eng.Status() != constants.PlayerStatusPlaying {
		t.Fatalf("status after Resume = %v want PLAYING", eng.Status())
	}
	if _, err := eng.Stop(ctx, nil); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if eng.Status() != constants.PlayerStatusStopped {
		t.Fatalf("status after Stop = %v want STOPPED", eng.Status())
	}

	want := []string{"play:abc", "pause", "resume", "stop"}
	got := sp.Calls()
	if len(got) != len(want) {
		t.Fatalf("calls = %v want %v", got, want)
	}
	for i, c := range want {
		if got[i] != c {
			t.Fatalf("call %d = %q want %q", i, got[i], c)
		}
	}
}

func TestEngineStateEvents(t *testing.T) {
	sp := &stubPlayer{}
	eng := newPlayerEngine(sp, nil)
	ctx := context.Background()

	sub := eng.Bus().Subscribe(8)
	video := types.Video{ID: "x", Client: types.Clients[types.ClientKeyYT]}
	if _, err := eng.Play(ctx, video, 0, nil); err != nil {
		t.Fatalf("Play: %v", err)
	}

	deadline := time.After(time.Second)
	events := []constants.PlayerStatus{}
loop:
	for {
		select {
		case ev, ok := <-sub:
			if !ok {
				break loop
			}
			events = append(events, ev.Current.Status)
			if ev.Current.Status == constants.PlayerStatusPlaying {
				break loop
			}
		case <-deadline:
			t.Fatalf("did not observe PLAYING within 1s; got %v", events)
		}
	}
	// Expect at minimum LOADING followed by PLAYING.
	if len(events) < 2 || events[0] != constants.PlayerStatusLoading || events[len(events)-1] != constants.PlayerStatusPlaying {
		t.Fatalf("state events = %v; expected LOADING → … → PLAYING", events)
	}
}

func TestYouTubeAppLaunchRejectsMissingPairingCode(t *testing.T) {
	sp := &stubPlayer{}
	app := NewYouTubeApp(sp, AppOptions{ScreenName: "test"})
	_, err := app.Launch(context.Background(), LaunchOptions{Theme: "cl"})
	if err == nil {
		t.Fatalf("Launch with missing pairingCode should error")
	}
	if !errors.Is(err, yterrors.ErrApp) {
		t.Fatalf("expected ErrApp, got %T: %v", err, err)
	}
}

func TestYouTubeAppLaunchRejectsUnknownTheme(t *testing.T) {
	sp := &stubPlayer{}
	app := NewYouTubeApp(sp, AppOptions{ScreenName: "test"})
	_, err := app.Launch(context.Background(), LaunchOptions{Theme: "zz", PairingCode: "1234"})
	if err == nil {
		t.Fatalf("Launch with unknown theme should error")
	}
	if !errors.Is(err, yterrors.ErrApp) {
		t.Fatalf("expected ErrApp, got %T: %v", err, err)
	}
}

func TestReceiverStatusBeforeStart(t *testing.T) {
	sp := &stubPlayer{}
	r, err := NewReceiver(Options{
		Player: sp,
		Device: DeviceOptions{Name: "Sonuntius (test)"},
		Dial:   DialOptions{Port: 0, UUID: "00000000-0000-4000-8000-000000000000"},
	})
	if err != nil {
		t.Fatalf("NewReceiver: %v", err)
	}
	if r.Status() != constants.StatusStopped {
		t.Fatalf("Status() before Start = %v want stopped", r.Status())
	}
}
