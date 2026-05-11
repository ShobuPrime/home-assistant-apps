// Maps to: N/A — Go-only tests for MatchPlayer / slugify.
package ma

import "testing"

func TestMatchPlayer(t *testing.T) {
	t.Parallel()
	players := []PlayerInfo{
		{PlayerID: "ma_4f3a", DisplayName: "3RSPK", Name: "3RSPK"},
		{PlayerID: "snapcast_living_room", DisplayName: "Living Room"},
		{PlayerID: "kitchen", DisplayName: "Kitchen"},
		{PlayerID: "ma_aa11bb22cc33dd44", DisplayName: "3rspk-a8e29151e187"},
	}
	tests := []struct {
		name       string
		entityID   string
		wantPlayer string
		wantRule   string
	}{
		{
			name:       "exact match on player_id",
			entityID:   "media_player.kitchen",
			wantPlayer: "kitchen",
			wantRule:   "exact_player_id",
		},
		{
			name:       "stripped _N matches player_id",
			entityID:   "media_player.kitchen_2",
			wantPlayer: "kitchen",
			wantRule:   "stripped_player_id",
		},
		{
			name:       "display_name slug match",
			entityID:   "media_player.3rspk",
			wantPlayer: "ma_4f3a",
			wantRule:   "display_name_slug",
		},
		{
			name:       "display_name slug stripped match (dash->underscore)",
			entityID:   "media_player.3rspk_a8e29151e187_2",
			wantPlayer: "ma_aa11bb22cc33dd44",
			wantRule:   "display_name_slug_stripped",
		},
		{
			name:     "no match returns empty",
			entityID: "media_player.does_not_exist",
			wantRule: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, rule := MatchPlayer(players, tc.entityID)
			if rule != tc.wantRule {
				t.Errorf("rule = %q, want %q (got player=%q)", rule, tc.wantRule, p.PlayerID)
			}
			if tc.wantRule != "" && p.PlayerID != tc.wantPlayer {
				t.Errorf("player_id = %q, want %q", p.PlayerID, tc.wantPlayer)
			}
		})
	}
}

func TestSlugify(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"3RSPK":              "3rspk",
		"Living Room":        "living_room",
		"3rspk-a8e29151e187": "3rspk_a8e29151e187",
		"  spaces  ":         "spaces",
		"!!!":                "",
		"alpha":              "alpha",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
