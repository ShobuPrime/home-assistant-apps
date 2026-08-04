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

func TestArmWebhookFires(t *testing.T) {
	const (
		dis  = alarm.StateDisarmed
		arm  = alarm.StateArming
		away = alarm.StateArmedAway
		trg  = alarm.StateTriggered
	)
	cases := []struct {
		name          string
		prev, cur     alarm.State
		atStart, want bool
	}{
		{"app: commit after exit delay", arm, away, false, true},
		{"app: instant arm (exit 0)", dis, away, false, true},
		{"app: not at arming start", dis, arm, false, false},
		{"app: no re-fire on trigger restore", trg, away, false, false},
		{"unifi: fire at arming start", dis, arm, true, true},
		{"unifi: instant arm fires", dis, away, true, true},
		{"unifi: no re-fire on commit", arm, away, true, false},
		{"unifi: no re-fire on trigger restore", trg, away, true, false},
	}
	for _, c := range cases {
		if got := armWebhookFires(c.prev, c.cur, c.atStart); got != c.want {
			t.Errorf("%s: armWebhookFires(%s,%s,%v)=%v want %v", c.name, c.prev, c.cur, c.atStart, got, c.want)
		}
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

func TestManagerMirrorsExternalProtectArm(t *testing.T) {
	// Read-sync only operates in Local mode (armMode.status is meaningful
	// there; Global mode does not expose the arm state).
	mk := &mock{armProfilesOK: true, nvrArm: "disabled"}
	c := newMock(t, mk)
	pub := newFakePub()
	eng := runEngine(t, alarm.Config{ArmModes: []string{"away"}, ExitDelay: 0})
	m := NewManager(c, eng, pub, Config{PreferMode: "auto", PollInterval: time.Hour, ArmModes: []string{"away"}}, nil)
	m.detect(t.Context()) // local mode → read-sync active

	// First observation establishes the baseline and must not act.
	m.syncArmState(t.Context())
	if got := eng.Current().State; got != alarm.StateDisarmed {
		t.Fatalf("baseline sync should not change state, got %s", got)
	}

	// Armed from the UniFi Protect app → AegisHA mirrors to armed_away.
	mk.nvrArm = "armed"
	m.syncArmState(t.Context())
	if got := eng.Current().State; got != alarm.StateArmedAway {
		t.Fatalf("external Protect arm should mirror to armed_away, got %s", got)
	}
	if cb := eng.Current().ChangedBy; cb != mirrorActor {
		t.Fatalf("mirror arm changed_by = %q, want %q", cb, mirrorActor)
	}

	// Disarmed from the app → AegisHA mirrors back to disarmed.
	mk.nvrArm = "disabled"
	m.syncArmState(t.Context())
	if got := eng.Current().State; got != alarm.StateDisarmed {
		t.Fatalf("external Protect disarm should mirror to disarmed, got %s", got)
	}
}

// TestManagerMirrorClearsTriggered confirms that disarming in the UniFi app
// clears an active AegisHA alarm (the triggered state must mirror to disarmed).
func TestManagerMirrorClearsTriggered(t *testing.T) {
	mk := &mock{armProfilesOK: true, nvrArm: "breach"}
	c := newMock(t, mk)
	eng := runEngine(t, alarm.Config{ArmModes: []string{"away"}, ExitDelay: 0, EntryDelay: 0, TriggerTime: 0})
	m := NewManager(c, eng, newFakePub(), Config{PreferMode: "auto", PollInterval: time.Hour, ArmModes: []string{"away"}}, nil)
	m.detect(t.Context())

	// Put AegisHA into triggered (panic) and baseline the observed Protect arm.
	eng.Trigger(true, alarm.Actor{Name: "test"})
	m.syncArmState(t.Context()) // baseline: observes "breach", takes no action
	if eng.Current().State != alarm.StateTriggered {
		t.Fatalf("precondition: want triggered, got %s", eng.Current().State)
	}

	// Disarm natively in UniFi → AegisHA's triggered alarm must clear.
	mk.nvrArm = "disabled"
	m.syncArmState(t.Context())
	if got := eng.Current().State; got != alarm.StateDisarmed {
		t.Fatalf("native disarm should clear triggered, got %s", got)
	}
}

// TestManagerAppManagedSkipsArmSync confirms app-managed mode opts out of
// mirroring Protect's arm state (AegisHA is the sole source of truth).
func TestManagerAppManagedSkipsArmSync(t *testing.T) {
	mk := &mock{nvrArm: "armed"}
	c := newMock(t, mk)
	eng := runEngine(t, alarm.Config{ArmModes: []string{"away"}, ExitDelay: 0})
	m := NewManager(c, eng, newFakePub(), Config{PreferMode: "app-managed", PollInterval: time.Hour, ArmModes: []string{"away"}}, nil)

	m.syncArmState(t.Context())
	m.syncArmState(t.Context())
	if got := eng.Current().State; got != alarm.StateDisarmed {
		t.Fatalf("app-managed mode should ignore Protect arm state, got %s", got)
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
