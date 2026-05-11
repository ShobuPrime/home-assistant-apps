// Maps to: N/A — Go-only tests for DerivePlayerID.
package ma

import "testing"

func TestDerivePlayerID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bare slug", "media_player.living_room", "living_room"},
		{"slug with _2 disambiguator", "media_player.3rspk_a8e29151e187_2", "3rspk_a8e29151e187"},
		{"slug with _10 disambiguator", "media_player.kitchen_10", "kitchen"},
		{"slug that ends in digits but no underscore", "media_player.player1", "player1"},
		{"slug that ends in _word", "media_player.something_kitchen", "something_kitchen"},
		{"empty", "", ""},
		{"missing prefix is returned unchanged", "living_room_2", "living_room_2"},
		{"prefix-only is returned unchanged (no slug to strip)", "media_player.", "media_player."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DerivePlayerID(tc.in); got != tc.want {
				t.Errorf("DerivePlayerID(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
