// Maps to: N/A — Go-only tests for the generic audio-URL LOAD parser.
package parsers

import (
	"testing"

	"github.com/shobuprime/sonuntius/internal/castv2/namespaces"
)

func TestGenericParserClaims(t *testing.T) {
	cases := []struct {
		name    string
		load    namespaces.MediaLoad
		wantURL string
	}{
		{
			name: "audio/mpeg URL",
			load: namespaces.MediaLoad{
				ContentID:   "https://example.com/track.mp3",
				ContentType: "audio/mpeg",
			},
			wantURL: "https://example.com/track.mp3",
		},
		{
			name: "audio/ogg URL",
			load: namespaces.MediaLoad{
				ContentID:   "http://example.com/stream.ogg",
				ContentType: "audio/ogg",
			},
			wantURL: "http://example.com/stream.ogg",
		},
		{
			name: "audio/mp4 URL with uppercase scheme",
			load: namespaces.MediaLoad{
				ContentID:   "HTTPS://example.com/track.m4a",
				ContentType: "audio/mp4",
			},
			wantURL: "HTTPS://example.com/track.m4a",
		},
		{
			name: "empty contentType but https URL",
			load: namespaces.MediaLoad{
				ContentID:   "https://radio.example.com/stream",
				ContentType: "",
			},
			wantURL: "https://radio.example.com/stream",
		},
		{
			name: "audio/* with mixed case content type",
			load: namespaces.MediaLoad{
				ContentID:   "https://example.com/track.flac",
				ContentType: "Audio/Flac",
			},
			wantURL: "https://example.com/track.flac",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewGeneric(quietLogger())
			intent, ok, err := p.Parse(&tc.load)
			if err != nil {
				t.Fatalf("Parse err: %v", err)
			}
			if !ok {
				t.Fatalf("Parse should have claimed payload")
			}
			if intent.Provider != "url" {
				t.Errorf("Provider = %q, want url", intent.Provider)
			}
			if intent.URL != tc.wantURL {
				t.Errorf("URL = %q, want %q", intent.URL, tc.wantURL)
			}
		})
	}
}

func TestGenericParserDefers(t *testing.T) {
	cases := []struct {
		name string
		load namespaces.MediaLoad
	}{
		{
			name: "empty LOAD",
			load: namespaces.MediaLoad{},
		},
		{
			name: "video contentType — not our problem",
			load: namespaces.MediaLoad{
				ContentID:   "https://example.com/video.mp4",
				ContentType: "video/mp4",
			},
		},
		{
			name: "empty contentType, non-URL contentId",
			load: namespaces.MediaLoad{
				ContentID:   "trackid-12345",
				ContentType: "",
			},
		},
		{
			name: "image content",
			load: namespaces.MediaLoad{
				ContentID:   "https://example.com/cover.png",
				ContentType: "image/png",
			},
		},
		{
			name: "http URL without host",
			load: namespaces.MediaLoad{
				ContentID:   "http:///broken",
				ContentType: "",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewGeneric(quietLogger())
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

func TestGenericParserName(t *testing.T) {
	p := NewGeneric(nil)
	if p.Name() != "url" {
		t.Errorf("Name = %q, want url", p.Name())
	}
}
