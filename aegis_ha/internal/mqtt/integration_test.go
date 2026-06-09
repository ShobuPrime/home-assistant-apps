package mqtt

import (
	"os"
	"testing"
	"time"

	"github.com/shobuprime/aegis_ha/internal/alarm"
	"github.com/shobuprime/aegis_ha/internal/store"
)

// TestIntegrationRoundTrip exercises the hand-rolled MQTT client against a
// real broker: it publishes a keypad command and asserts the alarm entity
// state flips on the retained state topic. Set MQTT_TEST_BROKER (host:port)
// to run; it is skipped otherwise so unit `go test` stays hermetic.
func TestIntegrationRoundTrip(t *testing.T) {
	broker := os.Getenv("MQTT_TEST_BROKER")
	if broker == "" {
		t.Skip("set MQTT_TEST_BROKER=host:port to run the MQTT integration test")
	}
	ctx := t.Context()

	st, err := store.Open(t.TempDir(), store.Policy{PINMin: 4, PINMax: 8, LockoutThreshold: 5, LockoutDuration: time.Minute})
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	_ = st.SetCode("1234")
	cfg := alarm.Config{ExitDelay: 0, ArmModes: []string{"away"}}
	eng := alarm.New(cfg, nil)
	go eng.Run(ctx)

	states := make(chan string, 16)
	sub := New(Options{Broker: broker, ClientID: "aegis_ha-it-sub"})
	_ = sub.Subscribe("aegis_ha/panel/state", func(m Message) { states <- string(m.Payload) })
	go sub.Run(ctx)

	bc := New(Options{Broker: broker, ClientID: "aegis_ha-it"})
	bridge := NewBridge(bc, eng, st, Config{Prefix: "aegis_ha", ArmModes: []string{"away"}, RequireCodeToDisarm: true}, cfg, nil)
	go bridge.Run(ctx)
	go bc.Run(ctx)

	waitConnected(t, bc)
	waitConnected(t, sub)
	// Give the bridge a moment to subscribe to its command topic.
	time.Sleep(500 * time.Millisecond)

	if err := sub.Publish("aegis_ha/panel/cmd", []byte(`{"action":"ARM_AWAY","code":"1234"}`), false); err != nil {
		t.Fatalf("publish cmd: %v", err)
	}
	expectState(t, states, "armed_away", 5*time.Second)

	if err := sub.Publish("aegis_ha/panel/cmd", []byte(`{"action":"DISARM","code":"1234"}`), false); err != nil {
		t.Fatalf("publish disarm: %v", err)
	}
	expectState(t, states, "disarmed", 5*time.Second)
}

func waitConnected(t *testing.T, c *Client) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if c.Connected() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("client did not connect within 5s")
}

func expectState(t *testing.T, ch <-chan string, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case s := <-ch:
			if s == want {
				return
			}
		case <-deadline:
			t.Fatalf("did not observe state %q within %s", want, timeout)
		}
	}
}
