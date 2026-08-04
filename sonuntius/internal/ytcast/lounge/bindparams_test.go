// Maps to: N/A — Go-only tests for BindParams AID/GSN arithmetic.
package lounge

import (
	"errors"
	"net/url"
	"strconv"
	"testing"

	"github.com/shobuprime/sonuntius/internal/ytcast/yterrors"
)

func newTestBindParams() *BindParams {
	return NewBindParams(BindParamsInitOptions{
		Theme:      "cl",
		DeviceID:   "device-1",
		ScreenName: "test-screen",
		ScreenApp:  "ytcr",
		Brand:      "Generic",
		Model:      "SmartTV",
	})
}

func TestBindParams_Defaults(t *testing.T) {
	bp := newTestBindParams()
	if bp.Device != "LOUNGE_SCREEN" {
		t.Errorf("Device = %q", bp.Device)
	}
	if bp.VER != 8 {
		t.Errorf("VER = %d", bp.VER)
	}
	if bp.AID != 3 {
		t.Errorf("AID = %d, want 3", bp.AID)
	}
	if bp.RID < 41000 || bp.RID > 49999 {
		t.Errorf("RID = %d, want in [41000, 49999]", bp.RID)
	}
}

func TestBindParams_ToQueryString_MissingFields(t *testing.T) {
	bp := newTestBindParams()
	_, err := bp.ToQueryString(QueryStringTypeRPC, nil)
	if err == nil {
		t.Fatalf("expected error for missing loungeIdToken/SID/gsessionid")
	}
	if !errors.Is(err, yterrors.ErrIncompleteAPIData) {
		t.Errorf("err is not IncompleteAPIDataError: %v", err)
	}
}

func TestBindParams_ToQueryString_InitSession_BumpsRID(t *testing.T) {
	bp := newTestBindParams()
	bp.LoungeIDToken = "ltoken"
	startRID := bp.RID
	qs, err := bp.ToQueryString(QueryStringTypeInitSession, nil)
	if err != nil {
		t.Fatalf("ToQueryString init: %v", err)
	}
	if bp.RID != startRID+1 {
		t.Errorf("RID not bumped: was %d, now %d", startRID, bp.RID)
	}
	parsed, err := url.ParseQuery(qs)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	if parsed.Get("RID") != strconv.Itoa(startRID) {
		t.Errorf("RID in query = %q, want %d", parsed.Get("RID"), startRID)
	}
	if parsed.Get("CVER") != "1" {
		t.Errorf("CVER = %q", parsed.Get("CVER"))
	}
	if parsed.Get("deviceInfo") == "" {
		t.Errorf("deviceInfo missing")
	}
}

func TestBindParams_ToQueryString_SendMessage_BumpsBoth(t *testing.T) {
	bp := newTestBindParams()
	bp.LoungeIDToken = "ltoken"
	bp.SID = "sid"
	bp.GSessionID = "gs"

	startRID := bp.RID
	startAID := bp.AID

	qs, err := bp.ToQueryString(QueryStringTypeSendMessage, nil)
	if err != nil {
		t.Fatalf("ToQueryString sendMessage: %v", err)
	}
	if bp.RID != startRID+1 {
		t.Errorf("RID not bumped: was %d, now %d", startRID, bp.RID)
	}
	if bp.AID != startAID+1 {
		t.Errorf("AID not bumped: was %d, now %d", startAID, bp.AID)
	}
	parsed, _ := url.ParseQuery(qs)
	if parsed.Get("AID") != strconv.Itoa(startAID) {
		t.Errorf("AID in query = %q, want %d", parsed.Get("AID"), startAID)
	}
	if parsed.Get("SID") != "sid" {
		t.Errorf("SID = %q", parsed.Get("SID"))
	}
}

func TestBindParams_ToQueryString_SendMessage_WithSuppliedAID(t *testing.T) {
	bp := newTestBindParams()
	bp.LoungeIDToken = "ltoken"
	bp.SID = "sid"
	bp.GSessionID = "gs"
	bp.AID = 5

	// Supplied AID is larger than bp.AID — bp.AID should jump up to it.
	suppliedAID := 9
	_, err := bp.ToQueryString(QueryStringTypeSendMessage, &suppliedAID)
	if err != nil {
		t.Fatalf("ToQueryString: %v", err)
	}
	if bp.AID != 9 {
		t.Errorf("AID after supplied=9: got %d, want 9 (no post-bump because AID was supplied)", bp.AID)
	}
}

func TestBindParams_ToQueryString_RPC_NoMutation(t *testing.T) {
	bp := newTestBindParams()
	bp.LoungeIDToken = "ltoken"
	bp.SID = "sid"
	bp.GSessionID = "gs"
	startRID := bp.RID
	startAID := bp.AID

	qs, err := bp.ToQueryString(QueryStringTypeRPC, nil)
	if err != nil {
		t.Fatalf("ToQueryString rpc: %v", err)
	}
	if bp.RID != startRID || bp.AID != startAID {
		t.Errorf("rpc query mutated AID/RID: AID %d→%d RID %d→%d", startAID, bp.AID, startRID, bp.RID)
	}
	parsed, _ := url.ParseQuery(qs)
	if parsed.Get("RID") != "rpc" {
		t.Errorf("rpc RID = %q, want 'rpc'", parsed.Get("RID"))
	}
	if parsed.Get("TYPE") != "xmlhttp" {
		t.Errorf("rpc TYPE = %q", parsed.Get("TYPE"))
	}
}

func TestBindParams_UpdateWithMessage_PinsSID(t *testing.T) {
	bp := newTestBindParams()
	aid := 0
	cmd := &Message{AID: &aid, Name: "c", Payload: []any{"sid-from-c", "", float64(8)}}
	bp.UpdateWithMessage(cmd)
	if bp.SID != "sid-from-c" {
		t.Errorf("SID = %q, want sid-from-c", bp.SID)
	}
}

func TestBindParams_UpdateWithMessage_PinsGSessionID(t *testing.T) {
	bp := newTestBindParams()
	aid := 1
	cmd := &Message{AID: &aid, Name: "S", Payload: "gs-value"}
	bp.UpdateWithMessage(cmd)
	if bp.GSessionID != "gs-value" {
		t.Errorf("GSessionID = %q", bp.GSessionID)
	}
}

func TestBindParams_UpdateWithMessage_AIDMonotonic(t *testing.T) {
	bp := newTestBindParams()
	// AID can only increase, never decrease.
	bp.AID = 10
	aid := 5
	bp.UpdateWithMessage(&Message{AID: &aid, Name: "noop"})
	if bp.AID != 10 {
		t.Errorf("AID decreased from 10 to %d", bp.AID)
	}
	aid = 25
	bp.UpdateWithMessage(&Message{AID: &aid, Name: "noop"})
	if bp.AID != 25 {
		t.Errorf("AID = %d, want 25", bp.AID)
	}
}

func TestBindParams_Reset_RestoresDefaults(t *testing.T) {
	bp := newTestBindParams()
	bp.LoungeIDToken = "ltoken"
	bp.SID = "sid"
	bp.GSessionID = "gs"
	bp.AID = 42

	bp.Reset()
	if bp.LoungeIDToken != "" || bp.SID != "" || bp.GSessionID != "" {
		t.Errorf("Reset() did not clear tokens: %+v", bp)
	}
	if bp.AID != 3 {
		t.Errorf("Reset() AID = %d, want 3", bp.AID)
	}
	if bp.RID < 41000 || bp.RID > 49999 {
		t.Errorf("Reset() RID = %d, want in [41000, 49999]", bp.RID)
	}
}

// Ensure many ToQueryString iterations under sendMessage produce AID
// values without wraparound or off-by-one (the load-bearing arithmetic).
func TestBindParams_ToQueryString_SendMessage_AIDProgression(t *testing.T) {
	bp := newTestBindParams()
	bp.LoungeIDToken = "ltoken"
	bp.SID = "sid"
	bp.GSessionID = "gs"
	bp.AID = 3

	for i := range 100 {
		qs, err := bp.ToQueryString(QueryStringTypeSendMessage, nil)
		if err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
		parsed, _ := url.ParseQuery(qs)
		got, _ := strconv.Atoi(parsed.Get("AID"))
		want := 3 + i
		if got != want {
			t.Fatalf("iter %d: AID = %d, want %d", i, got, want)
		}
	}
	if bp.AID != 103 {
		t.Errorf("final AID = %d, want 103", bp.AID)
	}
}
