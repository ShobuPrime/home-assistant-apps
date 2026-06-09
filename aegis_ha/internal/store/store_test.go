package store

import (
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir(), Policy{
		LockoutThreshold: 3,
		LockoutDuration:  5 * time.Minute,
		PINMin:           4,
		PINMax:           8,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return s
}

func armCode(mode string) Perm { return Perm{Action: "arm", Mode: mode, CodeRequired: true} }
func disarmCode() Perm         { return Perm{Action: "disarm", CodeRequired: true} }

func TestImportAndAuthorize(t *testing.T) {
	s := testStore(t)
	n, err := s.ImportBootstrap([]Bootstrap{{Name: "Anthony", PIN: "1234", Role: "admin"}}, "user")
	if err != nil || n != 1 {
		t.Fatalf("import: n=%d err=%v", n, err)
	}
	now := time.Now()
	if d := s.AuthorizeMQTT("1234", armCode("away"), now); !d.Allowed || d.User.Name != "Anthony" {
		t.Fatalf("valid pin: %+v", d)
	}
	if d := s.AuthorizeMQTT("9999", armCode("away"), now); d.Allowed || d.Reason != "invalid_code" {
		t.Fatalf("invalid pin should be rejected: %+v", d)
	}
	// Second import is a no-op (store not empty).
	if n, _ := s.ImportBootstrap([]Bootstrap{{Name: "X", PIN: "5555"}}, "user"); n != 0 {
		t.Fatalf("second import should be skipped, imported %d", n)
	}
}

func TestGlobalLockout(t *testing.T) {
	s := testStore(t)
	s.ImportBootstrap([]Bootstrap{{Name: "A", PIN: "1234"}}, "user")
	now := time.Now()
	for range 3 {
		s.AuthorizeMQTT("0000", disarmCode(), now)
	}
	// Now locked: even the correct PIN is rejected.
	if d := s.AuthorizeMQTT("1234", disarmCode(), now); d.Reason != "locked" {
		t.Fatalf("expected lockout, got %+v", d)
	}
	// After the lockout window, it works again.
	if d := s.AuthorizeMQTT("1234", disarmCode(), now.Add(6*time.Minute)); !d.Allowed {
		t.Fatalf("should be allowed after window: %+v", d)
	}
	// Manual clear also works.
	for range 3 {
		s.AuthorizeMQTT("0000", disarmCode(), now)
	}
	if !s.LockoutActive(now) {
		t.Fatal("lockout should be active")
	}
	if err := s.ClearLockout(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if s.LockoutActive(now) {
		t.Fatal("lockout should be cleared")
	}
}

func TestDuressPIN(t *testing.T) {
	s := testStore(t)
	if _, err := s.AddUser(User{Name: "Duress", IsDuress: true}, "0000"); err != nil {
		t.Fatalf("add duress: %v", err)
	}
	d := s.AuthorizeMQTT("0000", disarmCode(), time.Now())
	if !d.Allowed || !d.Duress {
		t.Fatalf("duress pin should be allowed+duress: %+v", d)
	}
}

func TestOneTimeCodeConsumed(t *testing.T) {
	s := testStore(t)
	if _, err := s.AddUser(User{Name: "Guest", OneTime: true}, "4321"); err != nil {
		t.Fatalf("add: %v", err)
	}
	now := time.Now()
	if d := s.AuthorizeMQTT("4321", disarmCode(), now); !d.Allowed {
		t.Fatalf("first use should work: %+v", d)
	}
	if d := s.AuthorizeMQTT("4321", disarmCode(), now); d.Allowed || d.Reason != "expired" {
		t.Fatalf("second use should be expired: %+v", d)
	}
}

func TestPINUniqueness(t *testing.T) {
	s := testStore(t)
	if _, err := s.AddUser(User{Name: "A"}, "1234"); err != nil {
		t.Fatalf("add A: %v", err)
	}
	if _, err := s.AddUser(User{Name: "B"}, "1234"); err != ErrPINTaken {
		t.Fatalf("duplicate pin should be rejected, got %v", err)
	}
}

func TestPINLength(t *testing.T) {
	s := testStore(t)
	if _, err := s.AddUser(User{Name: "A"}, "12"); err != ErrPINLength {
		t.Fatalf("short pin should be rejected, got %v", err)
	}
}

func TestArmModeRestriction(t *testing.T) {
	s := testStore(t)
	s.AddUser(User{Name: "Kid", AllowedArmModes: []string{"home"}}, "2468")
	now := time.Now()
	if d := s.AuthorizeMQTT("2468", armCode("away"), now); d.Allowed || d.Reason != "not_allowed" {
		t.Fatalf("away not allowed for this user: %+v", d)
	}
	if d := s.AuthorizeMQTT("2468", armCode("home"), now); !d.Allowed {
		t.Fatalf("home should be allowed: %+v", d)
	}
}

func TestSharedCode(t *testing.T) {
	s := testStore(t)
	now := time.Now()

	// No code configured: an action that needs no code is allowed for the
	// (already-authenticated) ingress user; one that needs a code is not.
	if d := s.AuthorizeUser("", Perm{Action: "arm", Mode: "away"}, now); !d.Allowed {
		t.Fatalf("no-code arm should be allowed with no code set: %+v", d)
	}
	if d := s.AuthorizeUser("", disarmCode(), now); d.Allowed || d.Reason != "invalid_code" {
		t.Fatalf("code-required disarm should fail with no code set: %+v", d)
	}

	// Configure a shared code.
	if err := s.SetCode("4321"); err != nil {
		t.Fatalf("set code: %v", err)
	}
	if d := s.AuthorizeUser("4321", disarmCode(), now); !d.Allowed {
		t.Fatalf("correct shared code: %+v", d)
	}
	if d := s.AuthorizeUser("0000", disarmCode(), now); d.Allowed || d.Reason != "invalid_code" {
		t.Fatalf("wrong code: %+v", d)
	}
	// The MQTT (identity-less) path validates against the same shared code.
	if d := s.AuthorizeMQTT("4321", disarmCode(), now); !d.Allowed {
		t.Fatalf("mqtt shared code: %+v", d)
	}

	// Clearing the code returns to no-code operation.
	if err := s.SetCode(""); err != nil {
		t.Fatalf("clear code: %v", err)
	}
	if d := s.AuthorizeUser("", Perm{Action: "disarm"}, now); !d.Allowed {
		t.Fatalf("no-code disarm should be allowed after clearing code: %+v", d)
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, Policy{PINMin: 4, PINMax: 8})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := s.AddUser(User{Name: "Anthony", Role: "admin"}, "1234"); err != nil {
		t.Fatalf("add: %v", err)
	}
	// Reopen and verify the user + PIN survived.
	s2, err := Open(dir, Policy{PINMin: 4, PINMax: 8})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if s2.Count() != 1 {
		t.Fatalf("want 1 user after reopen, got %d", s2.Count())
	}
	if d := s2.AuthorizeMQTT("1234", disarmCode(), time.Now()); !d.Allowed {
		t.Fatalf("pin should verify after reopen: %+v", d)
	}
}
