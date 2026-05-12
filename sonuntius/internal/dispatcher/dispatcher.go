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

// Dispatcher routes events to the HA client. When MAWS is set, the
// dispatcher uses MA's WebSocket directly for play_media, seek,
// transport, and volume — cutting out the HA REST + Python-integration
// hop. HA REST remains the fallback when MA WS is unavailable (e.g.,
// MA addon stopped) so the bridge still works in degraded mode.
//
// Concurrency model:
//
//   - Dispatch runs synchronously on the IPC reader goroutine. This
//     preserves strict event ordering across all event types — a
//     pause followed by play, or a play_media followed by seek,
//     always reaches MA in the order the sender issued them.
//   - The speed-critical commands (volume, mute, transport) use the
//     `WSClient.SendFireAndForget` path: write the WS frame, return
//     immediately, no per-call response wait. Each idempotent
//     command finishes in <1 ms even when MA's response would have
//     taken 20-50 ms.
//   - The cast-start path (play_media → seek → clear_queue) still
//     waits for responses — we use them to decide whether to fall
//     back to HA REST on auth-required / queue-not-found errors.
//     One round-trip per cast, not per press.
//
// v0.2.7's per-type worker goroutines were removed in v0.2.9 after
// they introduced play/pause flicker — out-of-order processing
// across types raced MA's state events back to the phone. FAF +
// synchronous dispatch keeps the speed wins and the ordering.
type Dispatcher struct {
	HA       *ha.Client
	EntityID string
	Logger   *slog.Logger

	// MA WS client (optional). When non-nil and Connected(), the
	// dispatcher routes commands directly to MA over its WebSocket
	// instead of HA REST. MAPlayerQueue is the MA-internal player_id
	// (which doubles as the queue_id for the player's own queue).
	MAWS          *ma.WSClient
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

// SetMAWS configures the long-lived MA WS client and the MA-internal
// player/queue ID for this dispatcher. queueID is MA's player_id; for
// single-player queues it doubles as the queue_id.
func (d *Dispatcher) SetMAWS(client *ma.WSClient, queueID string) {
	d.MAWS = client
	d.MAPlayerQueue = queueID
}

// maWSReady reports whether we can route commands through the MA WS.
func (d *Dispatcher) maWSReady() bool {
	return d.MAWS != nil && d.MAPlayerQueue != "" && d.MAWS.Connected()
}

// Ready reports whether the dispatcher has a target entity configured.
// When false, Dispatch logs and drops events instead of calling HA.
func (d *Dispatcher) Ready() bool {
	return d.EntityID != ""
}

// Dispatch routes one event synchronously to the type-specific
// handler. Synchronous keeps cross-type ordering intact (a pause then
// play always reaches MA in that order, a play_media then seek always
// in that order). The speed comes from the underlying WS path —
// idempotent transport / volume / mute commands use
// `WSClient.SendFireAndForget` which returns in <1 ms; only the
// cast-start path (play_media, seek, clear_queue) waits for a
// response, and only once per cast.
func (d *Dispatcher) Dispatch(ctx context.Context, ev events.Event) {
	if !d.Ready() {
		d.Logger.Warn("dispatcher: idle (ma_player_id unset)", "type", ev.EventType())
		return
	}
	switch e := ev.(type) {
	case *events.PlayIntent:
		d.dispatchPlay(ctx, e)
	case *events.QueueAddIntent:
		d.dispatchQueueAdd(ctx, e)
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
	if p.Provider == "url" && d.maWSReady() {
		if err := d.playViaMAWS(ctx, uri, p); err == nil {
			return
		} else if errors.Is(err, ma.ErrAuthRequired) {
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
			d.Logger.Warn("dispatcher: MA WS play path failed, falling back to HA REST",
				"err", err)
		}
	}
	// Fallback: HA REST. No rich metadata, no immediate seek, but
	// audio plays. Used when MA WS is unavailable or auth is missing.
	extra := metadataExtra(p.Metadata)
	if err := d.HA.PlayMedia(ctx, d.EntityID, uri, "music", extra); err != nil {
		d.Logger.Error("dispatcher: play_media failed", "err", err)
		return
	}
	if p.StartPosition > 0.5 {
		go func(pos float64) {
			select {
			case <-time.After(500 * time.Millisecond):
			case <-ctx.Done():
				return
			}
			if err := d.HA.MediaPlayerCommand(ctx, d.EntityID, "media_seek",
				map[string]any{"seek_position": pos}); err != nil {
				d.Logger.Warn("dispatcher: HA-REST post-play seek failed",
					"position", pos, "err", err)
				return
			}
			d.Logger.Info("dispatcher: HA-REST post-play seek issued",
				"position", pos, "entity", d.EntityID)
		}(p.StartPosition)
	}
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
	item := d.buildMediaItem(uri, p.TrackID, p.Metadata)
	// Clear-then-play-then-seek over a single MA WS connection. MA
	// processes commands in receipt order on a single connection, so
	// the seek lands on our new queue item — not the prior cast's or
	// some stale library track. No 500 ms HA-REST delay needed: WS
	// commands are typically acknowledged in <50 ms each, and the
	// player handler waits for the queue item to load before applying
	// transport commands.
	log := d.Logger.With("path", "ma-ws")
	if err := d.MAWS.ClearQueue(ctx, d.MAPlayerQueue); err != nil {
		log.Warn("dispatcher: queue clear failed (continuing)", "err", err)
	}
	log.Info("ma: PlayMediaItem", "queue_id", d.MAPlayerQueue,
		"name", item.Name, "provider", item.Provider,
		"start_position", p.StartPosition)
	if err := d.MAWS.PlayQueueMedia(ctx, d.MAPlayerQueue, item, "play"); err != nil {
		return err
	}
	if p.StartPosition > 0.5 {
		if err := d.MAWS.Seek(ctx, d.MAPlayerQueue, p.StartPosition); err != nil {
			log.Warn("dispatcher: post-play seek failed", "position", p.StartPosition, "err", err)
		} else {
			log.Info("dispatcher: post-play seek issued (WS)",
				"position", p.StartPosition, "queue_id", d.MAPlayerQueue)
		}
	}
	return nil
}

// dispatchQueueAdd appends a pre-resolved upcoming track to MA's queue.
// Called by the yt-cast adapter after a successful DoPlay so MA auto-
// advances to YouTube's "next" instead of MA's library autoplay when
// the current track ends. Skipped silently when MA WS isn't available
// — HA REST has no equivalent, and the consequence is just no preload,
// not a broken playback.
func (d *Dispatcher) dispatchQueueAdd(ctx context.Context, p *events.QueueAddIntent) {
	if !d.maWSReady() {
		d.Logger.Debug("dispatcher: queue add dropped (MA WS not ready)",
			"track_id", p.TrackID)
		return
	}
	uri := p.URL
	if uri == "" {
		d.Logger.Debug("dispatcher: queue add dropped (empty URL)",
			"track_id", p.TrackID)
		return
	}
	item := d.buildMediaItem(uri, p.TrackID, p.Metadata)
	log := d.Logger.With("path", "ma-ws-queue-add")
	log.Info("ma: AddToQueueMedia",
		"queue_id", d.MAPlayerQueue,
		"name", item.Name,
		"track_id", p.TrackID)
	if err := d.MAWS.AddToQueueMedia(ctx, d.MAPlayerQueue, item); err != nil {
		log.Warn("dispatcher: queue add failed", "err", err)
	}
}

// buildMediaItem assembles a MA-shaped MediaItem from a stream URL +
// metadata map. Extracted so the play and queue-add paths share the
// same construction logic.
func (d *Dispatcher) buildMediaItem(uri, trackID string, meta map[string]any) ma.MediaItem {
	title, _ := meta["title"].(string)
	channel, _ := meta["channel"].(string)
	thumb, _ := meta["thumbnail"].(string)
	durationVal, _ := meta["duration"].(float64)
	_ = trackID // reserved for future routing decisions

	contentType := guessAudioContentType(uri)
	mapping := ma.MediaItemProviderMapping{
		ItemID:           uri,
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
		ItemID:           uri,
		Provider:         "builtin",
		Name:             title,
		Version:          "",
		MediaType:        "track",
		URI:              "builtin://track/" + uri,
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
	return item
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
	// MA WS path — preferred when the long-lived connection is up.
	if d.maWSReady() {
		var err error
		switch t.Command {
		case "play":
			err = d.MAWS.PlayerPlay(ctx, d.MAPlayerQueue)
		case "pause":
			err = d.MAWS.PlayerPause(ctx, d.MAPlayerQueue)
		case "stop":
			err = d.MAWS.PlayerStop(ctx, d.MAPlayerQueue)
		case "next":
			err = d.MAWS.PlayerNext(ctx, d.MAPlayerQueue)
		case "previous":
			err = d.MAWS.PlayerPrevious(ctx, d.MAPlayerQueue)
		case "seek":
			if t.Position != nil {
				err = d.MAWS.Seek(ctx, d.MAPlayerQueue, *t.Position)
			}
		case "clear_queue":
			// No HA equivalent — clear is MA-WS-only. Quietly skip if
			// WS isn't connected (we're probably mid-shutdown).
			err = d.MAWS.ClearQueue(ctx, d.MAPlayerQueue)
		default:
			d.Logger.Warn("dispatcher: unknown transport command", "command", t.Command)
			return
		}
		if err == nil {
			return
		}
		d.Logger.Warn("dispatcher: MA WS transport failed, falling back to HA REST",
			"command", t.Command, "err", err)
	}
	if t.Command == "clear_queue" {
		// No HA REST equivalent. Best-effort only.
		return
	}
	// Fallback: HA REST.
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
	// MA WS path — preferred when the long-lived connection is up.
	if d.maWSReady() {
		var firstErr error
		if v.Muted != nil {
			if err := d.MAWS.SetMute(ctx, d.MAPlayerQueue, *v.Muted); err != nil {
				firstErr = err
			}
		}
		if v.Level != nil {
			// MA expects integer 0-100; we receive 0.0-1.0.
			level := int(*v.Level*100 + 0.5)
			if err := d.MAWS.SetVolume(ctx, d.MAPlayerQueue, level); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		if firstErr == nil {
			return
		}
		d.Logger.Warn("dispatcher: MA WS volume failed, falling back to HA REST",
			"err", firstErr)
	}
	// Fallback: HA REST.
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
