// Maps to: N/A — Go-only tests for the sonuntius Player adapter's
// resolveIntent provider mapping.
package main

import (
	"testing"

	"github.com/shobuprime/sonuntius/internal/events"
	"github.com/shobuprime/sonuntius/internal/ytcast/types"
)

func TestResolveIntent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		video   types.Video
		want    events.PlayIntent
	}{
		{
			name: "YouTube Music app — ytmusic provider",
			video: types.Video{
				ID:     "abc12345678",
				Client: types.Client{Theme: "m"},
			},
			want: events.PlayIntent{
				Provider: "ytmusic",
				TrackID:  "abc12345678",
				Source:   "yt-cast",
			},
		},
		{
			name: "YouTube classic — url provider with watch URL",
			video: types.Video{
				ID:     "bp4_7T9J6Fg",
				Client: types.Client{Theme: "cl"},
			},
			want: events.PlayIntent{
				Provider: "url",
				URL:      "https://www.youtube.com/watch?v=bp4_7T9J6Fg",
				TrackID:  "bp4_7T9J6Fg",
				Source:   "yt-cast",
			},
		},
		{
			name: "Unknown surface — passthrough theme as provider",
			video: types.Video{
				ID:     "xyz",
				Client: types.Client{Theme: "kids"},
			},
			want: events.PlayIntent{
				Provider: "kids",
				TrackID:  "xyz",
				Source:   "yt-cast",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveIntent(tc.video, "yt-cast")
			if got == nil {
				t.Fatalf("resolveIntent returned nil")
			}
			if got.Provider != tc.want.Provider {
				t.Errorf("Provider = %q, want %q", got.Provider, tc.want.Provider)
			}
			if got.TrackID != tc.want.TrackID {
				t.Errorf("TrackID = %q, want %q", got.TrackID, tc.want.TrackID)
			}
			if got.URL != tc.want.URL {
				t.Errorf("URL = %q, want %q", got.URL, tc.want.URL)
			}
			if got.Source != tc.want.Source {
				t.Errorf("Source = %q, want %q", got.Source, tc.want.Source)
			}
		})
	}
}
