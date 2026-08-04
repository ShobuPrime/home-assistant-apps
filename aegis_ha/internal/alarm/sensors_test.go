package alarm

import (
	"slices"
	"testing"
	"testing/synctest"
	"time"
)

func TestAlwaysOnSensorTriggersWhileDisarmed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, stop := run(Config{ArmModes: []string{"away"}})
		defer stop()
		e.ConfigureSensors([]SensorConfig{{ID: "smoke", Name: "Smoke", AlwaysOn: true}}, nil)
		e.SensorEvent("smoke", SensorOpen)
		if got := e.Current().State; got != StateTriggered {
			t.Fatalf("always_on sensor should trigger while disarmed, got %s", got)
		}
	})
}

func TestImmediateSensorTriggersInstantlyWhenArmed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, stop := run(Config{ExitDelay: 0, EntryDelay: 30 * time.Second, ArmModes: []string{"away"}})
		defer stop()
		e.ConfigureSensors([]SensorConfig{{ID: "glass", Name: "Glassbreak", Immediate: true}}, nil)
		e.Arm("away", Actor{Name: "a"}, false)
		e.SensorEvent("glass", SensorOpen)
		if got := e.Current().State; got != StateTriggered {
			t.Fatalf("immediate sensor should skip entry delay, got %s", got)
		}
	})
}

func TestNormalSensorUsesEntryDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, stop := run(Config{ExitDelay: 0, EntryDelay: 30 * time.Second, ArmModes: []string{"away"}})
		defer stop()
		e.ConfigureSensors([]SensorConfig{{ID: "door", Name: "Door"}}, nil)
		e.Arm("away", Actor{Name: "a"}, false)
		e.SensorEvent("door", SensorOpen)
		if got := e.Current().State; got != StatePending {
			t.Fatalf("normal sensor should enter pending, got %s", got)
		}
		time.Sleep(31 * time.Second)
		synctest.Wait()
		if got := e.Current().State; got != StateTriggered {
			t.Fatalf("should trigger after entry delay, got %s", got)
		}
	})
}

func TestAutoBypassArmsAndDoesNotTrigger(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, stop := run(Config{ExitDelay: 0, ArmModes: []string{"away"}})
		defer stop()
		e.ConfigureSensors([]SensorConfig{{ID: "win", Name: "Window", AutoBypass: true}}, nil)
		e.SensorEvent("win", SensorOpen)
		if r := e.Arm("away", Actor{Name: "a"}, false); !r.Accepted {
			t.Fatalf("auto_bypass sensor must not block arming: %+v", r)
		}
		if !slices.Contains(e.Current().BypassedSensors, "Window") {
			t.Fatalf("window should be auto-bypassed: %v", e.Current().BypassedSensors)
		}
		// Re-trip while auto-bypassed → no trigger.
		e.SensorEvent("win", SensorOpen)
		if e.Current().State != StateArmedAway {
			t.Fatalf("auto-bypassed sensor should not trigger, state=%s", e.Current().State)
		}
	})
}

func TestAllowOpenArmsAndTriggersAfterCloseReopen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, stop := run(Config{ExitDelay: 0, EntryDelay: 0, ArmModes: []string{"away"}})
		defer stop()
		e.ConfigureSensors([]SensorConfig{{ID: "door", Name: "Door", AllowOpen: true}}, nil)
		e.SensorEvent("door", SensorOpen)
		if r := e.Arm("away", Actor{Name: "a"}, false); !r.Accepted {
			t.Fatalf("allow_open sensor must not block arming: %+v", r)
		}
		// Still open → not live yet (pendingClose), must not trigger.
		e.SensorEvent("door", SensorOpen)
		if e.Current().State != StateArmedAway {
			t.Fatalf("allow_open sensor should be inert until it closes, state=%s", e.Current().State)
		}
		// Close then reopen → now live → triggers.
		e.SensorEvent("door", SensorClosed)
		e.SensorEvent("door", SensorOpen)
		if e.Current().State != StateTriggered {
			t.Fatalf("allow_open sensor should trigger after close+reopen, state=%s", e.Current().State)
		}
	})
}

func TestManualBypassExcludesSensor(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, stop := run(Config{ExitDelay: 0, ArmModes: []string{"away"}})
		defer stop()
		e.ConfigureSensors([]SensorConfig{{ID: "mot", Name: "Motion"}}, nil)
		e.SensorEvent("mot", SensorOpen)
		e.SetBypass("mot", true)
		if !e.Current().ReadyToArm {
			t.Fatal("bypassed sensor should not block readiness")
		}
		if r := e.Arm("away", Actor{Name: "a"}, false); !r.Accepted {
			t.Fatalf("should arm with the only open sensor bypassed: %+v", r)
		}
		e.SensorEvent("mot", SensorOpen)
		if e.Current().State != StateArmedAway {
			t.Fatalf("bypassed sensor must not trigger, state=%s", e.Current().State)
		}
	})
}

func TestSensorModeRestriction(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, stop := run(Config{ExitDelay: 0, EntryDelay: 0, ArmModes: []string{"away", "home"}})
		defer stop()
		e.ConfigureSensors([]SensorConfig{{ID: "mot", Name: "Motion", Modes: []string{"away"}}}, nil)
		// Armed home: an away-only sensor must not trigger.
		e.Arm("home", Actor{Name: "a"}, false)
		e.SensorEvent("mot", SensorOpen)
		if e.Current().State != StateArmedHome {
			t.Fatalf("away-only sensor must not trigger in home mode, state=%s", e.Current().State)
		}
		// Disarm, re-arm away: now it triggers.
		e.SensorEvent("mot", SensorClosed)
		e.Disarm(Actor{Name: "a"})
		e.Arm("away", Actor{Name: "a"}, false)
		e.SensorEvent("mot", SensorOpen)
		if e.Current().State != StateTriggered {
			t.Fatalf("away-only sensor should trigger in away mode, state=%s", e.Current().State)
		}
	})
}

func TestSensorGroupDebounce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, stop := run(Config{ExitDelay: 0, EntryDelay: 0, ArmModes: []string{"away"}})
		defer stop()
		e.ConfigureSensors(
			[]SensorConfig{
				{ID: "a", Name: "MotionA", Immediate: true, Group: "g"},
				{ID: "b", Name: "MotionB", Immediate: true, Group: "g"},
			},
			[]SensorGroup{{Name: "g", EventCount: 2, Timeout: time.Minute}},
		)
		e.Arm("away", Actor{Name: "a"}, false)
		e.SensorEvent("a", SensorOpen) // 1/2 — no trigger
		if e.Current().State != StateArmedAway {
			t.Fatalf("single group hit should not trigger, state=%s", e.Current().State)
		}
		e.SensorEvent("b", SensorOpen) // 2/2 — trigger
		if e.Current().State != StateTriggered {
			t.Fatalf("group threshold should trigger, state=%s", e.Current().State)
		}
	})
}
