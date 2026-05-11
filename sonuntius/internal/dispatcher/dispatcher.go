// Package dispatcher translates IPC events into Home Assistant service
// calls against the configured Music Assistant player entity.
package dispatcher

import (
	"context"
	"log/slog"

	"github.com/shobuprime/sonuntius/internal/events"
	"github.com/shobuprime/sonuntius/internal/ha"
)

// uriTemplates maps event provider names to MA's content_id schemes.
// Confirmed schemes go here; new ones land via Phase-3 reverse engineering.
var uriTemplates = map[string]string{
	"ytmusic": "ytmusic://track/%s",
	"tidal":   "tidal://track/%s",
}

// transportToService maps TransportCommand.Command to the HA
// media_player.<service> name.
var transportToService = map[string]string{
	"play":     "media_play",
	"pause":    "media_pause",
	"stop":     "media_stop",
	"next":     "media_next_track",
	"previous": "media_previous_track",
	"seek":     "media_seek",
}

// Dispatcher routes events to the HA client.
type Dispatcher struct {
	HA       *ha.Client
	EntityID string
	Logger   *slog.Logger
}

// New constructs a Dispatcher.
func New(haClient *ha.Client, entityID string, logger *slog.Logger) *Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{HA: haClient, EntityID: entityID, Logger: logger}
}

// Ready reports whether the dispatcher has a target entity configured.
// When false, Dispatch logs and drops events instead of calling HA.
func (d *Dispatcher) Ready() bool {
	return d.EntityID != ""
}

// Dispatch routes one event. Errors are logged but not propagated so a
// single failure does not kill the IPC reader goroutine.
func (d *Dispatcher) Dispatch(ctx context.Context, ev events.Event) {
	if !d.Ready() {
		d.Logger.Warn("dispatcher: idle (ma_player_id unset)", "type", ev.EventType())
		return
	}
	switch e := ev.(type) {
	case *events.PlayIntent:
		d.dispatchPlay(ctx, e)
	case *events.TransportCommand:
		d.dispatchTransport(ctx, e)
	case *events.VolumeCommand:
		d.dispatchVolume(ctx, e)
	default:
		d.Logger.Debug("dispatcher: ignoring event", "type", ev.EventType())
	}
}

func (d *Dispatcher) dispatchPlay(ctx context.Context, p *events.PlayIntent) {
	uri := ResolveURI(p)
	if uri == "" {
		d.Logger.Warn("dispatcher: dropping unresolved play intent",
			"provider", p.Provider, "track_id", p.TrackID, "url", p.URL)
		return
	}
	if err := d.HA.PlayMedia(ctx, d.EntityID, uri, "music"); err != nil {
		d.Logger.Error("dispatcher: play_media failed", "err", err)
	}
}

func (d *Dispatcher) dispatchTransport(ctx context.Context, t *events.TransportCommand) {
	svc, ok := transportToService[t.Command]
	if !ok {
		d.Logger.Warn("dispatcher: unknown transport command", "command", t.Command)
		return
	}
	var data map[string]any
	if t.Command == "seek" && t.Position != nil {
		data = map[string]any{"seek_position": *t.Position}
	}
	if err := d.HA.MediaPlayerCommand(ctx, d.EntityID, svc, data); err != nil {
		d.Logger.Error("dispatcher: transport failed", "command", t.Command, "err", err)
	}
}

func (d *Dispatcher) dispatchVolume(ctx context.Context, v *events.VolumeCommand) {
	if v.Muted != nil {
		if err := d.HA.MediaPlayerCommand(ctx, d.EntityID, "volume_mute",
			map[string]any{"is_volume_muted": *v.Muted}); err != nil {
			d.Logger.Error("dispatcher: volume_mute failed", "err", err)
		}
	}
	if v.Level != nil {
		if err := d.HA.MediaPlayerCommand(ctx, d.EntityID, "volume_set",
			map[string]any{"volume_level": *v.Level}); err != nil {
			d.Logger.Error("dispatcher: volume_set failed", "err", err)
		}
	}
}

// ResolveURI maps a PlayIntent onto a media_content_id for HA. Returns
// "" when the intent is unmappable.
func ResolveURI(p *events.PlayIntent) string {
	if p.Provider == "url" && p.URL != "" {
		return p.URL
	}
	tmpl, ok := uriTemplates[p.Provider]
	if !ok || p.TrackID == "" {
		return ""
	}
	return fmtTemplate(tmpl, p.TrackID)
}

// fmtTemplate replaces the single %s in tmpl with v. We avoid fmt.Sprintf
// only to keep the call site cheap and clear; fmt would work too.
func fmtTemplate(tmpl, v string) string {
	idx := indexByte(tmpl, '%')
	if idx < 0 || idx+1 >= len(tmpl) || tmpl[idx+1] != 's' {
		return tmpl
	}
	return tmpl[:idx] + v + tmpl[idx+2:]
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
