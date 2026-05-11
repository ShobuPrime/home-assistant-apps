// Maps to: N/A — Go-only tests for the Tidal LOAD parser.
package parsers

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/shobuprime/sonuntius/internal/castv2/namespaces"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestTidalParserCustomDataShapes(t *testing.T) {
	cases := []struct {
		name   string
		load   namespaces.MediaLoad
		wantID string
	}{
		{
			name: "outer customData tidal.trackId (string)",
			load: namespaces.MediaLoad{
				OuterCustomData: json.RawMessage(`{"tidal":{"trackId":"123456789"}}`),
			},
			wantID: "123456789",
		},
		{
			name: "outer customData tidal.trackId (numeric)",
			load: namespaces.MediaLoad{
				OuterCustomData: json.RawMessage(`{"tidal":{"trackId":123456789}}`),
			},
			wantID: "123456789",
		},
		{
			name: "inner customData trackId",
			load: namespaces.MediaLoad{
				CustomData: json.RawMessage(`{"trackId":"42"}`),
			},
			wantID: "42",
		},
		{
			name: "inner customData media.trackId",
			load: namespaces.MediaLoad{
				CustomData: json.RawMessage(`{"media":{"trackId":"77"}}`),
			},
			wantID: "77",
		},
		{
			name: "outer customData data.trackId",
			load: namespaces.MediaLoad{
				OuterCustomData: json.RawMessage(`{"data":{"trackId":"99"}}`),
			},
			wantID: "99",
		},
		{
			name: "metadata subtitle mentions Tidal, contentId is numeric",
			load: namespaces.MediaLoad{
				ContentID: "8675309",
				Metadata:  json.RawMessage(`{"subtitle":"Tidal HiFi"}`),
			},
			wantID: "8675309",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewTidal(quietLogger())
			intent, ok, err := p.Parse(&tc.load)
			if err != nil {
				t.Fatalf("Parse err: %v", err)
			}
			if !ok {
				t.Fatalf("Parse did not claim payload")
			}
			if intent.Provider != "tidal" {
				t.Errorf("Provider = %q, want tidal", intent.Provider)
			}
			if intent.TrackID != tc.wantID {
				t.Errorf("TrackID = %q, want %q", intent.TrackID, tc.wantID)
			}
		})
	}
}

func TestTidalParserDefers(t *testing.T) {
	cases := []struct {
		name string
		load namespaces.MediaLoad
	}{
		{
			name: "empty LOAD",
			load: namespaces.MediaLoad{},
		},
		{
			name: "youtube-shaped LOAD",
			load: namespaces.MediaLoad{
				ContentID:   "abcd1234",
				ContentType: "video/mp4",
				CustomData:  json.RawMessage(`{"vid":"abcd1234"}`),
			},
		},
		{
			name: "tidal key present but no trackId leaf",
			load: namespaces.MediaLoad{
				OuterCustomData: json.RawMessage(`{"tidal":{"foo":"bar"}}`),
			},
		},
		{
			name: "malformed JSON blob — must not crash",
			load: namespaces.MediaLoad{
				OuterCustomData: json.RawMessage(`{not json`),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewTidal(quietLogger())
			_, ok, err := p.Parse(&tc.load)
			if err != nil {
				t.Fatalf("Parse err: %v", err)
			}
			if ok {
				t.Fatalf("Parse claimed payload it should have deferred on")
			}
		})
	}
}

func TestTidalParserName(t *testing.T) {
	p := NewTidal(nil)
	if p.Name() != "tidal" {
		t.Errorf("Name = %q, want tidal", p.Name())
	}
}
