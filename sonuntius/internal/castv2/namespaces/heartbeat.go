// Maps to: urn:x-cast:com.google.cast.tp.heartbeat
//
// Cast senders and receivers exchange PING/PONG messages on this namespace
// to keep the TLS channel alive in the presence of NAT timeouts and to
// detect peer death. Either side may originate a PING; the recipient
// replies with PONG.
//
// Behaviour we implement:
//   - On {"type":"PING"} we immediately reply with {"type":"PONG"}.
//   - On {"type":"PONG"} we record the timestamp (used to evict stale
//     senders later, but the Phase 3a server does not yet act on staleness).
//   - We originate {"type":"PING"} every 5s for each open sender (the
//     Server starts the ticker and calls EmitPing on each tick). If no
//     PONG arrives within 30s, the Server is expected to close the conn —
//     that policy lives in server.go because it needs the conn handle.
//
// Reference: openscreen `cast/common/channel/cast_message_handler.cc`
// (CastSocketDevice::HandleHeartbeat).
package namespaces

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"
)

// PingInterval is how often the Server's heartbeat ticker fires.
const PingInterval = 5 * time.Second

// PongTimeout is how long the Server will wait between successive PONGs
// before considering a sender dead. The Server consults LastPong() to
// decide whether to evict a sender. Defined here so the constant lives
// with the namespace it semantically belongs to.
const PongTimeout = 30 * time.Second

// Heartbeat is the heartbeat-namespace handler.
type Heartbeat struct {
	logger *slog.Logger

	mu        sync.Mutex
	lastPong  map[string]time.Time
	originSrc string // source-id the receiver uses when originating PINGs
}

// NewHeartbeat constructs a Heartbeat handler. originSrc is the source-id
// the receiver uses for the PING messages it originates (conventionally
// "receiver-0").
func NewHeartbeat(originSrc string, logger *slog.Logger) *Heartbeat {
	if logger == nil {
		logger = slog.Default()
	}
	if originSrc == "" {
		originSrc = "receiver-0"
	}
	return &Heartbeat{
		logger:    logger,
		lastPong:  make(map[string]time.Time),
		originSrc: originSrc,
	}
}

// Handle processes one incoming heartbeat-namespace message.
func (h *Heartbeat) Handle(ctx context.Context, source string, payload json.RawMessage) (Reply, error) {
	var env struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		h.logger.Debug("heartbeat: malformed payload", "source", source, "err", err)
		return Reply{}, nil
	}

	switch env.Type {
	case "PING":
		// Reply PONG to the originating sender. The payload is a constant
		// JSON object; the encoded bytes never change so the Server could
		// pool them, but this is the hot path of an idle connection and
		// not worth premature optimisation.
		return Reply{
			DestinationID: source,
			Payload:       []byte(`{"type":"PONG"}`),
		}, nil
	case "PONG":
		h.mu.Lock()
		h.lastPong[source] = time.Now()
		h.mu.Unlock()
		return Reply{}, nil
	default:
		h.logger.Debug("heartbeat: ignoring message", "source", source, "type", env.Type)
		return Reply{}, nil
	}
}

// EmitPing returns the message bytes a Server should write to originate
// a PING toward dest. Kept as a pure function so the Server's ticker
// goroutine can call it without holding any lock.
func (h *Heartbeat) EmitPing(dest string) Reply {
	return Reply{
		DestinationID: dest,
		Payload:       []byte(`{"type":"PING"}`),
	}
}

// LastPong returns the timestamp of the most recent PONG received from
// source, or the zero time if none has been seen.
func (h *Heartbeat) LastPong(source string) time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastPong[source]
}

// Forget drops a sender from the bookkeeping table; called when the
// Server tears down a connection.
func (h *Heartbeat) Forget(source string) {
	h.mu.Lock()
	delete(h.lastPong, source)
	h.mu.Unlock()
}
