// Maps to: N/A — Go-only public types shared across the castv2 sub-packages.
//
// These live at the top-level so the Phase 3b cmd binary can import them
// without pulling in the namespace handlers or the auth responder when it
// only needs the small Intent / Message types.
package castv2

import (
	"encoding/json"
)

// Well-known CASTV2 namespace strings. Exported so namespace handler
// packages can register against them and so tests can construct messages.
const (
	NamespaceDeviceAuth = "urn:x-cast:com.google.cast.tp.deviceauth"
	NamespaceConnection = "urn:x-cast:com.google.cast.tp.connection"
	NamespaceHeartbeat  = "urn:x-cast:com.google.cast.tp.heartbeat"
	NamespaceReceiver   = "urn:x-cast:com.google.cast.receiver"
	NamespaceMedia      = "urn:x-cast:com.google.cast.media"
)

// Well-known Cast peer identifiers. "sender-0" / "receiver-0" are the
// default platform endpoints; per-app/per-session ids are negotiated via
// CONNECT messages on the connection namespace.
const (
	PlatformSenderID   = "sender-0"
	PlatformReceiverID = "receiver-0"
)

// Message is a higher-level representation of an incoming or outgoing
// CASTV2 frame as seen by namespace handlers. It carries the routing
// metadata (source / destination / namespace) and the decoded JSON payload
// for STRING messages, leaving binary payloads accessible via Raw.
type Message struct {
	// SourceID is the sender's source-id (e.g. "client-12345") for inbound
	// messages, or the receiver's source-id for outbound.
	SourceID string
	// DestinationID is conventionally PlatformReceiverID for control
	// messages and the launched session id for media messages.
	DestinationID string
	// Namespace is one of the Namespace* constants (or any custom namespace
	// registered by an app).
	Namespace string
	// PayloadJSON is the raw JSON object body for STRING payloads. Empty
	// for BINARY payloads.
	PayloadJSON json.RawMessage
	// PayloadBinary holds the bytes for BINARY payloads (currently only the
	// deviceauth namespace uses these).
	PayloadBinary []byte
}

// IsBinary reports whether the message carries a BINARY (vs STRING) payload.
func (m *Message) IsBinary() bool {
	return len(m.PayloadBinary) > 0 && len(m.PayloadJSON) == 0
}

// ToCastMessage projects m into the wire-level CastMessage struct.
func (m *Message) ToCastMessage() *CastMessage {
	cm := &CastMessage{
		ProtocolVersion: ProtocolVersionCastV21,
		SourceID:        m.SourceID,
		DestinationID:   m.DestinationID,
		Namespace:       m.Namespace,
	}
	if m.IsBinary() {
		cm.PayloadType = PayloadTypeBinary
		cm.PayloadBinary = m.PayloadBinary
	} else {
		cm.PayloadType = PayloadTypeString
		cm.PayloadUTF8 = string(m.PayloadJSON)
	}
	return cm
}

// FromCastMessage builds a Message from a wire-level CastMessage.
func FromCastMessage(cm *CastMessage) *Message {
	msg := &Message{
		SourceID:      cm.SourceID,
		DestinationID: cm.DestinationID,
		Namespace:     cm.Namespace,
	}
	if cm.PayloadType == PayloadTypeBinary {
		msg.PayloadBinary = cm.PayloadBinary
	} else {
		msg.PayloadJSON = json.RawMessage(cm.PayloadUTF8)
	}
	return msg
}

// Intent is the small struct Phase 3b's cmd binary maps onto the existing
// events.PlayIntent type. It is deliberately decoupled from internal/events
// so this package has zero dependencies on the rest of the addon.
//
// Provider is one of "tidal", "url" (Phase 4 generic Default Media
// Receiver), or any future identifier added by a Parser.
type Intent struct {
	Provider string
	TrackID  string
	URL      string
	Metadata map[string]any
}
