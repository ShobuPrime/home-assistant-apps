package mqtt

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/shobuprime/aegis_ha/internal/alarm"
	"github.com/shobuprime/aegis_ha/internal/store"
)

func newTestBridge(t *testing.T, arming, disarm bool) (*Bridge, *alarm.Engine, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir(), store.Policy{PINMin: 4, PINMax: 8, LockoutThreshold: 5, LockoutDuration: time.Minute})
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if err := st.SetCode("1234"); err != nil {
		t.Fatalf("set code: %v", err)
	}
	cfg := alarm.Config{ExitDelay: 0, ArmModes: []string{"away", "home"}}
	eng := alarm.New(cfg, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go eng.Run(ctx)

	// Client is never connected; its Publish calls fail silently, which is
	// fine — we exercise the inbound command path only.
	client := New(Options{Broker: "127.0.0.1:1", ClientID: "test"})
	b := NewBridge(client, eng, st, Config{
		Prefix:              "aegis_ha",
		ArmModes:            []string{"away", "home"},
		CodeConfigured:      true, // newTestBridge sets a shared code
		RequireCodeToArm:    arming,
		RequireCodeToDisarm: disarm,
	}, cfg, nil)
	return b, eng, st
}

func cmd(action, code string) Message {
	payload, _ := json.Marshal(panelCommand{Action: action, Code: code})
	return Message{Topic: "aegis_ha/panel/cmd", Payload: payload}
}

func TestBridgeArmDisarmWithCode(t *testing.T) {
	b, eng, _ := newTestBridge(t, false, true)

	b.handlePanelCmd(cmd("ARM_AWAY", "1234"))
	if got := eng.Current().State; got != alarm.StateArmedAway {
		t.Fatalf("after arm want armed_away, got %s", got)
	}

	// Disarm requires a code; the wrong PIN must not disarm.
	b.handlePanelCmd(cmd("DISARM", "0000"))
	if got := eng.Current().State; got != alarm.StateArmedAway {
		t.Fatalf("wrong disarm code changed state to %s", got)
	}

	// Correct PIN disarms.
	b.handlePanelCmd(cmd("DISARM", "1234"))
	if got := eng.Current().State; got != alarm.StateDisarmed {
		t.Fatalf("correct disarm code did not disarm, state=%s", got)
	}
}

func TestBridgeArmWithoutCodeWhenNotRequired(t *testing.T) {
	b, eng, _ := newTestBridge(t, false, true)
	b.handlePanelCmd(cmd("ARM_HOME", "")) // arming_requires_code = false
	if got := eng.Current().State; got != alarm.StateArmedHome {
		t.Fatalf("anonymous arm should work, got %s", got)
	}
}

func TestBridgeNumberSetUpdatesEngine(t *testing.T) {
	b, eng, _ := newTestBridge(t, false, true)
	b.handleSet(Message{Topic: "aegis_ha/exit_delay/set", Payload: []byte("45")})
	// Arming now should enter the arming countdown rather than arm instantly.
	b.handlePanelCmd(cmd("ARM_AWAY", "1234"))
	if got := eng.Current().State; got != alarm.StateArming {
		t.Fatalf("after exit_delay=45 arm should be arming, got %s", got)
	}
}

func TestBridgePanicButton(t *testing.T) {
	b, eng, _ := newTestBridge(t, false, true)
	b.handleSet(Message{Topic: "aegis_ha/panic/set", Payload: []byte("PRESS")})
	if got := eng.Current().State; got != alarm.StateTriggered {
		t.Fatalf("panic should trigger, got %s", got)
	}
}

func TestPanelDiscoveryNoCodeOmitsRemoteCode(t *testing.T) {
	// A bridge with no shared code configured must NOT advertise a PIN field,
	// so Home Assistant arms/disarms without prompting (the fix for a blank
	// code making the panel un-disarmable).
	b, _, _ := newTestBridge(t, false, false)
	b.cfg.CodeConfigured = false
	m := b.panelDiscovery()
	var p map[string]any
	if err := json.Unmarshal(m.payload, &p); err != nil {
		t.Fatalf("discovery payload: %v", err)
	}
	if _, hasCode := p["code"]; hasCode {
		t.Errorf("no-code panel must not set a code field, got %v", p["code"])
	}
	if _, hasReq := p["code_disarm_required"]; hasReq {
		t.Errorf("no-code panel must not set code_disarm_required")
	}
}

func TestPanelDiscoveryHasRemoteCodeContract(t *testing.T) {
	b, _, _ := newTestBridge(t, true, true)
	m := b.panelDiscovery()
	var p map[string]any
	if err := json.Unmarshal(m.payload, &p); err != nil {
		t.Fatalf("discovery payload: %v", err)
	}
	if p["code"] != "REMOTE_CODE" {
		t.Errorf("code = %v, want REMOTE_CODE (mandatory for PIN forwarding)", p["code"])
	}
	if p["command_template"] != `{"action":"{{action}}","code":"{{code}}"}` {
		t.Errorf("command_template missing/forwarding-broken: %v", p["command_template"])
	}
	if p["code_arm_required"] != true || p["code_disarm_required"] != true {
		t.Errorf("code_*_required not propagated: %+v", p)
	}
	if m.topic != "homeassistant/alarm_control_panel/aegis_ha/panel/config" {
		t.Errorf("discovery topic = %s", m.topic)
	}
	feats, _ := p["supported_features"].([]any)
	if len(feats) == 0 {
		t.Errorf("supported_features empty: %v", p["supported_features"])
	}
}
