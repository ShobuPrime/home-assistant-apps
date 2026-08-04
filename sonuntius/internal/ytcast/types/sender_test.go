// Maps to: N/A — Go-only tests for the Sender port.
package types

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/shobuprime/sonuntius/internal/ytcast/yterrors"
)

func TestParseValidYouTube(t *testing.T) {
	raw := json.RawMessage(`{
		"id": "abc123",
		"name": "Pixel 8",
		"theme": "cl",
		"app": "youtube-desktop",
		"capabilities": "atp,mute",
		"device": "{\"brand\":\"Google\"}",
		"user": "Alice",
		"userAvatarUri": "https://avatars/alice.png"
	}`)
	s, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if s.Client == nil || s.Client.Key != ClientKeyYT {
		t.Fatalf("expected YT client, got %+v", s.Client)
	}
	if !s.SupportsAutoplay() {
		t.Fatal("expected SupportsAutoplay")
	}
	if !s.SupportsMute() {
		t.Fatal("expected SupportsMute (youtube-desktop)")
	}
	if s.User == nil || s.User.Name != "Alice" {
		t.Fatalf("user not parsed: %+v", s.User)
	}
	if s.Device["brand"] != "Google" {
		t.Fatalf("device not parsed: %+v", s.Device)
	}
}

func TestParseFallsBackToClientName(t *testing.T) {
	raw := json.RawMessage(`{"id":"x","clientName":"YT Music App","theme":"m"}`)
	s, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if s.Name != "YT Music App" {
		t.Fatalf("expected fallback to clientName, got %q", s.Name)
	}
	if s.Client == nil || s.Client.Key != ClientKeyYTMusic {
		t.Fatalf("expected YTMUSIC client, got %+v", s.Client)
	}
}

func TestParseRejectsBadPayloads(t *testing.T) {
	cases := []string{
		`{}`,
		`{"id":""}`,
		`{"id":"x","name":"y","theme":"unknown"}`,
		`{"name":"y","theme":"cl"}`, // missing id
	}
	for _, raw := range cases {
		_, err := Parse(json.RawMessage(raw))
		if err == nil {
			t.Fatalf("expected DataError for %s", raw)
		}
		if !errors.Is(err, yterrors.ErrData) {
			t.Fatalf("expected ErrData sentinel for %s, got %v", raw, err)
		}
	}
}

func TestSupportsMuteRequiresDesktopSuffix(t *testing.T) {
	s := &Sender{App: "youtube-mobile"}
	if s.SupportsMute() {
		t.Fatal("non-desktop app should not SupportsMute")
	}
	s.App = "youtube.m-desktop"
	if !s.SupportsMute() {
		t.Fatal("youtube.m-desktop should SupportsMute")
	}
}
