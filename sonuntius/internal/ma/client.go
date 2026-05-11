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
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"golang.org/x/net/websocket"

	"github.com/shobuprime/sonuntius/internal/events"
)

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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
