// Package events defines the wire types exchanged over the IPC broker.
//
// Every event marshals to a JSON object with a "type" discriminator
// equal to the Go type name so frames are self-describing on the wire.
package events

import (
	"encoding/json"
	"fmt"
)

// Event is the marker interface all wire types satisfy. Implementers
// return a stable string used as the JSON "type" discriminator.
type Event interface {
	EventType() string
}

// PlayIntent is a request from a Cast/DIAL receiver to play a track via MA.
type PlayIntent struct {
	Provider string         `json:"provider"`
	TrackID  string         `json:"track_id,omitempty"`
	URL      string         `json:"url,omitempty"`
	Source   string         `json:"source,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// EventType implements Event.
func (PlayIntent) EventType() string { return "PlayIntent" }

// TransportCommand carries play/pause/skip/seek from a sender.
type TransportCommand struct {
	Command  string   `json:"command"`
	Position *float64 `json:"position,omitempty"`
	Source   string   `json:"source,omitempty"`
}

// EventType implements Event.
func (TransportCommand) EventType() string { return "TransportCommand" }

// VolumeCommand carries a volume / mute change from a sender.
type VolumeCommand struct {
	Level  *float64 `json:"level,omitempty"`
	Muted  *bool    `json:"muted,omitempty"`
	Source string   `json:"source,omitempty"`
}

// EventType implements Event.
func (VolumeCommand) EventType() string { return "VolumeCommand" }

// PlayerState is broadcast from ma-bridge back to receivers so they can
// keep the phone UI in sync.
type PlayerState struct {
	State    string   `json:"state"`
	Provider string   `json:"provider,omitempty"`
	TrackID  string   `json:"track_id,omitempty"`
	Position *float64 `json:"position,omitempty"`
	Duration *float64 `json:"duration,omitempty"`
	Volume   *float64 `json:"volume,omitempty"`
	Muted    *bool    `json:"muted,omitempty"`
	Title    string   `json:"title,omitempty"`
	Artist   string   `json:"artist,omitempty"`
}

// EventType implements Event.
func (PlayerState) EventType() string { return "PlayerState" }

var factories = map[string]func() Event{
	"PlayIntent":       func() Event { return &PlayIntent{} },
	"TransportCommand": func() Event { return &TransportCommand{} },
	"VolumeCommand":    func() Event { return &VolumeCommand{} },
	"PlayerState":      func() Event { return &PlayerState{} },
}

// Marshal serializes an event to a JSON object with the discriminator
// "type" field included.
func Marshal(e Event) ([]byte, error) {
	raw, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	typeBytes, err := json.Marshal(e.EventType())
	if err != nil {
		return nil, err
	}
	m["type"] = typeBytes
	return json.Marshal(m)
}

// Unmarshal reconstructs an event from a JSON object. The "type" field
// drives the dispatch.
func Unmarshal(data []byte) (Event, error) {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("events: read type: %w", err)
	}
	factory, ok := factories[head.Type]
	if !ok {
		return nil, fmt.Errorf("events: unknown type %q", head.Type)
	}
	e := factory()
	if err := json.Unmarshal(data, e); err != nil {
		return nil, fmt.Errorf("events: decode %s: %w", head.Type, err)
	}
	return e, nil
}
