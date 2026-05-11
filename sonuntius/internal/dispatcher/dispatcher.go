// Package dispatcher translates IPC events into Home Assistant service
// calls against the configured Music Assistant player entity.
package dispatcher

import (
	"context"
	"log/slog"

	"github.com/shobuprime/sonuntius/internal/events"
	"github.com/shobuprime/sonuntius/internal/ha"
	"github.com/shobuprime/sonuntius/internal/ma"
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

// Dispatcher routes events to the HA client. When MAWsURL is set the
// dispatcher also has a direct path to MA's WebSocket so url-provider
// PlayIntents can be sent as a fully-formed MediaItem (preserving
// title / artist / image), bypassing HA's media_player.play_media
// wrapper which strips metadata for the URL provider.
type Dispatcher struct {
	HA       *ha.Client
	EntityID string
	Logger   *slog.Logger

	// MA-WS direct path (optional). When MAWsURL is non-empty the
	// dispatcher tries this first for url-provider intents and falls
	// back to the HA REST path on any error.
	MAWsURL       string
	MAToken       string
	MAPlayerQueue string
}

// New constructs a Dispatcher.
func New(haClient *ha.Client, entityID string, logger *slog.Logger) *Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{HA: haClient, EntityID: entityID, Logger: logger}
}

// SetMAWS configures the MA WS direct path. queueID is MA's internal
// player_id (NOT the HA entity_id); use ma.DerivePlayerID to derive
// it from the entity_id, or accept an explicit override from config.
func (d *Dispatcher) SetMAWS(url, token, queueID string) {
	d.MAWsURL = url
	d.MAToken = token
	d.MAPlayerQueue = queueID
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

	// For url-provider intents prefer MA's native WS play_media when
	// it's configured. MA's URL provider strips most metadata when
	// routed through HA's media_player.play_media service; the WS
	// path accepts a full MediaItem, so the title / artist / image
	// land in MA's UI.
	if p.Provider == "url" && d.MAWsURL != "" && d.MAPlayerQueue != "" {
		if err := d.playViaMAWS(ctx, uri, p); err == nil {
			return
		} else {
			d.Logger.Warn("dispatcher: MA WS play_media failed, falling back to HA REST",
				"err", err)
		}
	}

	extra := metadataExtra(p.Metadata)
	if err := d.HA.PlayMedia(ctx, d.EntityID, uri, "music", extra); err != nil {
		d.Logger.Error("dispatcher: play_media failed", "err", err)
	}
}

// playViaMAWS sends a fully-formed MediaItem to MA's native WS
// `player_queues/play_media` command. Used for url-provider intents
// where rich metadata would otherwise be lost.
func (d *Dispatcher) playViaMAWS(ctx context.Context, uri string, p *events.PlayIntent) error {
	title, _ := p.Metadata["title"].(string)
	channel, _ := p.Metadata["channel"].(string)
	thumb, _ := p.Metadata["thumbnail"].(string)
	source, _ := p.Metadata["source"].(string)
	if source == "" {
		source = "builtin"
	}
	item := ma.MediaItem{
		ItemID:    uri,
		Provider:  "builtin",
		Name:      title,
		MediaType: "track",
		URI:       uri,
	}
	if channel != "" {
		item.Artists = []string{channel}
	}
	if thumb != "" {
		item.Image = &ma.MediaItemImage{
			Type:     "thumb",
			Path:     thumb,
			Provider: "url",
		}
	}
	return ma.PlayMediaItem(ctx, d.MAWsURL, d.MAToken, d.MAPlayerQueue, item, d.Logger.With("path", "ma-ws"))
}

// metadataExtra translates PlayIntent.Metadata (populated by the
// receivers with title / channel / thumbnail / video_id / source) into
// the shape Music Assistant's play_media service consumes. Returns nil
// when there is nothing to pass through so the dispatcher omits the
// field instead of sending an empty map.
//
// We emit fields under BOTH `extra.<field>` (flat) AND
// `extra.metadata.<field>` (nested). Different MA versions and
// provider code paths read from different locations — the flat form
// is what the URL provider's track resolver looks at in current
// releases, while the nested form is what the play_media schema
// historically documented. Emitting both maximises the chance that
// MA's UI picks up the title / artist / artwork.
func metadataExtra(meta map[string]any) map[string]any {
	if len(meta) == 0 {
		return nil
	}
	title, _ := meta["title"].(string)
	// MA's media item model uses `artist`; "channel" is the YouTube-side
	// equivalent we receive from the oEmbed lookup.
	artist, _ := meta["channel"].(string)
	thumb, _ := meta["thumbnail"].(string)
	videoID, _ := meta["video_id"].(string)
	source, _ := meta["source"].(string)

	nested := map[string]any{}
	if title != "" {
		nested["title"] = title
	}
	if artist != "" {
		nested["artist"] = artist
	}
	if thumb != "" {
		nested["image"] = thumb
		nested["thumb"] = thumb
		nested["artwork"] = thumb
	}
	if videoID != "" {
		nested["external_id"] = videoID
	}
	if source != "" {
		nested["source"] = source
	}
	if len(nested) == 0 {
		return nil
	}

	out := map[string]any{"metadata": nested}
	// Mirror title/thumb/etc. at the top level for MA versions that
	// read directly from `extra.<field>`.
	if title != "" {
		out["title"] = title
	}
	if artist != "" {
		out["artist"] = artist
	}
	if thumb != "" {
		out["thumb"] = thumb
		out["image"] = thumb
	}
	return out
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
