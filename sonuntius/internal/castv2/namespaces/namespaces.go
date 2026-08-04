// Maps to: N/A — Go-only shared types for the per-namespace handlers.
//
// Each namespace (connection / heartbeat / receiver / media) has its own
// .go file modelling the wire protocol, but the dispatcher in
// internal/castv2/server.go calls every handler through the same
// signature. This file pins that signature in one place so the Server can
// treat handlers polymorphically.
//
// Reply is the outbound side of the contract. Handlers return zero values
// when no reply should be emitted; otherwise the Server attaches the
// appropriate namespace + source-id and writes the frame.
package namespaces

import (
	"context"
	"encoding/json"
)

// Handler is the common shape of every namespace handler used by the
// Server. The Server dispatches on the incoming Message.Namespace; each
// Handler may return a Reply (which the Server frames + writes) and
// optionally an error (logged at warn).
type Handler interface {
	Handle(ctx context.Context, source string, payload json.RawMessage) (Reply, error)
}

// Reply is what a Handler asks the Server to send back to the sender. An
// empty Reply (Payload nil) means "no response — the inbound message was
// one-way upstream".
type Reply struct {
	// Namespace overrides the namespace the Server emits the reply on.
	// Empty means "use the same namespace as the inbound message", which
	// is the common case (PING → PONG, LOAD → MEDIA_STATUS, etc.).
	Namespace string
	// DestinationID overrides the destination-id of the outbound frame.
	// Empty means "echo the inbound source-id" (the conventional answer).
	DestinationID string
	// Payload is the JSON object bytes to put in the outbound STRING
	// payload. Empty means "no reply".
	Payload json.RawMessage
}

// IsEmpty reports whether the Reply represents "no reply".
func (r Reply) IsEmpty() bool {
	return len(r.Payload) == 0
}
