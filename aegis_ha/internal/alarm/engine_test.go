package alarm

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

// run starts an engine inside the current synctest bubble and returns it
// plus a stop func that cancels Run and waits for it to exit.
func run(cfg Config) (*Engine, func()) {
	e := New(cfg, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go e.Run(ctx)
	return e, func() {
		cancel()
		synctest.Wait()
	}
}

func TestExitDelayThenArm(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, stop := run(Config{ExitDelay: 30 * time.Second, ArmModes: []string{"away"}})
		defer stop()

		r := e.Arm("away", Actor{Name: "anthony"}, false)
		if !r.Accepted || r.Snapshot.State != StateArming {
			t.Fatalf("arm: accepted=%v state=%s", r.Accepted, r.Snapshot.State)
		}

		time.Sleep(29 * time.Second)
		synctest.Wait()
		if got := e.Current().State; got != StateArming {
			t.Fatalf("at 29s want arming, got %s", got)
		}

		time.Sleep(2 * time.Second)
		synctest.Wait()
		if got := e.Current().State; got != StateArmedAway {
			t.Fatalf("after exit delay want armed_away, got %s", got)
		}
	})
}

func TestImmediateArmWhenNoExitDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, stop := run(Config{ExitDelay: 0, ArmModes: []string{"away", "home"}})
		defer stop()

		r := e.Arm("home", Actor{Name: "anthony"}, false)
		if !r.Accepted || r.Snapshot.State != StateArmedHome {
			t.Fatalf("immediate arm: accepted=%v state=%s", r.Accepted, r.Snapshot.State)
		}
	})
}

func TestDisarmFromArming(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, stop := run(Config{ExitDelay: 60 * time.Second, ArmModes: []string{"away"}})
		defer stop()

		e.Arm("away", Actor{Name: "a"}, false)
		r := e.Disarm(Actor{Name: "a"})
		if !r.Accepted || r.Snapshot.State != StateDisarmed {
			t.Fatalf("disarm: %+v", r)
		}
		// The cancelled exit timer must not later flip us to armed.
		time.Sleep(120 * time.Second)
		synctest.Wait()
		if got := e.Current().State; got != StateDisarmed {
			t.Fatalf("cancelled timer fired: state=%s", got)
		}
	})
}

func TestPanicTriggerResolvesToPriorMode(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, stop := run(Config{ExitDelay: 0, TriggerTime: 60 * time.Second, ArmModes: []string{"away"}})
		defer stop()

		e.Arm("away", Actor{Name: "a"}, false)
		r := e.Trigger(true, Actor{Name: "panic"})
		if !r.Accepted || r.Snapshot.State != StateTriggered {
			t.Fatalf("trigger: %+v", r)
		}
		time.Sleep(61 * time.Second)
		synctest.Wait()
		if got := e.Current().State; got != StateArmedAway {
			t.Fatalf("after trigger_time want armed_away (restore), got %s", got)
		}
	})
}

func TestTriggerDisarmsWhenConfigured(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, stop := run(Config{ExitDelay: 0, TriggerTime: 30 * time.Second, DisarmAfterTrigger: true, ArmModes: []string{"away"}})
		defer stop()

		e.Arm("away", Actor{Name: "a"}, false)
		e.Trigger(true, Actor{Name: "panic"})
		time.Sleep(31 * time.Second)
		synctest.Wait()
		if got := e.Current().State; got != StateDisarmed {
			t.Fatalf("want disarmed after trigger, got %s", got)
		}
	})
}

func TestEntryDelayPendingThenTriggered(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, stop := run(Config{ExitDelay: 0, EntryDelay: 30 * time.Second, TriggerTime: 0, ArmModes: []string{"away"}})
		defer stop()

		e.Arm("away", Actor{Name: "a"}, false)
		r := e.Trigger(false, Actor{Name: "door"})
		if !r.Accepted || r.Snapshot.State != StatePending {
			t.Fatalf("entry-delay trigger: %+v", r)
		}
		time.Sleep(29 * time.Second)
		synctest.Wait()
		if got := e.Current().State; got != StatePending {
			t.Fatalf("at 29s want pending, got %s", got)
		}
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if got := e.Current().State; got != StateTriggered {
			t.Fatalf("after entry delay want triggered, got %s", got)
		}
	})
}

func TestSkipDelayArmsImmediately(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, stop := run(Config{ExitDelay: 120 * time.Second, ArmModes: []string{"night"}})
		defer stop()

		e.Arm("night", Actor{Name: "a"}, false)
		r := e.SkipDelay(Actor{Name: "a"})
		if !r.Accepted || r.Snapshot.State != StateArmedNight {
			t.Fatalf("skip delay: %+v", r)
		}
	})
}

func TestOpenSensorsGateArming(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, stop := run(Config{ExitDelay: 0, ArmModes: []string{"away"}})
		defer stop()

		e.ConfigureSensors([]SensorConfig{{ID: "fd", Name: "front_door"}}, nil)
		e.SensorEvent("fd", SensorOpen)
		if e.Current().ReadyToArm {
			t.Fatal("ready_to_arm should be false with an open sensor")
		}
		if r := e.Arm("away", Actor{Name: "a"}, false); r.Accepted || r.Reason != "open_sensors" {
			t.Fatalf("expected open_sensors rejection, got %+v", r)
		}
		if r := e.Arm("away", Actor{Name: "a"}, true); !r.Accepted {
			t.Fatalf("force arm should succeed, got %+v", r)
		}
	})
}

func TestRestoreArmModeOnStartup(t *testing.T) {
	e := New(Config{ArmModes: []string{"away"}, RestoreArmMode: "away"}, nil)
	if got := e.Current().State; got != StateArmedAway {
		t.Fatalf("restored state want armed_away, got %s", got)
	}
	// An invalid/empty restore stays disarmed.
	e2 := New(Config{ArmModes: []string{"away"}, RestoreArmMode: ""}, nil)
	if got := e2.Current().State; got != StateDisarmed {
		t.Fatalf("empty restore want disarmed, got %s", got)
	}
}

func TestOnCommitPersistsSettledStates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var got []string
		e := New(Config{ExitDelay: 0, ArmModes: []string{"away"}, OnCommit: func(m string) {
			got = append(got, m)
		}}, nil)
		ctx, cancel := context.WithCancel(context.Background())
		go e.Run(ctx)
		e.Arm("away", Actor{Name: "a"}, false) // commit "away"
		e.Disarm(Actor{Name: "a"})             // commit ""
		cancel()
		synctest.Wait()
		if len(got) != 2 || got[0] != "away" || got[1] != "" {
			t.Fatalf("commit sequence = %v, want [away \"\"]", got)
		}
	})
}

func TestInvalidModeRejected(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e, stop := run(Config{ExitDelay: 0, ArmModes: []string{"away"}})
		defer stop()
		if r := e.Arm("home", Actor{Name: "a"}, false); r.Accepted || r.Reason != "invalid_mode" {
			t.Fatalf("home not in arm_modes should be rejected, got %+v", r)
		}
	})
}
