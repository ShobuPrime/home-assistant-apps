// Package dispatcher translates IPC events into Home Assistant service
// calls against the configured Music Assistant player entity.
package dispatcher

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

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

	// authWarned tracks whether we've already surfaced the loud
	// "ma_token not configured" warning for ErrAuthRequired. Without
	// this the warning would fire on every PlayIntent — instead, log
	// once at warn, and at debug on subsequent attempts.
	authWarned atomic.Bool
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
		// Clear MA's queue first so leftover library/autoplay tracks
		// don't sit behind our item. Best-effort — if clear fails
		// we still try play_media (with option:"play" MA only
		// replaces the current item; queue items remain, but the
		// user has a more degraded but not broken experience).
		if err := ma.ClearQueue(ctx, d.MAWsURL, d.MAToken,
			d.MAPlayerQueue, d.Logger.With("path", "ma-ws-clear")); err != nil {
			d.Logger.Warn("dispatcher: queue clear failed (continuing to play_media)",
				"err", err)
		}
		if err := d.playViaMAWS(ctx, uri, p); err == nil {
			d.maybeSeekAfterPlay(ctx, p)
			return
		} else if errors.Is(err, ma.ErrAuthRequired) {
			// MA's URL provider is what would surface our metadata in
			// the MA UI. Without auth we can't reach it. Tell the user
			// once at warn, then degrade to debug so the log doesn't
			// drown in repeats — the HA REST fallback below still
			// plays audio, just without rich metadata.
			if !d.authWarned.Swap(true) {
				d.Logger.Warn(
					"dispatcher: MA WS requires a long-lived API token to display title/artist/thumbnail in the MA UI",
					"how_to_fix",
					"In Music Assistant: Settings → Security → API Tokens → create a token, then paste it into the Sonuntius addon option ma_token and restart the addon.",
					"err", err,
				)
			} else {
				d.Logger.Debug("dispatcher: MA WS auth still missing — using HA REST fallback",
					"err", err)
			}
		} else {
			d.Logger.Warn("dispatcher: MA WS play_media failed, falling back to HA REST",
				"err", err)
		}
	}

	extra := metadataExtra(p.Metadata)
	if err := d.HA.PlayMedia(ctx, d.EntityID, uri, "music", extra); err != nil {
		d.Logger.Error("dispatcher: play_media failed", "err", err)
		return
	}
	d.maybeSeekAfterPlay(ctx, p)
}

// maybeSeekAfterPlay fires media_seek on the entity if the play intent
// carried a non-zero start position. MA's `player_queues/play_media`
// has no built-in "start at N seconds" argument; the convention is
// to follow play_media with a seek. Without this, casting at 7:28
// from the YouTube app would still start playback at 0:00 on the
// speaker.
//
// Runs in the foreground but errors are non-fatal — playback already
// started, we just can't honor the requested offset.
func (d *Dispatcher) maybeSeekAfterPlay(ctx context.Context, p *events.PlayIntent) {
	if p.StartPosition <= 0.5 {
		return
	}
	// Give MA a moment to ingest the play_media before seeking — a
	// seek issued immediately can race the queue-item-loaded state
	// and be dropped silently. 500ms is a conservative single-RTT
	// budget.
	go func(pos float64) {
		select {
		case <-time.After(500 * time.Millisecond):
		case <-ctx.Done():
			return
		}
		if err := d.HA.MediaPlayerCommand(ctx, d.EntityID, "media_seek",
			map[string]any{"seek_position": pos}); err != nil {
			d.Logger.Warn("dispatcher: post-play seek failed",
				"position", pos, "err", err)
			return
		}
		d.Logger.Info("dispatcher: post-play seek issued",
			"position", pos, "entity", d.EntityID)
	}(p.StartPosition)
}

// playViaMAWS sends a fully-formed MediaItem to MA's native WS
// `player_queues/play_media` command. Used for url-provider intents
// where rich metadata would otherwise be lost.
//
// item_id is the stream URL itself. The previous v0.1.12 attempt at
// a synthetic id (`yt_<videoId>`) failed at stream time: MA's
// builtin provider's get_stream_details treats non-URL item_ids as
// filesystem paths, can't find the file, and the queue skips the
// item ("Unable to retrieve info for yt_xxx — No such file or
// directory" in MA's log). Using the URL puts builtin on its
// ffmpeg-probe path, which succeeds for a real audio stream.
//
// Our explicit `name` + `artists` (full Artist dicts) survive the
// probe — MA's QueueItem.from_media_item uses the dict fields we
// provide for display, and ffmpeg's metadata only fills in audio
// stream details. Confirmed against MA 2026.x: queue title rendered
// as "<artist[0].name> - <name>" from our dict even when item_id is
// the URL.
func (d *Dispatcher) playViaMAWS(ctx context.Context, uri string, p *events.PlayIntent) error {
	title, _ := p.Metadata["title"].(string)
	channel, _ := p.Metadata["channel"].(string)
	thumb, _ := p.Metadata["thumbnail"].(string)
	videoID, _ := p.Metadata["video_id"].(string)
	durationVal, _ := p.Metadata["duration"].(float64)

	// item_id must be the URL so MA's builtin provider can stream it
	// (its get_stream_details treats non-URL ids as filesystem paths).
	itemID := uri
	_ = videoID // kept available in metadata for future routing changes

	contentType := guessAudioContentType(uri)
	mapping := ma.MediaItemProviderMapping{
		ItemID:           itemID,
		ProviderDomain:   "builtin",
		ProviderInstance: "builtin",
		Available:        true,
		URL:              uri,
	}
	if contentType != "" {
		mapping.AudioFormat = &ma.MediaItemAudioFormat{
			ContentType: contentType,
			SampleRate:  48000,
			BitDepth:    16,
			Channels:    2,
		}
	}

	item := ma.MediaItem{
		ItemID:           itemID,
		Provider:         "builtin",
		Name:             title,
		Version:          "",
		MediaType:        "track",
		URI:              "builtin://track/" + itemID,
		Available:        true,
		IsPlayable:       true,
		Favorite:         false,
		Duration:         int(durationVal),
		ProviderMappings: []ma.MediaItemProviderMapping{mapping},
		ExternalIDs:      []any{},
		Metadata:         ma.MediaItemMetadata{},
	}
	if channel != "" {
		item.Artists = []ma.MediaItemArtist{{
			ItemID:    "yt_channel_" + slugifyChannel(channel),
			Provider:  "builtin",
			Name:      channel,
			MediaType: "artist",
			Available: true,
		}}
	}
	if thumb != "" {
		item.Metadata.Images = []ma.MediaItemImage{{
			Type:               "thumb",
			Path:               thumb,
			Provider:           "url",
			RemotelyAccessible: true,
		}}
	}
	return ma.PlayMediaItem(ctx, d.MAWsURL, d.MAToken, d.MAPlayerQueue, item, d.Logger.With("path", "ma-ws"))
}

// guessAudioContentType derives an MA-style content_type from the URL.
// We're forwarding YouTube googlevideo audio streams (`mime=audio/webm`
// for opus, `mime=audio/mp4` for AAC). Returns "" if unknown — MA
// then ffmpeg-probes for the codec on its own.
func guessAudioContentType(rawURL string) string {
	switch {
	case strings.Contains(rawURL, "mime=audio%2Fwebm") || strings.Contains(rawURL, "mime=audio/webm"):
		return "webm"
	case strings.Contains(rawURL, "mime=audio%2Fmp4") || strings.Contains(rawURL, "mime=audio/mp4"):
		return "m4a"
	}
	return ""
}

// slugifyChannel produces a stable ASCII slug suitable for an
// item_id. Lowercase, non-alphanumerics → '_', trimmed. Empty for
// empty input.
func slugifyChannel(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
			out = append(out, c+('a'-'A'))
		case (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9'):
			out = append(out, c)
		default:
			if len(out) > 0 && out[len(out)-1] != '_' {
				out = append(out, '_')
			}
		}
	}
	for len(out) > 0 && out[len(out)-1] == '_' {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return "unknown"
	}
	return string(out)
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
