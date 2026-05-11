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

	connected atomic.Bool
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
	if err := websocket.JSON.Send(conn, map[string]any{
		"id":   subID,
		"type": "subscribe_trigger",
		"trigger": map[string]any{
			"platform":  "state",
			"entity_id": w.EntityID,
		},
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

type triggerFrame struct {
	ID    int             `json:"id"`
	Type  string          `json:"type"`
	Event json.RawMessage `json:"event"`

	// Different HA versions land the new state under either .event.variables.trigger
	// or .variables.trigger; we try both.
	Variables struct {
		Trigger struct {
			ToState haEntityState `json:"to_state"`
		} `json:"trigger"`
	} `json:"variables"`
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
			w.Logger.Warn("state: subscribe_trigger rejected", "frame", string(raw))
		}
		return
	case "trigger":
		// fall through
	case "event":
		// fall through (older HA shape)
	default:
		return
	}
	var tf triggerFrame
	if err := json.Unmarshal(raw, &tf); err != nil {
		w.Logger.Debug("state: trigger decode failed", "err", err)
		return
	}
	ps := playerStateFrom(&tf.Variables.Trigger.ToState)
	if ps == nil {
		return
	}
	w.Logger.Debug("state: broadcasting", "state", ps.State, "title", ps.Title)
	w.IPC.Broadcast(ps)
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
