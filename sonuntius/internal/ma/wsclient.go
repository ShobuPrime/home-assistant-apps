// Package ma — long-lived MA WebSocket client.
//
// WSClient is a single persistent connection to Music Assistant's WS
// endpoint. It owns the read loop, authenticates on (re)connect,
// matches command responses to in-flight callers by `message_id`, and
// fans out event pushes to a registered handler.
//
// Why a single long-lived connection instead of the per-call short
// connections used by ClearQueue / PlayMediaItem / ListPlayers:
//
//   - Cuts hop count + handshake latency. A `media_seek` over HA REST
//     reliably takes 3-4 seconds because HA waits for MA's Python
//     integration to round-trip. Sent over a hot MA WS it finishes in
//     ~20 ms.
//   - Lets us issue play_media and an immediate `player_queues/seek`
//     on the same connection so MA never starts playback from 0 when
//     the cast sender specified a start position.
//   - Foundation for queue mirroring: events from the cast sender
//     (autoplay up-next, queue modifications) can be reflected to MA
//     via additional commands on this connection.
package ma

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/websocket"
)

// EventHandler is invoked for every event push (frames whose `event`
// field is set). The raw JSON of the `data` field is passed through so
// the caller can decode based on event type.
type EventHandler func(eventName, objectID string, data json.RawMessage)

// WSClient is a persistent MA WS connection with command/response
// correlation. Construct with NewWSClient, then Start once; methods
// can be called concurrently and block until response or context
// cancellation. Stop closes the underlying connection and stops the
// reconnect loop.
type WSClient struct {
	URL     string
	Token   string
	Logger  *slog.Logger
	OnEvent EventHandler

	// commandTimeout caps how long a single Send waits for a
	// matching response. A connection drop while waiting unblocks
	// the call early with ErrDisconnected.
	commandTimeout time.Duration

	mu      sync.Mutex
	conn    *websocket.Conn
	pending map[string]chan map[string]any
	closed  bool

	connected atomic.Bool

	started bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// ErrDisconnected is returned from Send when the connection drops
// before a response is received. Callers may retry once the client
// reconnects (use Connected() to check) — but most call sites in
// this codebase fall back to the HA REST path instead.
var ErrDisconnected = errors.New("ma ws: disconnected")

// ErrNotStarted is returned by methods called before Start.
var ErrNotStarted = errors.New("ma ws: not started")

// NewWSClient constructs a client. URL is the full ws:// URL
// (typically `ws://<ma-addon-host>:8095/ws`). Token is optional —
// required only when MA's server-info reports schema_version >=
// authSchemaVersion. OnEvent may be nil if the caller doesn't care
// about event pushes.
func NewWSClient(url, token string, logger *slog.Logger, onEvent EventHandler) *WSClient {
	if logger == nil {
		logger = slog.Default()
	}
	return &WSClient{
		URL:            url,
		Token:          token,
		Logger:         logger,
		OnEvent:        onEvent,
		commandTimeout: 15 * time.Second,
		pending:        make(map[string]chan map[string]any),
	}
}

// Start kicks off the background dial / read / reconnect loop. Safe
// to call once. Returns nil immediately; connection happens
// asynchronously.
func (c *WSClient) Start(ctx context.Context) {
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return
	}
	c.started = true
	rctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.done = make(chan struct{})
	c.mu.Unlock()
	go c.runLoop(rctx)
}

// Stop closes the active connection (if any) and signals the loop to
// exit. Safe to call multiple times.
func (c *WSClient) Stop() {
	c.mu.Lock()
	c.closed = true
	cancel := c.cancel
	conn := c.conn
	doneCh := c.done
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if conn != nil {
		_ = conn.Close()
	}
	if doneCh != nil {
		<-doneCh
	}
}

// Connected reports whether the read loop currently has an open
// connection (and has finished its auth handshake).
func (c *WSClient) Connected() bool {
	return c.connected.Load()
}

// runLoop dials, authenticates, reads, reconnects with exponential
// backoff. Exits only on ctx cancel.
func (c *WSClient) runLoop(ctx context.Context) {
	defer func() {
		c.mu.Lock()
		done := c.done
		c.mu.Unlock()
		if done != nil {
			close(done)
		}
	}()
	backoff := reconnectFloor
	for {
		if ctx.Err() != nil {
			return
		}
		err := c.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			c.Logger.Warn("ma ws: connection lost", "err", err, "retry_in", backoff)
		}
		c.failPending(ErrDisconnected)
		c.connected.Store(false)
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

// runOnce performs one connect/auth/read cycle. Returns when the
// connection drops, an error occurs, or ctx is cancelled.
func (c *WSClient) runOnce(ctx context.Context) error {
	cfg, err := websocket.NewConfig(c.URL, origin())
	if err != nil {
		return err
	}
	conn, err := websocket.DialConfig(cfg)
	if err != nil {
		return err
	}
	info, err := readServerInfo(conn)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("server info: %w", err)
	}
	if info.SchemaVersion >= authSchemaVersion {
		if c.Token == "" {
			_ = conn.Close()
			return fmt.Errorf("%w (schema=%d)", ErrAuthRequired, info.SchemaVersion)
		}
		if err := c.authenticate(conn); err != nil {
			_ = conn.Close()
			return fmt.Errorf("auth: %w", err)
		}
	}
	c.Logger.Info("ma ws: connected",
		"server_id", info.ServerID, "server_version", info.ServerVersion,
		"schema", info.SchemaVersion)

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	c.connected.Store(true)

	// Close the connection when ctx is cancelled so the read loop unblocks.
	closer := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-closer:
		}
	}()
	defer close(closer)

	for {
		var raw []byte
		if err := websocket.Message.Receive(conn, &raw); err != nil {
			c.mu.Lock()
			c.conn = nil
			c.mu.Unlock()
			return err
		}
		c.handleFrame(raw)
	}
}

// authenticate sends the auth command on a freshly-dialled connection
// and waits for the response. Used only during connect.
func (c *WSClient) authenticate(conn *websocket.Conn) error {
	msgID, err := randHex(16)
	if err != nil {
		return err
	}
	body := map[string]any{
		"message_id": msgID,
		"command":    authCommand,
		"args":       map[string]any{"token": c.Token},
	}
	if err := websocket.JSON.Send(conn, body); err != nil {
		return err
	}
	if err := conn.SetReadDeadline(time.Now().Add(connectReadTimeout)); err != nil {
		return err
	}
	defer conn.SetReadDeadline(time.Time{})
	var resp map[string]any
	if err := websocket.JSON.Receive(conn, &resp); err != nil {
		return err
	}
	if ec, ok := resp["error_code"]; ok && ec != nil {
		if isAuthRequiredCode(ec) {
			return fmt.Errorf("%w: %v", ErrAuthRequired, resp)
		}
		return fmt.Errorf("auth rejected: %v", resp)
	}
	return nil
}

// handleFrame routes one incoming frame: command response -> pending
// channel, event push -> OnEvent.
func (c *WSClient) handleFrame(raw []byte) {
	var env struct {
		MessageID string          `json:"message_id"`
		Event     string          `json:"event"`
		ObjectID  string          `json:"object_id"`
		Data      json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		c.Logger.Debug("ma ws: dropped malformed frame", "err", err)
		return
	}
	if env.MessageID != "" {
		c.deliverResponse(env.MessageID, raw)
		return
	}
	if env.Event != "" && c.OnEvent != nil {
		c.OnEvent(env.Event, env.ObjectID, env.Data)
	}
}

func (c *WSClient) deliverResponse(msgID string, raw []byte) {
	c.mu.Lock()
	ch, ok := c.pending[msgID]
	if ok {
		delete(c.pending, msgID)
	}
	c.mu.Unlock()
	if !ok {
		return
	}
	var resp map[string]any
	if err := json.Unmarshal(raw, &resp); err != nil {
		resp = map[string]any{"message_id": msgID, "_decode_err": err.Error()}
	}
	// Non-blocking send: the receiver may have abandoned the wait.
	select {
	case ch <- resp:
	default:
	}
}

// failPending unblocks every in-flight Send caller with the given
// error, used when the connection drops.
func (c *WSClient) failPending(err error) {
	c.mu.Lock()
	pending := c.pending
	c.pending = make(map[string]chan map[string]any)
	c.mu.Unlock()
	for _, ch := range pending {
		select {
		case ch <- map[string]any{"_disconnect_err": err.Error()}:
		default:
		}
	}
}

// Send issues a command and waits for the matching response. Returns
// the decoded response map (caller inspects `result` / `error_code`)
// or an error if the call timed out / context expired / the WS dropped.
func (c *WSClient) Send(ctx context.Context, command string, args map[string]any) (map[string]any, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrNotStarted
	}
	conn := c.conn
	c.mu.Unlock()
	if conn == nil || !c.connected.Load() {
		return nil, ErrDisconnected
	}
	msgID, err := randHex(16)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	c.mu.Lock()
	c.pending[msgID] = ch
	c.mu.Unlock()

	body := map[string]any{
		"message_id": msgID,
		"command":    command,
		"args":       args,
	}
	if err := websocket.JSON.Send(conn, body); err != nil {
		c.mu.Lock()
		delete(c.pending, msgID)
		c.mu.Unlock()
		return nil, fmt.Errorf("ma ws: send %s: %w", command, err)
	}

	timeout := c.commandTimeout
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case resp := <-ch:
		if discErr, ok := resp["_disconnect_err"].(string); ok {
			return nil, fmt.Errorf("ma ws: %s: %s", command, discErr)
		}
		if ec, ok := resp["error_code"]; ok && ec != nil {
			details, _ := resp["details"].(string)
			if isAuthRequiredCode(ec) {
				return resp, fmt.Errorf("%w: %s: %v %s", ErrAuthRequired, command, ec, details)
			}
			return resp, fmt.Errorf("ma ws: %s: error_code=%v %s", command, ec, details)
		}
		return resp, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, msgID)
		c.mu.Unlock()
		return nil, ctx.Err()
	case <-timer.C:
		c.mu.Lock()
		delete(c.pending, msgID)
		c.mu.Unlock()
		return nil, fmt.Errorf("ma ws: %s: timed out after %s", command, timeout)
	}
}

// SendFireAndForget writes a command to the WS without registering a
// response slot. The frame goes out as a single Write() — no
// per-message goroutine, no pending-map entry, no wait for MA's ACK.
// Use this for idempotent commands where the caller doesn't need to
// branch on success/failure: volume_set, volume_mute, players/cmd/
// pause|play|stop|next|previous. Two rapid volume presses can each
// dispatch in <1 ms instead of being serialised behind a 20-50 ms
// response wait.
//
// Returns an error only when the WS is not currently connected or
// when the Write itself fails. The caller logs and moves on.
func (c *WSClient) SendFireAndForget(_ context.Context, command string, args map[string]any) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrNotStarted
	}
	conn := c.conn
	c.mu.Unlock()
	if conn == nil || !c.connected.Load() {
		return ErrDisconnected
	}
	msgID, err := randHex(16)
	if err != nil {
		return err
	}
	body := map[string]any{
		"message_id": msgID,
		"command":    command,
		"args":       args,
	}
	if err := websocket.JSON.Send(conn, body); err != nil {
		return fmt.Errorf("ma ws: send %s: %w", command, err)
	}
	return nil
}

// PlayQueueMedia issues `player_queues/play_media`. Returns the
// command result map (caller can ignore for fire-and-forget).
//
// `option` is one of "play", "replace", "next", "replace_next", "add".
func (c *WSClient) PlayQueueMedia(ctx context.Context, queueID string, item MediaItem, option string) error {
	_, err := c.Send(ctx, "player_queues/play_media", map[string]any{
		"queue_id": queueID,
		"media":    []MediaItem{item},
		"option":   option,
	})
	return err
}

// AddToQueueMedia is a convenience wrapper for PlayQueueMedia with
// option="add". Used by queue-mirroring to append upcoming items.
func (c *WSClient) AddToQueueMedia(ctx context.Context, queueID string, item MediaItem) error {
	return c.PlayQueueMedia(ctx, queueID, item, "add")
}

// ClearQueue clears the queue for queueID.
func (c *WSClient) ClearQueue(ctx context.Context, queueID string) error {
	_, err := c.Send(ctx, "player_queues/clear", map[string]any{"queue_id": queueID})
	return err
}

// Seek issues `player_queues/seek` to the absolute position in seconds.
func (c *WSClient) Seek(ctx context.Context, queueID string, position float64) error {
	_, err := c.Send(ctx, "player_queues/seek", map[string]any{
		"queue_id": queueID,
		"position": int(position),
	})
	return err
}

// PlayerPause / PlayerPlay / PlayerStop / PlayerNext / PlayerPrevious
// call MA's per-player transport commands. playerID is the MA internal
// player_id (same value as queueID for single-player queues).
//
// Sent fire-and-forget — these are idempotent and "latest wins" from
// MA's perspective, so we don't pay the WS round-trip response wait.
func (c *WSClient) PlayerPause(ctx context.Context, playerID string) error {
	return c.SendFireAndForget(ctx, "players/cmd/pause", map[string]any{"player_id": playerID})
}
func (c *WSClient) PlayerPlay(ctx context.Context, playerID string) error {
	return c.SendFireAndForget(ctx, "players/cmd/play", map[string]any{"player_id": playerID})
}
func (c *WSClient) PlayerStop(ctx context.Context, playerID string) error {
	return c.SendFireAndForget(ctx, "players/cmd/stop", map[string]any{"player_id": playerID})
}
func (c *WSClient) PlayerNext(ctx context.Context, playerID string) error {
	return c.SendFireAndForget(ctx, "players/cmd/next", map[string]any{"player_id": playerID})
}
func (c *WSClient) PlayerPrevious(ctx context.Context, playerID string) error {
	return c.SendFireAndForget(ctx, "players/cmd/previous", map[string]any{"player_id": playerID})
}

// SetVolume sets the per-player volume in the 0-100 wire range.
// Fire-and-forget: rapid presses can each dispatch in <1 ms instead
// of being serialised behind a 20-50 ms response wait.
func (c *WSClient) SetVolume(ctx context.Context, playerID string, level int) error {
	if level < 0 {
		level = 0
	}
	if level > 100 {
		level = 100
	}
	return c.SendFireAndForget(ctx, "players/cmd/volume_set", map[string]any{
		"player_id":    playerID,
		"volume_level": level,
	})
}

// SetMute sets the per-player mute state. Fire-and-forget.
func (c *WSClient) SetMute(ctx context.Context, playerID string, muted bool) error {
	return c.SendFireAndForget(ctx, "players/cmd/volume_mute", map[string]any{
		"player_id": playerID,
		"muted":     muted,
	})
}
