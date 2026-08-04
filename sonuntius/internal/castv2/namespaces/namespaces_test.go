// Maps to: N/A — Go-only tests for the per-namespace handlers.
//
// Each handler test follows the same pattern: feed in a JSON payload
// representative of what real senders emit, assert the Reply.Payload
// decodes back to the expected JSON shape. We avoid byte-exact comparison
// because field ordering in encoding/json is stable but not guaranteed by
// the language spec.
package namespaces

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestConnectionConnectClose(t *testing.T) {
	h := NewConnection(newTestLogger())
	reply, err := h.Handle(context.Background(), "client-1", json.RawMessage(`{"type":"CONNECT"}`))
	if err != nil {
		t.Fatalf("Handle CONNECT: %v", err)
	}
	if !reply.IsEmpty() {
		t.Errorf("CONNECT should produce no reply, got %q", reply.Payload)
	}
	if !h.IsOpen("client-1") {
		t.Error("IsOpen(client-1) = false after CONNECT")
	}

	_, _ = h.Handle(context.Background(), "client-1", json.RawMessage(`{"type":"CLOSE"}`))
	if h.IsOpen("client-1") {
		t.Error("IsOpen(client-1) = true after CLOSE")
	}
}

func TestConnectionMalformedPayload(t *testing.T) {
	h := NewConnection(newTestLogger())
	reply, err := h.Handle(context.Background(), "c", json.RawMessage(`not-json`))
	if err != nil {
		t.Fatalf("malformed payload err: %v", err)
	}
	if !reply.IsEmpty() {
		t.Error("malformed payload yielded a reply")
	}
}

func TestHeartbeatPingPong(t *testing.T) {
	h := NewHeartbeat("receiver-0", newTestLogger())
	reply, err := h.Handle(context.Background(), "client-1", json.RawMessage(`{"type":"PING"}`))
	if err != nil {
		t.Fatalf("Handle PING: %v", err)
	}
	if reply.IsEmpty() {
		t.Fatal("PING produced no reply")
	}
	var env struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(reply.Payload, &env); err != nil {
		t.Fatalf("decode PONG: %v", err)
	}
	if env.Type != "PONG" {
		t.Errorf("PONG type = %q", env.Type)
	}
	if reply.DestinationID != "client-1" {
		t.Errorf("DestinationID = %q want client-1", reply.DestinationID)
	}

	// PONG from sender records lastPong.
	_, _ = h.Handle(context.Background(), "client-1", json.RawMessage(`{"type":"PONG"}`))
	if h.LastPong("client-1").IsZero() {
		t.Error("LastPong not recorded")
	}
}

func TestHeartbeatEmitPing(t *testing.T) {
	h := NewHeartbeat("receiver-0", newTestLogger())
	r := h.EmitPing("client-7")
	if r.DestinationID != "client-7" {
		t.Errorf("DestinationID = %q", r.DestinationID)
	}
	if string(r.Payload) != `{"type":"PING"}` {
		t.Errorf("Payload = %q", r.Payload)
	}
}

func TestReceiverLaunchAndGetStatus(t *testing.T) {
	h := NewReceiver("Sonuntius (Tidal)", newTestLogger())
	reply, err := h.Handle(context.Background(), "client-1",
		json.RawMessage(`{"type":"LAUNCH","requestId":42,"appId":"CC1AD845"}`))
	if err != nil {
		t.Fatalf("Handle LAUNCH: %v", err)
	}
	if reply.IsEmpty() {
		t.Fatal("LAUNCH produced no reply")
	}
	st := decodeReceiverStatus(t, reply.Payload)
	if st.Type != "RECEIVER_STATUS" {
		t.Errorf("type = %q", st.Type)
	}
	if st.RequestID != 42 {
		t.Errorf("requestId = %d", st.RequestID)
	}
	if len(st.Status.Applications) != 1 {
		t.Fatalf("applications len = %d want 1", len(st.Status.Applications))
	}
	app := st.Status.Applications[0]
	if app.AppID != "CC1AD845" {
		t.Errorf("appId = %q", app.AppID)
	}
	if app.DisplayName != "Sonuntius (Tidal)" {
		t.Errorf("displayName = %q", app.DisplayName)
	}
	if app.SessionID == "" {
		t.Error("sessionId is empty")
	}

	// GET_STATUS should return the same launched app.
	reply, err = h.Handle(context.Background(), "client-1",
		json.RawMessage(`{"type":"GET_STATUS","requestId":7}`))
	if err != nil {
		t.Fatalf("Handle GET_STATUS: %v", err)
	}
	st = decodeReceiverStatus(t, reply.Payload)
	if st.RequestID != 7 {
		t.Errorf("requestId = %d want 7", st.RequestID)
	}
	if len(st.Status.Applications) != 1 {
		t.Errorf("GET_STATUS lost the launched app")
	}
}

func TestReceiverStop(t *testing.T) {
	h := NewReceiver("Sonuntius (Tidal)", newTestLogger())
	_, _ = h.Handle(context.Background(), "c",
		json.RawMessage(`{"type":"LAUNCH","requestId":1,"appId":"X"}`))
	reply, _ := h.Handle(context.Background(), "c",
		json.RawMessage(`{"type":"STOP","requestId":2,"sessionId":"x"}`))
	st := decodeReceiverStatus(t, reply.Payload)
	if len(st.Status.Applications) != 0 {
		t.Errorf("STOP did not clear applications: %d", len(st.Status.Applications))
	}
}

func TestReceiverSetVolume(t *testing.T) {
	h := NewReceiver("S", newTestLogger())
	_, _ = h.Handle(context.Background(), "c",
		json.RawMessage(`{"type":"SET_VOLUME","requestId":3,"volume":{"level":0.42,"muted":true}}`))
	reply, _ := h.Handle(context.Background(), "c",
		json.RawMessage(`{"type":"GET_STATUS","requestId":4}`))
	st := decodeReceiverStatus(t, reply.Payload)
	if st.Status.Volume.Level != 0.42 {
		t.Errorf("volume level = %v want 0.42", st.Status.Volume.Level)
	}
	if !st.Status.Volume.Muted {
		t.Error("volume muted = false want true")
	}
}

func TestMediaLoadInvokesParserChain(t *testing.T) {
	// First parser claims; second should never run.
	p1 := &fakeParser{name: "first", claim: true, intent: ParsedIntent{
		Provider: "tidal", TrackID: "12345",
	}}
	p2 := &fakeParser{name: "second", claim: true, intent: ParsedIntent{
		Provider: "should-not-run",
	}}
	var got ParsedIntent
	var fired bool
	h := NewMedia([]Parser{p1, p2}, func(ctx context.Context, src string, in ParsedIntent) {
		fired = true
		got = in
	}, newTestLogger())

	loadPayload := `{
        "type":"LOAD",
        "requestId":1,
        "media":{
            "contentId":"https://example/track/12345",
            "contentType":"audio/mp4",
            "customData":{"tidal":{"trackId":"12345"}}
        }
    }`
	reply, err := h.Handle(context.Background(), "client-1", json.RawMessage(loadPayload))
	if err != nil {
		t.Fatalf("Handle LOAD: %v", err)
	}
	if !fired {
		t.Fatal("intent handler not fired")
	}
	if got.Provider != "tidal" || got.TrackID != "12345" {
		t.Errorf("intent = %+v", got)
	}
	if p1.calls != 1 {
		t.Errorf("first parser calls = %d want 1", p1.calls)
	}
	if p2.calls != 0 {
		t.Errorf("second parser ran despite first claiming: %d calls", p2.calls)
	}

	// Reply should be MEDIA_STATUS with playerState=PLAYING.
	var env struct {
		Type   string `json:"type"`
		Status []struct {
			PlayerState string `json:"playerState"`
		} `json:"status"`
	}
	if err := json.Unmarshal(reply.Payload, &env); err != nil {
		t.Fatalf("decode MEDIA_STATUS: %v", err)
	}
	if env.Type != "MEDIA_STATUS" || env.Status[0].PlayerState != "PLAYING" {
		t.Errorf("MEDIA_STATUS shape = %+v", env)
	}
}

func TestMediaLogOnlyParserNeverClaims(t *testing.T) {
	p := NewLogOnlyParser(newTestLogger())
	_, ok, err := p.Parse(&MediaLoad{ContentID: "x"})
	if ok || err != nil {
		t.Errorf("LogOnly Parse: ok=%v err=%v want ok=false err=nil", ok, err)
	}
	if p.Name() != "logonly" {
		t.Errorf("Name = %q", p.Name())
	}
}

func TestMediaPauseStop(t *testing.T) {
	h := NewMedia(nil, nil, newTestLogger())
	reply, _ := h.Handle(context.Background(), "c",
		json.RawMessage(`{"type":"PAUSE","requestId":1}`))
	var env mediaStatusEnv
	_ = json.Unmarshal(reply.Payload, &env)
	if env.Status[0].PlayerState != "PAUSED" {
		t.Errorf("PAUSE -> playerState = %q", env.Status[0].PlayerState)
	}

	reply, _ = h.Handle(context.Background(), "c",
		json.RawMessage(`{"type":"STOP","requestId":2}`))
	_ = json.Unmarshal(reply.Payload, &env)
	if env.Status[0].PlayerState != "IDLE" {
		t.Errorf("STOP -> playerState = %q", env.Status[0].PlayerState)
	}
}

// ---------- helpers ----------

type fakeParser struct {
	name   string
	claim  bool
	intent ParsedIntent
	calls  int
}

func (p *fakeParser) Name() string { return p.name }
func (p *fakeParser) Parse(load *MediaLoad) (ParsedIntent, bool, error) {
	p.calls++
	if p.claim {
		return p.intent, true, nil
	}
	return ParsedIntent{}, false, nil
}

type receiverStatusEnv struct {
	Type      string `json:"type"`
	RequestID int64  `json:"requestId"`
	Status    struct {
		Applications []struct {
			AppID       string `json:"appId"`
			DisplayName string `json:"displayName"`
			SessionID   string `json:"sessionId"`
		} `json:"applications"`
		Volume struct {
			Level float64 `json:"level"`
			Muted bool    `json:"muted"`
		} `json:"volume"`
	} `json:"status"`
}

func decodeReceiverStatus(t *testing.T, b []byte) receiverStatusEnv {
	t.Helper()
	var env receiverStatusEnv
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatalf("decode RECEIVER_STATUS: %v: %s", err, string(b))
	}
	return env
}

type mediaStatusEnv struct {
	Status []struct {
		PlayerState string `json:"playerState"`
	} `json:"status"`
}
