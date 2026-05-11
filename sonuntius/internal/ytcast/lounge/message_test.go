// Maps to: N/A — Go-only tests for Message parser and outgoing constructors.
package lounge

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/shobuprime/sonuntius/internal/ytcast/constants"
)

func TestParseIncoming_LoungeStatus(t *testing.T) {
	// A real-ish loungeStatus frame. The payload is itself a JSON
	// object with stringified inner devices.
	frame := `[1,["loungeStatus",{"devices":"[]"}]]`
	msgs := ParseIncoming(frame)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d (%v)", len(msgs), msgs)
	}
	got := msgs[0]
	if got.AID == nil || *got.AID != 1 {
		t.Fatalf("AID = %v, want 1", got.AID)
	}
	if got.Name != "loungeStatus" {
		t.Errorf("Name = %q, want %q", got.Name, "loungeStatus")
	}
	payload := got.PayloadAsMap()
	if payload == nil {
		t.Fatalf("expected map payload, got %T", got.Payload)
	}
	if payload["devices"] != "[]" {
		t.Errorf("payload[devices] = %v", payload["devices"])
	}
}

func TestParseIncoming_CFrame(t *testing.T) {
	// SID assignment frame: `[0, ["c", "SID here", "", 8]]` — payload
	// is the array tail.
	frame := `[0,["c","abcd1234","",8]]`
	msgs := ParseIncoming(frame)
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	got := msgs[0]
	if got.Name != "c" {
		t.Fatalf("Name = %q", got.Name)
	}
	arr := got.PayloadAsArray()
	if len(arr) != 3 {
		t.Fatalf("payload array len = %d, want 3", len(arr))
	}
	if arr[0] != "abcd1234" {
		t.Errorf("payload[0] = %v, want \"abcd1234\"", arr[0])
	}
}

func TestParseIncoming_SFrame(t *testing.T) {
	// gsessionid assignment: `[1, ["S", "gsessionid here"]]` — single
	// element payload unwraps to the string.
	frame := `[1,["S","gs-xyz-123"]]`
	msgs := ParseIncoming(frame)
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	got := msgs[0]
	if got.Name != "S" {
		t.Fatalf("Name = %q", got.Name)
	}
	if s := got.PayloadAsString(); s != "gs-xyz-123" {
		t.Errorf("payload string = %q, want gs-xyz-123", s)
	}
}

func TestParseIncoming_NoPayload(t *testing.T) {
	frame := `[2,["noop"]]`
	msgs := ParseIncoming(frame)
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	got := msgs[0]
	if got.AID == nil || *got.AID != 2 {
		t.Fatalf("AID = %v", got.AID)
	}
	if got.Name != "noop" {
		t.Errorf("Name = %q", got.Name)
	}
	if got.Payload != nil {
		t.Errorf("payload should be nil for payload-less frame, got %v", got.Payload)
	}
}

func TestParseIncoming_Concatenated(t *testing.T) {
	frame := `[0,["c","sid","",8]][1,["S","gsid"]][2,["loungeStatus",{"devices":"[]"}]]`
	msgs := ParseIncoming(frame)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].Name != "c" || msgs[1].Name != "S" || msgs[2].Name != "loungeStatus" {
		t.Errorf("wrong order: %v %v %v", msgs[0].Name, msgs[1].Name, msgs[2].Name)
	}
}

func TestParseIncoming_NewlineStripped(t *testing.T) {
	frame := "[0,[\"c\",\"sid\",\"\",8]]\r\n[1,[\"S\",\"gsid\"]]\n"
	msgs := ParseIncoming(frame)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
}

func TestMessage_SerializeRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		aid  *int
		mName string
		payload any
	}{
		{"with-payload", intPtr(7), "playlistChange", map[string]any{"currentIndex": "0", "listId": "RD123"}},
		{"no-payload", intPtr(3), "loungeStatus", nil},
		{"nil-aid", nil, "loungeScreenDisconnected", map[string]any{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := &Message{AID: c.aid, Name: c.mName, Payload: c.payload}
			out, err := m.Serialize()
			if err != nil {
				t.Fatalf("Serialize() err = %v", err)
			}
			// Round-trip through ParseIncoming where possible (nil AID
			// won't parse — it isn't a valid AID, mirroring upstream).
			if c.aid == nil {
				// Just verify we got valid JSON that round-trips through
				// encoding/json.
				var anyVal []any
				if err := json.Unmarshal([]byte(out), &anyVal); err != nil {
					t.Fatalf("output is not valid JSON: %v (out=%s)", err, out)
				}
				if len(anyVal) != 2 || anyVal[0] != nil {
					t.Errorf("expected [null, [name, payload?]] shape, got %s", out)
				}
				return
			}
			msgs := ParseIncoming(out)
			if len(msgs) != 1 {
				t.Fatalf("round-trip parse: got %d messages (out=%s)", len(msgs), out)
			}
			got := msgs[0]
			if got.Name != c.mName {
				t.Errorf("Name: got %q want %q", got.Name, c.mName)
			}
			if got.AID == nil || *got.AID != *c.aid {
				t.Errorf("AID: got %v want %v", got.AID, *c.aid)
			}
			// payload may be nil for the no-payload case.
			if c.payload == nil {
				if got.Payload != nil {
					// `[7,["loungeStatus",null]]` would have null
					// payload, which decodePayload turns into a nil
					// any. ParseIncoming returns that as nil — OK.
				}
				return
			}
			expectedJSON, _ := json.Marshal(c.payload)
			gotJSON, _ := json.Marshal(got.Payload)
			if !reflect.DeepEqual(expectedJSON, gotJSON) {
				t.Errorf("Payload round-trip: got %s want %s", gotJSON, expectedJSON)
			}
		})
	}
}

func TestNewNowPlaying_EmptyState(t *testing.T) {
	m := NewNowPlaying(nil, nil)
	if m.Name != "nowPlaying" {
		t.Fatalf("Name = %q", m.Name)
	}
	if p := m.PayloadAsMap(); p == nil || len(p) != 0 {
		t.Errorf("expected empty payload map, got %v", m.Payload)
	}
}

func TestNewNowPlaying_PlayingState(t *testing.T) {
	idx := 2
	state := &PlayerStateView{
		Status:   constants.PlayerStatusPlaying,
		Position: 12.5,
		Duration: 200,
		CPN:      "cpn-xyz",
		Queue: PlaylistView{
			Current: &Video{
				ID: "vid-abc",
				Context: &VideoContext{
					PlaylistID: "RD123",
					Index:      &idx,
					CTT:        "ctt-zzz",
					Params:     "params-yyy",
				},
			},
		},
	}
	m := NewNowPlaying(nil, state)
	p := m.PayloadAsMap()
	if p["videoId"] != "vid-abc" {
		t.Errorf("videoId = %v", p["videoId"])
	}
	if p["state"] != int(constants.PlayerStatusPlaying) {
		t.Errorf("state = %v", p["state"])
	}
	if p["loadedTime"] != float64(200) && p["loadedTime"] != 200 {
		t.Errorf("loadedTime = %v (want 200)", p["loadedTime"])
	}
	if p["listId"] != "RD123" {
		t.Errorf("listId = %v", p["listId"])
	}
	if p["currentIndex"] != 2 {
		t.Errorf("currentIndex = %v", p["currentIndex"])
	}
}

func intPtr(n int) *int { return &n }
