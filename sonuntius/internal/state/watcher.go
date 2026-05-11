// Package state subscribes to Home Assistant state_changed events for
// the configured Music Assistant player entity and broadcasts derived
// PlayerState updates over the IPC broker.
//
// We use the HA core WebSocket (ws://supervisor/core/websocket) via
// golang.org/x/net/websocket — the Go team's blessed WebSocket package.
// Music Assistant exposes its players as media_player.* entities in HA,
// so subscribing through HA's WS gives us everything we need without
// implementing MA's native WS protocol yet.
package state

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"golang.org/x/net/websocket"

	"github.com/shobuprime/sonuntius/internal/events"
)

const (
	defaultHACoreWSURL = "ws://supervisor/core/websocket"
	wsOrigin           = "http://supervisor"
	reconnectFloor     = 2 * time.Second
	reconnectCeil      = 60 * time.Second
)

// Broadcaster is the subset of *ipc.Server we need.
type Broadcaster interface {
	Broadcast(events.Event)
}

// Watcher maintains a persistent HA WS subscription with exponential
// backoff. Run blocks until ctx is cancelled.
type Watcher struct {
	WSURL    string
	Token    string
	EntityID string
	IPC      Broadcaster
	Logger   *slog.Logger

	connected     atomic.Bool
	firstBroadcst atomic.Bool
}

// New constructs a Watcher with the default Supervisor-proxy WS URL.
func New(token, entityID string, ipc Broadcaster, logger *slog.Logger) *Watcher {
	return NewWithURL("", token, entityID, ipc, logger)
}

// NewWithURL constructs a Watcher with an optional WS URL override.
// Pass wsURL == "" to use the Supervisor proxy default.
func NewWithURL(wsURL, token, entityID string, ipc Broadcaster, logger *slog.Logger) *Watcher {
	if logger == nil {
		logger = slog.Default()
	}
	if wsURL == "" {
		wsURL = defaultHACoreWSURL
	}
	return &Watcher{
		WSURL:    wsURL,
		Token:    token,
		EntityID: entityID,
		IPC:      ipc,
		Logger:   logger,
	}
}

// Connected reports whether the watcher currently has an active WS.
// Useful for tests and health endpoints.
func (w *Watcher) Connected() bool {
	return w.connected.Load()
}

// Run blocks, reconnecting on failure with exponential backoff.
func (w *Watcher) Run(ctx context.Context) {
	if w.EntityID == "" {
		w.Logger.Info("state watcher idle (entity_id not set)")
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
			w.Logger.Warn("state watcher disconnected", "err", err, "retry_in", backoff)
		} else {
			w.Logger.Info("state watcher cycle ended cleanly", "retry_in", backoff)
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
	url := w.WSURL
	if url == "" {
		url = defaultHACoreWSURL
	}
	cfg, err := websocket.NewConfig(url, wsOrigin)
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

	// HA WS hello / auth handshake.
	var hello struct {
		Type string `json:"type"`
	}
	if err := websocket.JSON.Receive(conn, &hello); err != nil {
		return err
	}
	if hello.Type != "auth_required" {
		w.Logger.Warn("state: unexpected handshake", "type", hello.Type)
	}
	if err := websocket.JSON.Send(conn, map[string]any{
		"type":         "auth",
		"access_token": w.Token,
	}); err != nil {
		return err
	}
	var authResp struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if err := websocket.JSON.Receive(conn, &authResp); err != nil {
		return err
	}
	if authResp.Type != "auth_ok" {
		w.Logger.Error("state: HA WS auth rejected", "msg", authResp.Message)
		return nil
	}
	w.Logger.Info("state: HA WS authenticated", "entity_id", w.EntityID)

	subID := 1
	// Subscribe to raw state_changed events rather than the higher-level
	// state-trigger. The state-trigger API on Home Assistant only fires
	// on transitions of the primary `state` field (idle ↔ playing ↔
	// paused), so attribute-only updates such as `media_position` ticking
	// forward never arrive. media_player attribute changes are what carry
	// position / duration / volume back to the receiver — we need the
	// raw event stream to see them. We filter for our entity_id
	// client-side in handleFrame.
	if err := websocket.JSON.Send(conn, map[string]any{
		"id":         subID,
		"type":       "subscribe_events",
		"event_type": "state_changed",
	}); err != nil {
		return err
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

// eventFrame is the wire shape of a subscribe_events message — the
// only one we expect now that the watcher uses the raw state_changed
// event stream rather than the higher-level state trigger.
type eventFrame struct {
	ID    int    `json:"id"`
	Type  string `json:"type"`
	Event struct {
		EventType string `json:"event_type"`
		Data      struct {
			EntityID string         `json:"entity_id"`
			NewState *haEntityState `json:"new_state"`
		} `json:"data"`
	} `json:"event"`
}

type haEntityState struct {
	State      string         `json:"state"`
	Attributes map[string]any `json:"attributes"`
}

func (w *Watcher) handleFrame(raw []byte) {
	var head struct {
		Type    string `json:"type"`
		Success *bool  `json:"success,omitempty"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		w.Logger.Debug("state: malformed frame", "err", err)
		return
	}
	switch head.Type {
	case "result":
		if head.Success != nil && !*head.Success {
			w.Logger.Warn("state: subscription rejected", "frame", string(raw))
		}
		return
	case "event":
		// fall through
	default:
		// Unexpected frame types (e.g. pong, auth, etc.) — ignore quietly.
		return
	}
	var ef eventFrame
	if err := json.Unmarshal(raw, &ef); err != nil {
		w.Logger.Debug("state: event decode failed", "err", err)
		return
	}
	if ef.Event.EventType != "state_changed" {
		return
	}
	// state_changed fires for EVERY entity in the system — filter for
	// the one we care about. The cost of receiving the firehose is one
	// 200-byte JSON parse per event, which on a typical HA install is
	// fine; the alternative (per-entity subscribe) doesn't surface
	// attribute-only updates which is exactly what we need.
	if ef.Event.Data.EntityID != w.EntityID {
		return
	}
	ps := playerStateFrom(ef.Event.Data.NewState)
	if ps == nil {
		return
	}
	// First broadcast for the configured entity is logged at info level
	// so users can confirm the HA→sonuntius state pipeline is alive
	// without flipping to debug. Subsequent updates stay at debug to
	// avoid log spam (a playing track ticks position every few seconds).
	if !w.firstBroadcst.Swap(true) {
		w.Logger.Info("state: first HA state update received — broadcasting to IPC clients",
			"entity_id", w.EntityID,
			"state", ps.State,
			"title", ps.Title,
			"track_id", ps.TrackID,
			"position", stringOrEmpty(ps.Position),
			"duration", stringOrEmpty(ps.Duration),
			"volume", stringOrEmpty(ps.Volume))
	} else {
		w.Logger.Debug("state: broadcasting", "state", ps.State,
			"position", stringOrEmpty(ps.Position))
	}
	w.IPC.Broadcast(ps)
}

func stringOrEmpty(f *float64) string {
	if f == nil {
		return ""
	}
	return fmt.Sprintf("%.2f", *f)
}

func playerStateFrom(s *haEntityState) *events.PlayerState {
	if s == nil || s.State == "" {
		return nil
	}
	ps := &events.PlayerState{State: s.State}
	if v, ok := s.Attributes["media_title"].(string); ok {
		ps.Title = v
	}
	if v, ok := s.Attributes["media_artist"].(string); ok {
		ps.Artist = v
	}
	if v, ok := s.Attributes["media_content_id"].(string); ok {
		ps.TrackID = v
	}
	if v, ok := floatAttr(s.Attributes, "media_position"); ok {
		ps.Position = &v
	}
	if v, ok := floatAttr(s.Attributes, "media_duration"); ok {
		ps.Duration = &v
	}
	if v, ok := floatAttr(s.Attributes, "volume_level"); ok {
		ps.Volume = &v
	}
	if v, ok := s.Attributes["is_volume_muted"].(bool); ok {
		ps.Muted = &v
	}
	return ps
}

func floatAttr(m map[string]any, key string) (float64, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}
