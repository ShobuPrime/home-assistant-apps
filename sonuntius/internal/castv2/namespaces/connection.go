// Maps to: urn:x-cast:com.google.cast.tp.connection
//
// Cast senders open a virtual connection per (sender, receiver) pair by
// sending {"type":"CONNECT"} on this namespace before any other traffic.
// They close it by sending {"type":"CLOSE"} (often during graceful sender
// shutdown). The receiver never originates a message on this namespace and
// never replies to CONNECT — CONNECT is one-way upstream.
//
// Real Cast receivers (Chromecast) also surface CONNECT_PROBE in newer
// versions; we ignore it for now since it is only used by enterprise
// senders to keep the channel alive without traffic, and Phase 3a senders
// (Tidal, Default Media Receiver) do not emit it.
//
// Reference: openscreen `cast/standalone_receiver/sender_socket_factory.cc`
// + `cast/common/channel/virtual_connection_router.cc`.
package namespaces

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
)

// Connection is the connection-namespace handler. It tracks which sender
// source-ids have an open virtual connection so the heartbeat and receiver
// handlers can ignore traffic from senders that have not handshaked.
type Connection struct {
	logger *slog.Logger

	mu      sync.Mutex
	openIDs map[string]struct{}
}

// NewConnection constructs a Connection handler.
func NewConnection(logger *slog.Logger) *Connection {
	if logger == nil {
		logger = slog.Default()
	}
	return &Connection{
		logger:  logger,
		openIDs: make(map[string]struct{}),
	}
}

// Handle processes one incoming connection-namespace message. Returns the
// outbound message to send (or nil — CONNECT yields no reply) and an
// error.
//
// Inputs and outputs use the host-defined Reply type because handler
// signatures vary across namespaces (some originate unsolicited frames on
// timers, some are pure request/response). Keeping the signature
// consistent (ctx, source, payload) → Reply lets the Server dispatch table
// stay simple.
func (c *Connection) Handle(ctx context.Context, source string, payload json.RawMessage) (Reply, error) {
	var env struct {
		Type string `json:"type"`
	}
	// Connection payloads are sometimes empty JSON objects ("{}"); tolerate
	// that by treating Unmarshal errors as "unknown type" and logging.
	if err := json.Unmarshal(payload, &env); err != nil {
		c.logger.Debug("connection: malformed payload", "source", source, "err", err)
		return Reply{}, nil
	}

	switch env.Type {
	case "CONNECT", "CONNECT_PROBE":
		c.mu.Lock()
		c.openIDs[source] = struct{}{}
		c.mu.Unlock()
		c.logger.Debug("connection: opened", "source", source)
		return Reply{}, nil // CONNECT is one-way upstream
	case "CLOSE":
		c.mu.Lock()
		delete(c.openIDs, source)
		c.mu.Unlock()
		c.logger.Debug("connection: closed", "source", source)
		return Reply{}, nil
	default:
		c.logger.Debug("connection: ignoring message", "source", source, "type", env.Type)
		return Reply{}, nil
	}
}

// IsOpen reports whether the given sender source-id has an open virtual
// connection. The Server uses this to gate other namespaces — Cast does
// not require this, but it makes the receiver more predictable when a
// sender misbehaves.
func (c *Connection) IsOpen(source string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.openIDs[source]
	return ok
}

// Open marks a sender as connected. The Server calls this for senders that
// emit a non-CONNECT message first (Chrome's openscreen sometimes does
// this on reconnects) so we don't drop a real session.
func (c *Connection) Open(source string) {
	c.mu.Lock()
	c.openIDs[source] = struct{}{}
	c.mu.Unlock()
}

// Close drops a sender from the open set. Called by the Server on
// connection teardown.
func (c *Connection) Close(source string) {
	c.mu.Lock()
	delete(c.openIDs, source)
	c.mu.Unlock()
}

// Sources snapshots the current set of open sender source-ids.
// Test-friendly accessor.
func (c *Connection) Sources() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.openIDs))
	for k := range c.openIDs {
		out = append(out, k)
	}
	return out
}
