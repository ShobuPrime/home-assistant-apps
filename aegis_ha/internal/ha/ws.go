package ha

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/net/websocket"
)

// RegisterLovelaceResource registers a JavaScript-module Lovelace resource
// in Home Assistant via the Supervisor Core-WebSocket proxy
// (ws://supervisor/core/websocket). The proxy authenticates the app's
// SUPERVISOR_TOKEN and connects upstream as the admin Supervisor system
// user, so the admin-gated lovelace/resources/create succeeds — no Core
// admin token of our own is needed (requires homeassistant_api: true).
//
// It is idempotent (dedupes by URL, ignoring the ?v= cache-buster) and
// best-effort: it only works in storage-mode Lovelace. On YAML-mode HA the
// create command is unavailable and an error is returned, which the caller
// surfaces alongside the manual resource snippet.
func RegisterLovelaceResource(token, resourceURL string, log *slog.Logger) error {
	conn, err := websocket.Dial("ws://supervisor/core/websocket", "", "http://supervisor")
	if err != nil {
		return fmt.Errorf("ha: dial core ws: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))

	// Auth handshake: auth_required -> auth -> auth_ok.
	if _, err := readFrame(conn); err != nil {
		return err
	}
	if err := websocket.JSON.Send(conn, map[string]any{"type": "auth", "access_token": token}); err != nil {
		return err
	}
	auth, err := readFrame(conn)
	if err != nil {
		return err
	}
	if auth.Type != "auth_ok" {
		return fmt.Errorf("ha: core ws auth failed: %s %s", auth.Type, auth.Message)
	}

	// Dedupe against existing resources.
	if err := websocket.JSON.Send(conn, map[string]any{"id": 1, "type": "lovelace/resources"}); err != nil {
		return err
	}
	list, err := readResult(conn, 1)
	if err != nil {
		return err
	}
	if !list.Success {
		return fmt.Errorf("ha: list resources: %s", list.Error.Message)
	}
	want := stripVersion(resourceURL)
	for _, r := range list.Result {
		if stripVersion(r.URL) == want {
			log.Info("card: Lovelace resource already registered", "url", r.URL)
			return nil
		}
	}

	// Create the resource.
	if err := websocket.JSON.Send(conn, map[string]any{
		"id": 2, "type": "lovelace/resources/create", "res_type": "module", "url": resourceURL,
	}); err != nil {
		return err
	}
	create, err := readResult(conn, 2)
	if err != nil {
		return err
	}
	if !create.Success {
		return fmt.Errorf("ha: create resource: %s (%s)", create.Error.Message, create.Error.Code)
	}
	log.Info("card: Lovelace resource auto-registered", "url", resourceURL)
	return nil
}

type wsFrame struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type wsResource struct {
	URL string `json:"url"`
}

type wsResult struct {
	ID      int          `json:"id"`
	Type    string       `json:"type"`
	Success bool         `json:"success"`
	Result  []wsResource `json:"result"`
	Error   struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func readFrame(conn *websocket.Conn) (wsFrame, error) {
	var raw []byte
	if err := websocket.Message.Receive(conn, &raw); err != nil {
		return wsFrame{}, err
	}
	var f wsFrame
	if err := json.Unmarshal(raw, &f); err != nil {
		return wsFrame{}, err
	}
	return f, nil
}

// readResult reads frames until the result for id arrives (skipping any
// interleaved events).
func readResult(conn *websocket.Conn, id int) (wsResult, error) {
	for {
		var raw []byte
		if err := websocket.Message.Receive(conn, &raw); err != nil {
			return wsResult{}, err
		}
		var r wsResult
		if err := json.Unmarshal(raw, &r); err != nil {
			continue
		}
		if r.Type == "result" && r.ID == id {
			return r, nil
		}
	}
}

func stripVersion(u string) string {
	base, _, _ := strings.Cut(u, "?")
	return base
}
