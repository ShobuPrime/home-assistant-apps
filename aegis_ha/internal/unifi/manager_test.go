package unifi

import (
	"sync"
	"testing"
	"time"

	"github.com/shobuprime/aegis_ha/internal/alarm"
)

type fakePub struct {
	mu        sync.Mutex
	enabled   bool
	mode      string
	connected bool
	zones     map[string]string
	zoneState map[string]bool
}

func newFakePub() *fakePub {
	return &fakePub{zones: map[string]string{}, zoneState: map[string]bool{}}
}

func (f *fakePub) EnableProtect() { f.mu.Lock(); f.enabled = true; f.mu.Unlock() }
func (f *fakePub) AnnounceZone(id, name string) {
	f.mu.Lock()
	f.zones[id] = name
	f.mu.Unlock()
}
func (f *fakePub) PublishProtectStatus(mode string, connected bool) {
	f.mu.Lock()
	f.mode, f.connected = mode, connected
	f.mu.Unlock()
}
func (f *fakePub) PublishZone(id string, open bool) {
	f.mu.Lock()
	f.zoneState[id] = open
	f.mu.Unlock()
}

func runEngine(t *testing.T, cfg alarm.Config) *alarm.Engine {
	t.Helper()
	e := alarm.New(cfg, nil)
	go e.Run(t.Context())
	return e
}

func TestManagerDetectsGlobalMode(t *testing.T) {
	c := newMock(t, &mock{armProfilesGlobal: true})
	pub := newFakePub()
	m := NewManager(c, runEngine(t, alarm.Config{ArmModes: []string{"away"}}), pub, Config{PreferMode: "auto", PollInterval: time.Hour}, nil)

	m.detect(t.Context())
	pub.mu.Lock()
	defer pub.mu.Unlock()
	if pub.mode != string(ModeGlobal) || !pub.connected {
		t.Fatalf("want global+connected, got mode=%s connected=%v", pub.mode, pub.connected)
	}
}

func TestManagerPollFeedsOpenSensors(t *testing.T) {
	c := newMock(t, &mock{armProfilesOK: true})
	pub := newFakePub()
	eng := runEngine(t, alarm.Config{ArmModes: []string{"away"}})
	m := NewManager(c, eng, pub, Config{PreferMode: "auto", PollInterval: time.Hour, ExposeZones: true}, nil)

	m.poll(t.Context())

	if got := eng.Current().OpenSensors; len(got) != 1 || got[0] != "Front Door" {
		t.Fatalf("open sensors = %v, want [Front Door]", got)
	}
	if eng.Current().ReadyToArm {
		t.Fatal("ready_to_arm should be false with an open sensor")
	}
	pub.mu.Lock()
	defer pub.mu.Unlock()
	if pub.zones["s1"] != "Front Door" || !pub.zoneState["s1"] {
		t.Fatalf("zone not announced/published: zones=%v state=%v", pub.zones, pub.zoneState)
	}
}

func TestManagerSkipsZoneEntitiesWhenNotExposed(t *testing.T) {
	c := newMock(t, &mock{armProfilesOK: true})
	pub := newFakePub()
	eng := runEngine(t, alarm.Config{ArmModes: []string{"away"}})
	// ExposeZones defaults to false.
	m := NewManager(c, eng, pub, Config{PreferMode: "auto", PollInterval: time.Hour}, nil)

	m.poll(t.Context())

	// The engine still sees the sensor (for readiness/breach)...
	if got := eng.Current().OpenSensors; len(got) != 1 || got[0] != "Front Door" {
		t.Fatalf("engine should still get the sensor: %v", got)
	}
	// ...but no per-zone entity is published to HA (no duplicate of the
	// official UniFi Protect integration).
	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.zones) != 0 {
		t.Fatalf("no zone entities should be published when ExposeZones is false, got %v", pub.zones)
	}
}

func TestManagerBreachTriggersWhenArmed(t *testing.T) {
	c := newMock(t, &mock{armProfilesOK: true})
	pub := newFakePub()
	// Entry delay 0 so a breach goes straight to triggered.
	eng := runEngine(t, alarm.Config{ArmModes: []string{"away"}, ExitDelay: 0, EntryDelay: 0})
	m := NewManager(c, eng, pub, Config{PreferMode: "app-managed", PollInterval: time.Hour}, nil)

	// Arm the engine, then simulate the manager having observed the armed state.
	eng.Arm("away", alarm.Actor{Name: "test"}, true)
	m.lastState = alarm.StateArmedAway

	m.poll(t.Context()) // sensor s1 is open and newly-open while armed

	if got := eng.Current().State; got != alarm.StateTriggered {
		t.Fatalf("breach while armed should trigger, got %s", got)
	}
}
