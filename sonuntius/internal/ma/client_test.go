package ma

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"testing"

	"github.com/shobuprime/sonuntius/internal/events"
)

func TestURLFromHost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		host string
		want string
	}{
		{"empty", "", ""},
		{"plain host", "music-assistant", "ws://music-assistant:8095/ws"},
		{"docker hostname", "a0d7b954-music-assistant", "ws://a0d7b954-music-assistant:8095/ws"},
		{"with dots", "music-assistant.local", "ws://music-assistant.local:8095/ws"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := URLFromHost(tc.host); got != tc.want {
				t.Errorf("URLFromHost(%q) = %q, want %q", tc.host, got, tc.want)
			}
		})
	}
}

// captureBroadcaster records every event broadcast for assertions.
type captureBroadcaster struct {
	mu     sync.Mutex
	events []events.Event
}

func (c *captureBroadcaster) Broadcast(e events.Event) {
	c.mu.Lock()
	c.events = append(c.events, e)
	c.mu.Unlock()
}

func (c *captureBroadcaster) last() events.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.events) == 0 {
		return nil
	}
	return c.events[len(c.events)-1]
}

func TestHandleFrame_PlayerUpdated_BroadcastsPlayerState(t *testing.T) {
	t.Parallel()
	bus := &captureBroadcaster{}
	w := &Watcher{
		PlayerID: "sendspin_living_room",
		IPC:      bus,
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}

	frame := map[string]any{
		"event":     "player_updated",
		"object_id": "sendspin_living_room",
		"data": map[string]any{
			"player_id":    "sendspin_living_room",
			"state":        "playing",
			"volume_level": 0.42,
			"volume_muted": false,
			"current_item": map[string]any{
				"media_item": map[string]any{
					"name":    "Sample Track",
					"item_id": "tidal://track/12345",
					"duration": 245.0,
					"artists": []map[string]any{
						{"name": "Sample Artist"},
					},
				},
				"stream_details": map[string]any{
					"position": 17.5,
				},
			},
		},
	}
	raw, _ := json.Marshal(frame)
	w.handleFrame(raw)

	ev := bus.last()
	if ev == nil {
		t.Fatal("expected a broadcast, got none")
	}
	ps, ok := ev.(*events.PlayerState)
	if !ok {
		t.Fatalf("expected *events.PlayerState, got %T", ev)
	}
	if ps.State != "playing" {
		t.Errorf("State = %q, want %q", ps.State, "playing")
	}
	if ps.Title != "Sample Track" {
		t.Errorf("Title = %q, want %q", ps.Title, "Sample Track")
	}
	if ps.Artist != "Sample Artist" {
		t.Errorf("Artist = %q, want %q", ps.Artist, "Sample Artist")
	}
	if ps.TrackID != "tidal://track/12345" {
		t.Errorf("TrackID = %q, want %q", ps.TrackID, "tidal://track/12345")
	}
	if ps.Volume == nil || *ps.Volume != 0.42 {
		t.Errorf("Volume = %v, want 0.42", ps.Volume)
	}
	if ps.Duration == nil || *ps.Duration != 245.0 {
		t.Errorf("Duration = %v, want 245.0", ps.Duration)
	}
	if ps.Position == nil || *ps.Position != 17.5 {
		t.Errorf("Position = %v, want 17.5", ps.Position)
	}
}

func TestHandleFrame_FiltersByPlayerID(t *testing.T) {
	t.Parallel()
	bus := &captureBroadcaster{}
	w := &Watcher{
		PlayerID: "wanted_player",
		IPC:      bus,
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}

	frame := map[string]any{
		"event":     "player_updated",
		"object_id": "different_player",
		"data": map[string]any{
			"player_id": "different_player",
			"state":     "playing",
		},
	}
	raw, _ := json.Marshal(frame)
	w.handleFrame(raw)

	if got := bus.last(); got != nil {
		t.Errorf("expected no broadcast for non-matching player, got %T", got)
	}
}

func TestHandleFrame_IgnoresCommandResults(t *testing.T) {
	t.Parallel()
	bus := &captureBroadcaster{}
	w := &Watcher{
		IPC:    bus,
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}

	frame := map[string]any{
		"message_id": "abc123",
		"result":     map[string]any{"ok": true},
	}
	raw, _ := json.Marshal(frame)
	w.handleFrame(raw)

	if got := bus.last(); got != nil {
		t.Errorf("command-result frames should be ignored, got %T", got)
	}
}

func TestHandleFrame_IgnoresUnknownEvents(t *testing.T) {
	t.Parallel()
	bus := &captureBroadcaster{}
	w := &Watcher{
		IPC:    bus,
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}

	frame := map[string]any{
		"event": "music_library_changed",
		"data":  map[string]any{"foo": "bar"},
	}
	raw, _ := json.Marshal(frame)
	w.handleFrame(raw)

	if got := bus.last(); got != nil {
		t.Errorf("unknown events should be ignored, got %T", got)
	}
}
