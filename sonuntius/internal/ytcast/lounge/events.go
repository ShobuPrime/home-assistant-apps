// Maps to: N/A — Go-only event bus extracted from upstream's per-class
// Node `EventEmitter` usage. Upstream emits `messages` / `terminate` from
// `RPCConnection` and `Session`, and `request` / `response` / `error` from
// `PairingCodeRequestService`. Go has no global EventEmitter; this file
// declares a tiny channel-based fan-out used by the lounge layer to surface
// session-level events to the Phase 3 orchestrator.
//
// Contract
//
//   - The bus is a single broadcast point that the lounge layer publishes
//     into. Consumers subscribe and receive on a returned channel.
//   - Each individual lounge component (Session, RPCConnection,
//     PairingCodeRequestService, Playlist) still exposes its own
//     fine-grained callbacks so callers can wire low-level concerns
//     directly. The bus carries the higher-level, session-scoped events
//     the orchestrator cares about.
//   - The publisher never blocks. Subscribers with full buffers miss
//     events — matching upstream where a stalled `on('messages')` handler
//     would not stop `emit('messages')` from returning.
//   - All event types implement the unexported eventTag() interface so the
//     channel element type is constrained.
//
// Event taxonomy
//
//   - SessionConnectedEvent      — session reached RUNNING.
//   - SessionDisconnectedEvent   — session ended (clean or error).
//   - MessageReceivedEvent       — a batch of incoming messages was parsed
//                                  and dispatched.
//   - PairingCodeReadyEvent      — a fresh pairing code is available.
//   - PairingCodeErrorEvent      — the pairing service errored and stopped.
//   - RPCConnectionTerminatedEvent — the underlying RPC stream gave up; the
//                                    Session reacts internally but the event
//                                    is published for observability.
package lounge

import (
	"sync"
	"time"
)

// Event is the closed interface every event type implements. The unexported
// method keeps the variant set sealed to this package.
type Event interface {
	eventTag()
}

// SessionConnectedEvent is published when Session.Begin succeeds.
type SessionConnectedEvent struct {
	Session *Session
}

func (SessionConnectedEvent) eventTag() {}

// SessionDisconnectedEvent is published when a session transitions to
// stopped. Reason is nil for a clean disconnect (Session.End) and carries
// the wrapped error for involuntary terminations.
type SessionDisconnectedEvent struct {
	Session *Session
	Reason  error
}

func (SessionDisconnectedEvent) eventTag() {}

// MessageReceivedEvent is published every time the session dispatches a
// batch of incoming messages (after BindParams has been updated with their
// AIDs). Messages share ownership with the session — consumers must not
// mutate them.
type MessageReceivedEvent struct {
	Session  *Session
	Messages []*Message
}

func (MessageReceivedEvent) eventTag() {}

// PairingCodeReadyEvent is published whenever the pairing-code service
// receives a fresh code. RefreshAfter is the soonest the next code will
// arrive (matching upstream's REFRESH_INTERVAL of 5 minutes; the bus
// passes this through so consumers can show an expiry hint without
// hard-coding the value).
type PairingCodeReadyEvent struct {
	Code         string
	RefreshAfter time.Duration
}

func (PairingCodeReadyEvent) eventTag() {}

// PairingCodeErrorEvent is published when the pairing-code service errors
// out. The service stops on error (matching upstream), so subscribers can
// treat this as terminal until Start is called again.
type PairingCodeErrorEvent struct {
	Err error
}

func (PairingCodeErrorEvent) eventTag() {}

// RPCConnectionTerminatedEvent is published when the underlying RPC
// long-poll has exhausted its retries. The Session reacts internally by
// refreshing the lounge token; this event surfaces the condition for
// telemetry / debugging consumers.
type RPCConnectionTerminatedEvent struct {
	Err error
}

func (RPCConnectionTerminatedEvent) eventTag() {}

// EventBus is a thread-safe, broadcast event bus. The zero value is not
// usable — use NewEventBus.
type EventBus struct {
	mu          sync.RWMutex
	subscribers []*eventSubscription
}

type eventSubscription struct {
	ch chan Event
}

// NewEventBus constructs an empty EventBus.
func NewEventBus() *EventBus {
	return &EventBus{}
}

// Subscribe returns a buffered channel that receives every Publish call
// until Unsubscribe is called. bufferSize must be >= 1; smaller values are
// clamped to 1. Slow consumers drop events.
func (b *EventBus) Subscribe(bufferSize int) <-chan Event {
	if bufferSize < 1 {
		bufferSize = 1
	}
	sub := &eventSubscription{ch: make(chan Event, bufferSize)}
	b.mu.Lock()
	b.subscribers = append(b.subscribers, sub)
	b.mu.Unlock()
	return sub.ch
}

// Unsubscribe removes a previously returned channel and closes it. It is
// safe to pass an unknown channel (no-op).
func (b *EventBus) Unsubscribe(ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.subscribers[:0]
	for _, s := range b.subscribers {
		if (<-chan Event)(s.ch) == ch {
			close(s.ch)
			continue
		}
		out = append(out, s)
	}
	b.subscribers = out
}

// Publish broadcasts evt to every subscriber. Subscribers whose buffers
// are full miss the event; the call never blocks.
func (b *EventBus) Publish(evt Event) {
	if b == nil {
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, s := range b.subscribers {
		select {
		case s.ch <- evt:
		default:
			// Slow subscriber — drop.
		}
	}
}

// Close releases every subscriber channel. Subsequent Publish calls are
// no-ops.
func (b *EventBus) Close() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, s := range b.subscribers {
		close(s.ch)
	}
	b.subscribers = nil
}
