// Package ma is a Music Assistant WebSocket client that subscribes to
// player events and broadcasts derived PlayerState frames over the IPC
// broker.
//
// The MA WebSocket protocol is:
//
//  1. Connect to ws://<host>:8095/ws (port + path are configurable).
//  2. The server sends a server-info frame immediately:
//     {schema_version, min_supported_schema_version, server_id,
//     server_version, base_url}.
//  3. For schema >= 28 the client must send an auth command:
//     {message_id: <uuid>, command: "auth", args: {token: <token>}}.
//     For local addon-to-addon access on older MA servers auth is
//     optional — we skip the auth handshake when schema < 28 OR when
//     no token is configured.
//  4. Subsequent frames are either command-results
//     ({message_id, result}) or event pushes
//     ({event: "<name>", data: {...}, object_id: "<optional>"}).
//
// We subscribe to all events implicitly — MA pushes player_updated /
// player_queue_updated / player_queue_time_updated frames to every
// connected client by default. We filter for events that touch our
// configured player and translate them to events.PlayerState frames
// that ride the existing IPC bus.
//
// MA's WS protocol is not in a public spec at the time of writing; this
// implementation is calibrated against music-assistant/client (Python)
// and tracked as TODO(yt-cast Phase 6a refinement) where exact field
// shapes are uncertain. Untranslatable frames are logged at debug and
// dropped — falling back to the HA core WS path remains the safety net.
package ma

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"golang.org/x/net/websocket"

	"github.com/shobuprime/sonuntius/internal/events"
)

// ErrAuthRequired is returned when the MA server requires the client to
// authenticate before issuing a command (schema_version >= 28) but no
// `ma_token` was configured (or the configured token was rejected). The
// dispatcher uses errors.Is to detect this specific case and surface a
// one-time actionable warning to the user, since the play path silently
// falls back to HA REST otherwise.
var ErrAuthRequired = errors.New("ma: authentication required (set ma_token in addon options — create one at MA Settings → Security → API Tokens)")

const (
	// DefaultPort is the TCP port the music_assistant addon exposes its
	// WebSocket on. Used by URLFromHost when the host part is auto-
	// discovered via the Supervisor /addons listing.
	DefaultPort = 8095
	// DefaultPath is the WebSocket path on the MA server.
	DefaultPath = "/ws"

	reconnectFloor     = 2 * time.Second
	reconnectCeil      = 60 * time.Second
	authSchemaVersion  = 28
	authCommand        = "auth"
	connectReadTimeout = 5 * time.Second
)

// URLFromHost builds a ws:// URL from a bare hostname using DefaultPort
// and DefaultPath. Returns "" when host is empty.
func URLFromHost(host string) string {
	if host == "" {
		return ""
	}
	return fmt.Sprintf("ws://%s:%d%s", host, DefaultPort, DefaultPath)
}

// Broadcaster is the subset of *ipc.Server we need.
type Broadcaster interface {
	Broadcast(events.Event)
}

// Watcher maintains a persistent MA WS subscription with exponential
// backoff. Run blocks until ctx is cancelled.
type Watcher struct {
	URL        string
	Token      string
	PlayerID   string // optional — when set, filters events to this player
	IPC        Broadcaster
	Logger     *slog.Logger

	connected atomic.Bool
}

// NewWatcher constructs a Watcher. url is the full WebSocket URL
// (ws://host:port/path). PlayerID is the user-configured MA player_id
// to filter events on; pass "" to broadcast all player updates.
func NewWatcher(url, token, playerID string, ipc Broadcaster, logger *slog.Logger) *Watcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Watcher{
		URL:      url,
		Token:    token,
		PlayerID: playerID,
		IPC:      ipc,
		Logger:   logger,
	}
}

// Connected reports whether the watcher currently has an active WS.
func (w *Watcher) Connected() bool {
	return w.connected.Load()
}

// TryConnect attempts a single connect + server-info handshake and
// returns nil on success. Used by the lead watcher to decide whether MA
// direct is reachable before committing to it for the long-lived loop.
// The connection is closed before return.
func (w *Watcher) TryConnect(ctx context.Context) error {
	cfg, err := websocket.NewConfig(w.URL, origin())
	if err != nil {
		return err
	}
	conn, err := websocket.DialConfig(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = readServerInfo(conn)
	return err
}

// Run is the reconnect-with-backoff outer loop.
func (w *Watcher) Run(ctx context.Context) {
	if w.URL == "" {
		w.Logger.Info("ma watcher idle (no URL configured)")
		<-ctx.Done()
		return
	}
	backoff := reconnectFloor
	for {
		err := w.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			w.Logger.Warn("ma watcher disconnected", "err", err, "retry_in", backoff)
		} else {
			w.Logger.Info("ma watcher cycle ended cleanly", "retry_in", backoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < reconnectCeil {
			backoff *= 2
			if backoff > reconnectCeil {
				backoff = reconnectCeil
			}
		}
	}
}

func (w *Watcher) runOnce(ctx context.Context) error {
	cfg, err := websocket.NewConfig(w.URL, origin())
	if err != nil {
		return err
	}
	conn, err := websocket.DialConfig(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()
	w.connected.Store(true)
	defer w.connected.Store(false)

	info, err := readServerInfo(conn)
	if err != nil {
		return fmt.Errorf("ma: server info: %w", err)
	}
	w.Logger.Info("ma: connected",
		"server_id", info.ServerID,
		"server_version", info.ServerVersion,
		"schema_version", info.SchemaVersion,
		"base_url", info.BaseURL,
	)

	if info.SchemaVersion >= authSchemaVersion && w.Token != "" {
		if err := w.authenticate(conn); err != nil {
			return fmt.Errorf("ma: auth: %w", err)
		}
		w.Logger.Info("ma: authenticated")
	} else if info.SchemaVersion >= authSchemaVersion {
		w.Logger.Warn("ma: schema requires auth but ma.token is empty — server may close the connection")
	}

	// Close the connection when ctx is cancelled so the read loop exits.
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	for {
		var raw []byte
		if err := websocket.Message.Receive(conn, &raw); err != nil {
			return err
		}
		w.handleFrame(raw)
	}
}

// serverInfo mirrors the ServerInfoMessage MA sends on connect. We
// only decode the fields we use; unknown fields are ignored.
type serverInfo struct {
	SchemaVersion            int    `json:"schema_version"`
	MinSupportedSchemaVersion int   `json:"min_supported_schema_version"`
	ServerID                 string `json:"server_id"`
	ServerVersion            string `json:"server_version"`
	BaseURL                  string `json:"base_url"`
}

func readServerInfo(conn *websocket.Conn) (*serverInfo, error) {
	if err := conn.SetReadDeadline(time.Now().Add(connectReadTimeout)); err != nil {
		return nil, err
	}
	var raw []byte
	if err := websocket.Message.Receive(conn, &raw); err != nil {
		return nil, err
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return nil, err
	}
	var info serverInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return nil, fmt.Errorf("decode: %w (frame=%s)", err, truncate(string(raw), 200))
	}
	if info.ServerID == "" && info.SchemaVersion == 0 {
		return nil, fmt.Errorf("unexpected first frame: %s", truncate(string(raw), 200))
	}
	return &info, nil
}

func (w *Watcher) authenticate(conn *websocket.Conn) error {
	msgID, err := randHex(16)
	if err != nil {
		return err
	}
	body := map[string]any{
		"message_id": msgID,
		"command":    authCommand,
		"args":       map[string]any{"token": w.Token},
	}
	if err := websocket.JSON.Send(conn, body); err != nil {
		return err
	}
	// Read the auth response.
	if err := conn.SetReadDeadline(time.Now().Add(connectReadTimeout)); err != nil {
		return err
	}
	defer conn.SetReadDeadline(time.Time{})
	var resp map[string]any
	if err := websocket.JSON.Receive(conn, &resp); err != nil {
		return err
	}
	if errCode, ok := resp["error_code"]; ok && errCode != nil {
		return fmt.Errorf("auth rejected: %v", resp)
	}
	return nil
}

// frameEnvelope is the loose-superset shape used to discriminate which
// kind of frame we received. The MA wire protocol uses one of:
//
//	{event: <name>, data: {...}, object_id: <id>}   ← push event
//	{message_id: <id>, result: <any>}               ← command success
//	{message_id: <id>, error_code: <code>, ...}     ← command error
type frameEnvelope struct {
	Event     string          `json:"event,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	ObjectID  string          `json:"object_id,omitempty"`
	MessageID string          `json:"message_id,omitempty"`
	ErrorCode any             `json:"error_code,omitempty"`
}

func (w *Watcher) handleFrame(raw []byte) {
	var env frameEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		w.Logger.Debug("ma: malformed frame", "err", err)
		return
	}
	if env.Event == "" {
		// Command response — uninteresting for now.
		return
	}
	w.translateEvent(env)
}

// PlayerStateFromPlayerEvent decodes the `data` field of an MA
// `player_updated` (or `player_added`) event. The Player object's
// `state` is the speaker-on/off state; the player-level title/artist
// info is best-effort here because MA often emits player_updated
// events that omit current_item entirely. Callers should merge,
// not replace, when applying this to cached state.
func PlayerStateFromPlayerEvent(raw json.RawMessage) *events.PlayerState {
	var p playerUpdate
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil
	}
	ps := &events.PlayerState{State: p.State}
	if p.Volume > 0 {
		v := p.Volume
		ps.Volume = &v
	}
	if p.Muted {
		m := p.Muted
		ps.Muted = &m
	}
	if p.CurrentItem != nil {
		ps.Title = p.CurrentItem.MediaItem.Name
		if len(p.CurrentItem.MediaItem.Artists) > 0 {
			ps.Artist = p.CurrentItem.MediaItem.Artists[0].Name
		}
		ps.TrackID = p.CurrentItem.MediaItem.ItemID
		if p.CurrentItem.MediaItem.Duration > 0 {
			d := p.CurrentItem.MediaItem.Duration
			ps.Duration = &d
		}
		if p.CurrentItem.StreamDetails.Position > 0 {
			pos := p.CurrentItem.StreamDetails.Position
			ps.Position = &pos
		}
	}
	return ps
}

// PlayerStateFromMAEvent is the back-compat alias for the player_updated
// decoder. New code should prefer the named variants
// (PlayerStateFromPlayerEvent / PlayerStateFromQueueEvent).
func PlayerStateFromMAEvent(raw json.RawMessage) *events.PlayerState {
	return PlayerStateFromPlayerEvent(raw)
}

// queueUpdate mirrors the subset of MA's PlayerQueue payload we
// translate. queue_updated and queue_time_updated event payloads
// both decode against this shape.
type queueUpdate struct {
	QueueID      string      `json:"queue_id"`
	Active       bool        `json:"active"`
	State        string      `json:"state"` // idle / playing / paused / buffering
	ElapsedTime  float64     `json:"elapsed_time"`
	CurrentIndex *int        `json:"current_index"`
	CurrentItem  *struct {
		QueueItemID string `json:"queue_item_id"`
		Name        string `json:"name"`
		Duration    float64 `json:"duration"`
		MediaItem   struct {
			Name    string `json:"name"`
			ItemID  string `json:"item_id"`
			URI     string `json:"uri"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
			Duration float64 `json:"duration"`
		} `json:"media_item"`
	} `json:"current_item,omitempty"`
	Items int `json:"items"`
}

// PlayerStateFromQueueEvent decodes the `data` field of an MA
// `queue_updated` or `queue_time_updated` event. The PlayerQueue's
// `state` is the authoritative source for pause/playing/buffering
// transitions — MA's player_updated emissions don't always carry
// that field (and when they do it can lag the queue state by a
// frame or two).
func PlayerStateFromQueueEvent(raw json.RawMessage) *events.PlayerState {
	var q queueUpdate
	if err := json.Unmarshal(raw, &q); err != nil {
		return nil
	}
	state := q.State
	// MA's Universal Player + Sendspin path reports "idle" both for
	// "user paused" and "no longer playing anything". Disambiguate
	// here: if the queue still considers itself active and has a
	// current item, treat idle as paused — semantically that's what
	// the cast sender should display. A truly stopped queue clears
	// active or current_item.
	if state == "idle" && q.Active && q.CurrentItem != nil {
		state = "paused"
	}
	ps := &events.PlayerState{State: state}
	if q.ElapsedTime > 0 {
		pos := q.ElapsedTime
		ps.Position = &pos
	}
	if q.CurrentItem != nil {
		// Prefer the embedded media_item's fields when present,
		// otherwise fall back to QueueItem-level name/duration.
		if name := q.CurrentItem.MediaItem.Name; name != "" {
			ps.Title = name
		} else if q.CurrentItem.Name != "" {
			ps.Title = q.CurrentItem.Name
		}
		if len(q.CurrentItem.MediaItem.Artists) > 0 {
			ps.Artist = q.CurrentItem.MediaItem.Artists[0].Name
		}
		if q.CurrentItem.MediaItem.ItemID != "" {
			ps.TrackID = q.CurrentItem.MediaItem.ItemID
		}
		duration := q.CurrentItem.MediaItem.Duration
		if duration == 0 {
			duration = q.CurrentItem.Duration
		}
		if duration > 0 {
			ps.Duration = &duration
		}
	}
	return ps
}

// playerUpdate mirrors the subset of MA's player payload we care about
// for state translation. Unknown fields are ignored.
//
// TODO(yt-cast Phase 6a refinement): confirm exact field names against
// running MA. Today's best-guess uses snake_case matching MA's Python
// dataclasses.
type playerUpdate struct {
	PlayerID      string  `json:"player_id"`
	State         string  `json:"state"`
	Volume        float64 `json:"volume_level"`
	Muted         bool    `json:"volume_muted"`
	CurrentItem   *struct {
		MediaItem struct {
			Name     string `json:"name"`
			Artists  []struct {
				Name string `json:"name"`
			} `json:"artists"`
			ItemID string `json:"item_id"`
			URI    string `json:"uri"`
			Duration float64 `json:"duration"`
		} `json:"media_item"`
		StreamDetails struct {
			Position float64 `json:"position"`
		} `json:"stream_details"`
	} `json:"current_item,omitempty"`
}

func (w *Watcher) translateEvent(env frameEnvelope) {
	switch env.Event {
	case "player_updated", "player_added", "player_queue_time_updated":
		// fall through
	default:
		w.Logger.Debug("ma: ignoring event", "event", env.Event, "object_id", env.ObjectID)
		return
	}
	if w.PlayerID != "" && env.ObjectID != "" && env.ObjectID != w.PlayerID {
		return
	}
	var p playerUpdate
	if err := json.Unmarshal(env.Data, &p); err != nil {
		w.Logger.Debug("ma: player payload decode failed", "err", err, "event", env.Event)
		return
	}
	if w.PlayerID != "" && p.PlayerID != "" && p.PlayerID != w.PlayerID {
		return
	}
	ps := &events.PlayerState{State: p.State}
	if p.Volume > 0 {
		v := p.Volume
		ps.Volume = &v
	}
	if p.Muted {
		m := p.Muted
		ps.Muted = &m
	}
	if p.CurrentItem != nil {
		ps.Title = p.CurrentItem.MediaItem.Name
		if len(p.CurrentItem.MediaItem.Artists) > 0 {
			ps.Artist = p.CurrentItem.MediaItem.Artists[0].Name
		}
		ps.TrackID = p.CurrentItem.MediaItem.ItemID
		if p.CurrentItem.MediaItem.Duration > 0 {
			d := p.CurrentItem.MediaItem.Duration
			ps.Duration = &d
		}
		if p.CurrentItem.StreamDetails.Position > 0 {
			pos := p.CurrentItem.StreamDetails.Position
			ps.Position = &pos
		}
	}
	w.Logger.Debug("ma: broadcasting PlayerState",
		"state", ps.State, "title", ps.Title, "track_id", ps.TrackID)
	w.IPC.Broadcast(ps)
}

// origin returns the Origin header value for the WS handshake. MA's WS
// server accepts any origin, but the websocket library requires the
// value to be non-empty.
func origin() string {
	return "http://sonuntius"
}

func randHex(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// MediaItemImage is one entry in MediaItemMetadata.Images.
type MediaItemImage struct {
	Type               string `json:"type"`                          // "thumb", "fanart", "logo"
	Path               string `json:"path"`                          // URL
	Provider           string `json:"provider"`                      // "url" for remote
	RemotelyAccessible bool   `json:"remotely_accessible,omitempty"` // true for public HTTP URLs
}

// MediaItemMetadata is the metadata sub-object of a MediaItem.
type MediaItemMetadata struct {
	Images []MediaItemImage `json:"images,omitempty"`
}

// MediaItemAudioFormat describes the wire audio format of a provider
// mapping. We only fill the fields MA looks at on the playback path.
type MediaItemAudioFormat struct {
	ContentType string `json:"content_type,omitempty"` // "webm", "m4a", "mp3"
	SampleRate  int    `json:"sample_rate,omitempty"`
	BitDepth    int    `json:"bit_depth,omitempty"`
	Channels    int    `json:"channels,omitempty"`
}

// MediaItemProviderMapping is a single entry under provider_mappings.
// The critical field is `url` — when set, MA's streams pipeline uses
// it directly and does NOT call the provider's parse_item / probe
// flow. `item_id` here must be a synthetic ID (not the HTTP URL),
// otherwise the builtin provider's parse_item re-probes the URL with
// ffmpeg and clobbers our name/artist.
type MediaItemProviderMapping struct {
	ItemID           string                `json:"item_id"`
	ProviderDomain   string                `json:"provider_domain"`
	ProviderInstance string                `json:"provider_instance"`
	Available        bool                  `json:"available"`
	URL              string                `json:"url,omitempty"`
	AudioFormat      *MediaItemAudioFormat `json:"audio_format,omitempty"`
}

// MediaItemArtist is a full Artist dict for use under MediaItem.Artists.
// MA's deserializer drops a list of strings silently — the artist
// name surfaces in the UI only when each entry is a proper dict.
type MediaItemArtist struct {
	ItemID    string `json:"item_id"`
	Provider  string `json:"provider"`
	Name      string `json:"name"`
	MediaType string `json:"media_type"` // "artist"
	Available bool   `json:"available"`
}

// MediaItem mirrors the subset of MA's Track schema we populate when
// dispatching a play_media via the native WS API. Bypassing HA's
// media_player.play_media wrapper preserves rich metadata that would
// otherwise be stripped on the way through the HA integration's
// `extra` field.
//
// CRITICAL fields for skipping MA's re-resolution of the URL (which
// would clobber the title/artist with ffmpeg-probed values from the
// signed googlevideo URL):
//
//   - ItemID is a synthetic, stable id (e.g. "yt_<videoId>"), NOT the
//     HTTP URL. A URL-shaped item_id triggers builtin.parse_item which
//     ffmpeg-probes and overwrites the metadata.
//   - ProviderMappings[0].URL holds the resolved stream URL. MA's
//     stream pipeline prefers this when set.
//   - Artists must be full dicts (not strings) — MA's mashumaro
//     deserializer silently drops list-of-string artists.
//   - Metadata.Images replaces the older flat `image` field.
//   - Available + IsPlayable must both be true; the play_media loop
//     filters items by `available`.
type MediaItem struct {
	ItemID           string                     `json:"item_id"`
	Provider         string                     `json:"provider"`
	Name             string                     `json:"name"`
	Version          string                     `json:"version"`
	MediaType        string                     `json:"media_type"` // "track"
	URI              string                     `json:"uri,omitempty"`
	Available        bool                       `json:"available"`
	IsPlayable       bool                       `json:"is_playable"`
	Favorite         bool                       `json:"favorite"`
	Duration         int                        `json:"duration,omitempty"`
	Artists          []MediaItemArtist          `json:"artists,omitempty"`
	Metadata         MediaItemMetadata          `json:"metadata"`
	ProviderMappings []MediaItemProviderMapping `json:"provider_mappings"`
	ExternalIDs      []any                      `json:"external_ids"`
}

// PlayMediaItem sends `player_queues/play_media` to MA's WebSocket
// with a fully-formed MediaItem. queueID is MA's internal player_id
// (NOT the HA entity_id). Returns nil on success.
//
// We open a short-lived connection per call rather than multiplexing
// on the long-running Watcher's read loop — MA's WS is on the local
// addon network, so the dial + auth handshake is sub-millisecond,
// and the simplicity of one-connection-per-call avoids us building a
// request/response correlator on top of the event-stream read loop.
func PlayMediaItem(ctx context.Context, url, token, queueID string, item MediaItem, log *slog.Logger) error {
	if log == nil {
		log = slog.Default()
	}
	cfg, err := websocket.NewConfig(url, origin())
	if err != nil {
		return fmt.Errorf("ma: ws config: %w", err)
	}
	conn, err := websocket.DialConfig(cfg)
	if err != nil {
		return fmt.Errorf("ma: ws dial: %w", err)
	}
	defer conn.Close()

	info, err := readServerInfo(conn)
	if err != nil {
		return fmt.Errorf("ma: server info: %w", err)
	}
	if info.SchemaVersion >= authSchemaVersion && token == "" {
		// MA schema 28+ requires auth for every command. Skip the
		// connection attempt entirely and surface a typed error so the
		// dispatcher can log actionable guidance once. Falling through
		// would only result in MA returning error_code=20 a few ms later.
		return fmt.Errorf("%w (server schema=%d, server_id=%s)",
			ErrAuthRequired, info.SchemaVersion, info.ServerID)
	}
	if info.SchemaVersion >= authSchemaVersion && token != "" {
		msgID, mErr := randHex(16)
		if mErr != nil {
			return fmt.Errorf("ma: msg id: %w", mErr)
		}
		if err := websocket.JSON.Send(conn, map[string]any{
			"message_id": msgID,
			"command":    authCommand,
			"args":       map[string]any{"token": token},
		}); err != nil {
			return fmt.Errorf("ma: auth send: %w", err)
		}
		var authResp map[string]any
		if err := conn.SetReadDeadline(time.Now().Add(connectReadTimeout)); err != nil {
			return err
		}
		if err := websocket.JSON.Receive(conn, &authResp); err != nil {
			return fmt.Errorf("ma: auth recv: %w", err)
		}
		_ = conn.SetReadDeadline(time.Time{})
		if errCode, ok := authResp["error_code"]; ok && errCode != nil {
			return fmt.Errorf("ma: auth rejected: %v", authResp)
		}
	}

	msgID, err := randHex(16)
	if err != nil {
		return fmt.Errorf("ma: msg id: %w", err)
	}
	body := map[string]any{
		"message_id": msgID,
		"command":    "player_queues/play_media",
		"args": map[string]any{
			"queue_id": queueID,
			"media":    []MediaItem{item},
			// "play" replaces the current item but keeps any queue
			// position state MA had. The earlier "replace" caused a
			// regression where casting at a non-zero start position
			// (user scrubbed to 7:28 then cast) ignored the
			// subsequent media_seek — MA's full queue reset raced
			// the seek and dropped it. To keep the "clean queue"
			// behavior we explicitly call `player_queues/clear`
			// before this command (see ClearQueue) so the queue is
			// empty when our item lands.
			"option": "play",
		},
	}
	log.Info("ma: PlayMediaItem", "queue_id", queueID,
		"item_id_short", truncate(item.ItemID, 80),
		"name", item.Name, "provider", item.Provider)
	if err := websocket.JSON.Send(conn, body); err != nil {
		return fmt.Errorf("ma: command send: %w", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(15 * time.Second)); err != nil {
		return err
	}
	defer conn.SetReadDeadline(time.Time{})
	// MA may push event frames before responding to our command. Loop
	// until we see a frame whose message_id matches our command.
	for {
		var resp map[string]any
		if err := websocket.JSON.Receive(conn, &resp); err != nil {
			return fmt.Errorf("ma: command recv: %w", err)
		}
		if respID, ok := resp["message_id"].(string); ok && respID == msgID {
			if ec, ok := resp["error_code"]; ok && ec != nil {
				details, _ := resp["details"].(string)
				// MA error_code 20 is the documented "Authentication
				// required" code. Wrap with our sentinel so the
				// dispatcher can detect it via errors.Is.
				if isAuthRequiredCode(ec) {
					return fmt.Errorf("%w: server reply: %v %s",
						ErrAuthRequired, ec, details)
				}
				return fmt.Errorf("ma: play_media error: %v %s", ec, details)
			}
			return nil
		}
		// non-matching frame — most likely a player_updated event
		// MA pushed in response to our command. Ignore and keep
		// reading.
	}
}

// PlayerInfo is the subset of MA's Player schema we use for queue-id
// discovery. Unknown fields are ignored.
type PlayerInfo struct {
	PlayerID    string `json:"player_id"`
	Provider    string `json:"provider"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Available   bool   `json:"available"`
	Type        string `json:"type"`
}

// ListPlayers opens a short-lived WS connection, authenticates if the
// server schema requires it, sends `players/all`, and returns the
// decoded player list. Used by ma-bridge to discover the correct
// queue_id (MA's internal player_id) at startup — the HA entity_id is
// not, in general, identical to MA's internal id, so any auto-derived
// queue_id can be wrong.
func ListPlayers(ctx context.Context, url, token string, log *slog.Logger) ([]PlayerInfo, error) {
	if log == nil {
		log = slog.Default()
	}
	cfg, err := websocket.NewConfig(url, origin())
	if err != nil {
		return nil, fmt.Errorf("ma: ws config: %w", err)
	}
	conn, err := websocket.DialConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("ma: ws dial: %w", err)
	}
	defer conn.Close()

	info, err := readServerInfo(conn)
	if err != nil {
		return nil, fmt.Errorf("ma: server info: %w", err)
	}
	if info.SchemaVersion >= authSchemaVersion && token == "" {
		return nil, fmt.Errorf("%w (server schema=%d, server_id=%s)",
			ErrAuthRequired, info.SchemaVersion, info.ServerID)
	}
	if info.SchemaVersion >= authSchemaVersion && token != "" {
		msgID, mErr := randHex(16)
		if mErr != nil {
			return nil, fmt.Errorf("ma: msg id: %w", mErr)
		}
		if err := websocket.JSON.Send(conn, map[string]any{
			"message_id": msgID,
			"command":    authCommand,
			"args":       map[string]any{"token": token},
		}); err != nil {
			return nil, fmt.Errorf("ma: auth send: %w", err)
		}
		if err := conn.SetReadDeadline(time.Now().Add(connectReadTimeout)); err != nil {
			return nil, err
		}
		var authResp map[string]any
		if err := websocket.JSON.Receive(conn, &authResp); err != nil {
			return nil, fmt.Errorf("ma: auth recv: %w", err)
		}
		_ = conn.SetReadDeadline(time.Time{})
		if errCode, ok := authResp["error_code"]; ok && errCode != nil {
			if isAuthRequiredCode(errCode) {
				return nil, fmt.Errorf("%w: server reply: %v", ErrAuthRequired, authResp)
			}
			return nil, fmt.Errorf("ma: auth rejected: %v", authResp)
		}
	}

	msgID, err := randHex(16)
	if err != nil {
		return nil, fmt.Errorf("ma: msg id: %w", err)
	}
	if err := websocket.JSON.Send(conn, map[string]any{
		"message_id": msgID,
		"command":    "players/all",
		"args":       map[string]any{},
	}); err != nil {
		return nil, fmt.Errorf("ma: players/all send: %w", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return nil, err
	}
	defer conn.SetReadDeadline(time.Time{})
	for {
		var resp map[string]any
		if err := websocket.JSON.Receive(conn, &resp); err != nil {
			return nil, fmt.Errorf("ma: players/all recv: %w", err)
		}
		respID, _ := resp["message_id"].(string)
		if respID != msgID {
			// event push (player_updated etc) — ignore and keep reading
			continue
		}
		if ec, ok := resp["error_code"]; ok && ec != nil {
			details, _ := resp["details"].(string)
			return nil, fmt.Errorf("ma: players/all error: %v %s", ec, details)
		}
		// Re-marshal the result field so we can decode through the typed
		// PlayerInfo struct without hand-walking the map[string]any tree.
		result, ok := resp["result"]
		if !ok {
			return nil, fmt.Errorf("ma: players/all: response missing result")
		}
		raw, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("ma: players/all: re-marshal: %w", err)
		}
		var out []PlayerInfo
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("ma: players/all: decode: %w", err)
		}
		return out, nil
	}
}

// MatchPlayer finds the PlayerInfo whose player_id, display_name, or
// name best matches the supplied HA entity slug (the part after
// `media_player.`). Matching is best-effort:
//
//  1. Exact match on player_id.
//  2. Exact match after stripping a trailing `_N` (where N is 1-3
//     digits — HA appends this when a slug collides).
//  3. Slug-equivalent match on display_name (lowercase, ASCII only,
//     non-alphanumeric → '_').
//  4. Substring containment (player_id contains entity slug, or vice
//     versa) — covers cases where MA uses a `<provider>_<id>` form.
//
// Returns the matched PlayerInfo and the matching rule used, or an
// empty PlayerInfo and "" when nothing matched.
func MatchPlayer(players []PlayerInfo, entityID string) (PlayerInfo, string) {
	slug := entityID
	const prefix = "media_player."
	if len(slug) > len(prefix) && slug[:len(prefix)] == prefix {
		slug = slug[len(prefix):]
	}
	stripped := DerivePlayerID(entityID)

	for _, p := range players {
		if p.PlayerID == slug {
			return p, "exact_player_id"
		}
	}
	for _, p := range players {
		if p.PlayerID == stripped {
			return p, "stripped_player_id"
		}
	}
	for _, p := range players {
		if slugify(p.DisplayName) == slug || slugify(p.Name) == slug {
			return p, "display_name_slug"
		}
		if slugify(p.DisplayName) == stripped || slugify(p.Name) == stripped {
			return p, "display_name_slug_stripped"
		}
	}
	for _, p := range players {
		if p.PlayerID == "" {
			continue
		}
		if containsFold(p.PlayerID, stripped) || containsFold(stripped, p.PlayerID) {
			return p, "substring"
		}
	}
	return PlayerInfo{}, ""
}

func slugify(s string) string {
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
	return string(out)
}

func containsFold(haystack, needle string) bool {
	if needle == "" {
		return false
	}
	h := slugify(haystack)
	n := slugify(needle)
	if n == "" {
		return false
	}
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return true
		}
	}
	return false
}

// ClearQueue sends `player_queues/clear` for the given queue. Used
// before PlayMediaItem so a fresh cast is the only item on the
// queue — without this, leftover tracks (MA library autoplay,
// previous casts, favourites) auto-advance when our cast ends.
//
// Errors are returned but ClearQueue is best-effort: a failure here
// shouldn't block the play_media that follows; the caller logs and
// continues.
func ClearQueue(ctx context.Context, url, token, queueID string, log *slog.Logger) error {
	if log == nil {
		log = slog.Default()
	}
	cfg, err := websocket.NewConfig(url, origin())
	if err != nil {
		return fmt.Errorf("ma: ws config: %w", err)
	}
	conn, err := websocket.DialConfig(cfg)
	if err != nil {
		return fmt.Errorf("ma: ws dial: %w", err)
	}
	defer conn.Close()

	info, err := readServerInfo(conn)
	if err != nil {
		return fmt.Errorf("ma: server info: %w", err)
	}
	if info.SchemaVersion >= authSchemaVersion && token == "" {
		return fmt.Errorf("%w (server schema=%d)", ErrAuthRequired, info.SchemaVersion)
	}
	if info.SchemaVersion >= authSchemaVersion && token != "" {
		msgID, mErr := randHex(16)
		if mErr != nil {
			return fmt.Errorf("ma: msg id: %w", mErr)
		}
		if err := websocket.JSON.Send(conn, map[string]any{
			"message_id": msgID,
			"command":    authCommand,
			"args":       map[string]any{"token": token},
		}); err != nil {
			return fmt.Errorf("ma: auth send: %w", err)
		}
		if err := conn.SetReadDeadline(time.Now().Add(connectReadTimeout)); err != nil {
			return err
		}
		var authResp map[string]any
		if err := websocket.JSON.Receive(conn, &authResp); err != nil {
			return fmt.Errorf("ma: auth recv: %w", err)
		}
		_ = conn.SetReadDeadline(time.Time{})
		if ec, ok := authResp["error_code"]; ok && ec != nil {
			if isAuthRequiredCode(ec) {
				return fmt.Errorf("%w: %v", ErrAuthRequired, authResp)
			}
			return fmt.Errorf("ma: auth rejected: %v", authResp)
		}
	}

	msgID, err := randHex(16)
	if err != nil {
		return fmt.Errorf("ma: msg id: %w", err)
	}
	if err := websocket.JSON.Send(conn, map[string]any{
		"message_id": msgID,
		"command":    "player_queues/clear",
		"args":       map[string]any{"queue_id": queueID},
	}); err != nil {
		return fmt.Errorf("ma: clear send: %w", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	defer conn.SetReadDeadline(time.Time{})
	for {
		var resp map[string]any
		if err := websocket.JSON.Receive(conn, &resp); err != nil {
			return fmt.Errorf("ma: clear recv: %w", err)
		}
		if respID, _ := resp["message_id"].(string); respID == msgID {
			if ec, ok := resp["error_code"]; ok && ec != nil {
				return fmt.Errorf("ma: clear error: %v", ec)
			}
			log.Info("ma: queue cleared", "queue_id", queueID)
			return nil
		}
	}
}

// DerivePlayerID returns MA's internal player_id derived from the
// HA media_player entity_id. The rule is:
//
//   "media_player.<player_id>" → "<player_id>"
//   "media_player.<player_id>_N" (where N is a small integer disambiguator
//        HA adds when multiple integrations expose the same player) →
//        "<player_id>".
//
// This is the standard convention MA's HA integration uses to name
// the entities it registers. Caller should still treat the result as
// best-effort; if MA rejects the queue_id, fall back to HA REST.
func DerivePlayerID(entityID string) string {
	const prefix = "media_player."
	if len(entityID) <= len(prefix) || entityID[:len(prefix)] != prefix {
		return entityID
	}
	id := entityID[len(prefix):]
	// Strip "_N" suffix where N is 1-3 digits.
	for i := len(id) - 1; i >= 0; i-- {
		if id[i] == '_' {
			tail := id[i+1:]
			if len(tail) >= 1 && len(tail) <= 3 && allDigits(tail) {
				return id[:i]
			}
			break
		}
		if id[i] < '0' || id[i] > '9' {
			break
		}
	}
	return id
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isAuthRequiredCode reports whether ec, as decoded from a JSON value,
// represents the MA `error_code: 20` ("Authentication required").
// JSON numerics round-trip to float64 via encoding/json, but we tolerate
// the int and string forms too.
func isAuthRequiredCode(ec any) bool {
	switch v := ec.(type) {
	case float64:
		return v == 20
	case int:
		return v == 20
	case int64:
		return v == 20
	case string:
		return v == "20"
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
