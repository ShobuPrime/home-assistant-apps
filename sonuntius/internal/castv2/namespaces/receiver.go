// Maps to: urn:x-cast:com.google.cast.receiver
//
// The receiver namespace is the control surface senders use to launch,
// stop, and query the receiver-app lifecycle. Real receivers run the
// Cast Receiver SDK and respond truthfully; we lie convincingly: on LAUNCH
// we respond with a synthetic RECEIVER_STATUS that claims the requested
// app is running. Tidal's Android sender (and most senders) verifies only
// that *some* app with the matching appId is present in the status — they
// do not check that the receiver actually executed HTML/JS.
//
// Wire shape (JSON over the STRING payload):
//
//	→ {"type":"LAUNCH","requestId":N,"appId":"<id>"}
//	← {"type":"RECEIVER_STATUS","requestId":N,"status":{
//	     "applications":[{
//	       "appId":"<id>", "displayName":"<our friendly name>",
//	       "sessionId":"<random>", "statusText":"Ready to cast",
//	       "transportId":"<random>", "isIdleScreen":false,
//	       "namespaces":[{"name":"<ns>"}, ...]
//	     }],
//	     "volume":{"level":1.0, "muted":false}
//	  }}
//
//	→ {"type":"STOP","requestId":N,"sessionId":"<id>"}
//	← {"type":"RECEIVER_STATUS","requestId":N,"status":{"applications":[]}}
//
//	→ {"type":"GET_STATUS","requestId":N}
//	← same RECEIVER_STATUS as the current state
//
// Reference: openscreen `cast/standalone_receiver/cast_agent.cc` and the
// Cast Receiver SDK's `cast.receiver.system.SystemSender`.
package namespaces

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"sync"
)

// Receiver is the receiver-namespace handler.
type Receiver struct {
	logger *slog.Logger

	// displayName is surfaced inside `applications[].displayName`. Phase 3b
	// will plumb the friendly_name_tidal option through here.
	displayName string

	mu          sync.Mutex
	current     *appState // nil means "idle"
	volumeLevel float64
	muted       bool
}

// appState records one launched fake app. We allow only a single app at a
// time — Cast senders never request more than one concurrently in
// practice, and Phase 3a does not need to model the more complex
// multi-app cases the real Cast Receiver SDK supports.
type appState struct {
	AppID       string   `json:"appId"`
	DisplayName string   `json:"displayName"`
	SessionID   string   `json:"sessionId"`
	StatusText  string   `json:"statusText"`
	TransportID string   `json:"transportId"`
	IsIdle      bool     `json:"isIdleScreen"`
	Namespaces  []nsName `json:"namespaces"`
}

type nsName struct {
	Name string `json:"name"`
}

// NewReceiver constructs a Receiver handler. displayName is the
// `applications[].displayName` value the receiver reports — the Cast UI
// surfaces this on the phone's "currently casting" sheet.
func NewReceiver(displayName string, logger *slog.Logger) *Receiver {
	if logger == nil {
		logger = slog.Default()
	}
	if displayName == "" {
		displayName = "Sonuntius"
	}
	return &Receiver{
		logger:      logger,
		displayName: displayName,
		volumeLevel: 1.0,
	}
}

// Handle processes one incoming receiver-namespace message.
func (r *Receiver) Handle(ctx context.Context, source string, payload json.RawMessage) (Reply, error) {
	var env struct {
		Type      string `json:"type"`
		RequestID int64  `json:"requestId"`
		AppID     string `json:"appId,omitempty"`
		SessionID string `json:"sessionId,omitempty"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		r.logger.Debug("receiver: malformed payload", "source", source, "err", err)
		return Reply{}, nil
	}

	switch env.Type {
	case "LAUNCH":
		r.launch(env.AppID)
		return r.statusReply(source, env.RequestID), nil
	case "STOP":
		r.stop()
		return r.statusReply(source, env.RequestID), nil
	case "GET_STATUS":
		return r.statusReply(source, env.RequestID), nil
	case "SET_VOLUME":
		// The full SET_VOLUME message carries either {"volume":{"level":x}}
		// or {"volume":{"muted":b}}. Phase 3a stores the incoming values so
		// the next RECEIVER_STATUS echoes them back; actual volume routing
		// to MA happens in Phase 3b's dispatcher mapping.
		var vol struct {
			Volume struct {
				Level *float64 `json:"level,omitempty"`
				Muted *bool    `json:"muted,omitempty"`
			} `json:"volume"`
		}
		if err := json.Unmarshal(payload, &vol); err == nil {
			r.mu.Lock()
			if vol.Volume.Level != nil {
				r.volumeLevel = *vol.Volume.Level
			}
			if vol.Volume.Muted != nil {
				r.muted = *vol.Volume.Muted
			}
			r.mu.Unlock()
		}
		return r.statusReply(source, env.RequestID), nil
	default:
		r.logger.Debug("receiver: ignoring message", "source", source, "type", env.Type)
		return Reply{}, nil
	}
}

// launch transitions the receiver into the "running" state for appID.
// Generates a fresh random session/transport id so each LAUNCH looks like
// a new session to the sender.
func (r *Receiver) launch(appID string) {
	if appID == "" {
		// Some senders launch with an empty appId; tolerate but log.
		r.logger.Debug("receiver: LAUNCH with empty appId")
	}
	r.mu.Lock()
	r.current = &appState{
		AppID:       appID,
		DisplayName: r.displayName,
		SessionID:   randomHex(16),
		StatusText:  "Ready to cast",
		TransportID: randomHex(16),
		IsIdle:      false,
		Namespaces: []nsName{
			{Name: "urn:x-cast:com.google.cast.media"},
			{Name: "urn:x-cast:com.google.cast.tp.connection"},
			{Name: "urn:x-cast:com.google.cast.tp.heartbeat"},
		},
	}
	r.mu.Unlock()
}

// stop transitions back to idle.
func (r *Receiver) stop() {
	r.mu.Lock()
	r.current = nil
	r.mu.Unlock()
}

// CurrentSessionID returns the session id of the currently-launched app,
// or "" if idle. The media-namespace handler queries this so it can echo
// the right sessionId on MEDIA_STATUS responses.
func (r *Receiver) CurrentSessionID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current == nil {
		return ""
	}
	return r.current.SessionID
}

// statusReply builds a RECEIVER_STATUS Reply for the given sender +
// requestId. Locks briefly to snapshot state.
func (r *Receiver) statusReply(dest string, requestID int64) Reply {
	r.mu.Lock()
	defer r.mu.Unlock()

	type volume struct {
		Level float64 `json:"level"`
		Muted bool    `json:"muted"`
	}
	type status struct {
		Applications []*appState `json:"applications"`
		Volume       volume      `json:"volume"`
	}
	type envelope struct {
		Type      string `json:"type"`
		RequestID int64  `json:"requestId"`
		Status    status `json:"status"`
	}
	env := envelope{
		Type:      "RECEIVER_STATUS",
		RequestID: requestID,
		Status: status{
			Applications: []*appState{},
			Volume:       volume{Level: r.volumeLevel, Muted: r.muted},
		},
	}
	if r.current != nil {
		env.Status.Applications = append(env.Status.Applications, r.current)
	}
	b, _ := json.Marshal(env)
	return Reply{
		DestinationID: dest,
		Payload:       b,
	}
}

// randomHex returns a hex-encoded random string of `n` bytes.
func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand should never fail on Linux; if it does, fall back to
		// a fixed string rather than panicking — the receiver still works,
		// just with a less-unique session id.
		return "0000000000000000"
	}
	return hex.EncodeToString(buf)
}
