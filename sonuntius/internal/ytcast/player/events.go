// Maps to: N/A — Go-only event types extracted from Player.ts EventEmitter.
//
// Upstream relies on Node's EventEmitter to broadcast state transitions to
// the rest of the receiver (`emit('state', { current, previous, AID })`). Go
// has no equivalent so we model the same payload as a struct plus a tiny
// channel-based publisher the Phase 3 orchestrator wires up.
//
// The Phase 1 contract is just the data shape. Phase 3 will use these types
// to drive the lounge layer when player state changes.
package player

import (
	"sync"

	"github.com/shobuprime/sonuntius/internal/ytcast/constants"
)

// State captures everything the upstream `getState()` returns. The Queue
// field is intentionally an opaque any so this package can stay independent
// of the Phase 2 Playlist port; the Phase 3 orchestrator will type-assert it
// to its concrete Playlist state struct.
type State struct {
	Status   constants.PlayerStatus `json:"status"`
	Queue    any                    `json:"queue,omitempty"`
	Position float64                `json:"position"`
	Duration float64                `json:"duration"`
	Volume   Volume                 `json:"volume"`
	CPN      string                 `json:"cpn"`
}

// StateEvent ports the payload upstream emits as the `state` event:
// `{ AID, current, previous }`. Previous is nil for the first transition.
type StateEvent struct {
	// AID is the lounge action id (a serial number assigned by the receiver
	// when the state change is the result of a sender request). It is
	// `*int` because upstream allows `null` / `undefined`.
	AID *int `json:"AID,omitempty"`
	// Current is the post-transition state.
	Current State `json:"current"`
	// Previous is the pre-transition state, or nil on the first event.
	Previous *State `json:"previous,omitempty"`
}

// EventBus is a tiny, channel-based fan-out the orchestrator can use to
// broadcast StateEvent values to multiple subscribers.
//
// Why not use the IPC events bus in internal/events? That bus is for the
// IPC wire protocol; this one is in-process and avoids JSON round-trips.
//
// Subscribers receive on the channel returned by Subscribe. Slow subscribers
// drop events (the publisher never blocks) — this matches upstream where a
// stalled listener also wouldn't block emit(); the listener simply misses
// events while it catches up.
type EventBus struct {
	mu          sync.RWMutex
	subscribers []*subscription
}

type subscription struct {
	ch chan StateEvent
}

// NewEventBus constructs an empty EventBus.
func NewEventBus() *EventBus {
	return &EventBus{}
}

// Subscribe returns a buffered channel that receives every Publish call
// until Unsubscribe is called or the EventBus is garbage-collected.
//
// The buffer size is configurable via `bufferSize`; pass 1 for the
// "latest event only" pattern (subsequent events overwrite a stalled slot
// after a full Drop), pass a larger value to tolerate transient slowness.
func (b *EventBus) Subscribe(bufferSize int) <-chan StateEvent {
	if bufferSize < 1 {
		bufferSize = 1
	}
	sub := &subscription{ch: make(chan StateEvent, bufferSize)}
	b.mu.Lock()
	b.subscribers = append(b.subscribers, sub)
	b.mu.Unlock()
	return sub.ch
}

// Unsubscribe removes a previously-subscribed channel. It is safe to pass an
// unknown channel (no-op).
func (b *EventBus) Unsubscribe(ch <-chan StateEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.subscribers[:0]
	for _, s := range b.subscribers {
		if (<-chan StateEvent)(s.ch) == ch {
			close(s.ch)
			continue
		}
		out = append(out, s)
	}
	b.subscribers = out
}

// Publish broadcasts `event` to every current subscriber. Subscribers whose
// buffers are full miss the event (the publisher never blocks).
func (b *EventBus) Publish(event StateEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, s := range b.subscribers {
		select {
		case s.ch <- event:
		default:
			// Slow subscriber — drop.
		}
	}
}

// Close releases every subscriber channel. Future Publish calls are no-ops
// (subscribers slice is reset).
func (b *EventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, s := range b.subscribers {
		close(s.ch)
	}
	b.subscribers = nil
}
