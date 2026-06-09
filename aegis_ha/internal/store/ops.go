package store

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// Perm describes the capability a command needs.
type Perm struct {
	Action       string // arm | disarm | trigger
	Mode         string // arm mode (ignored for disarm/trigger)
	CodeRequired bool
}

// Decision is the outcome of an authorization attempt.
type Decision struct {
	Allowed bool
	Duress  bool
	User    *User  // nil for an anonymous (no-code) allow
	Reason  string // "" | invalid_code | locked | not_allowed | expired
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// AuthorizeMQTT authorizes an identity-less command from the MQTT keypad
// by resolving the PIN to a user. The global lockout is checked first and
// a wrong PIN increments it before any slow hash work is performed; the
// HMAC index guarantees at most one PBKDF2 verify per call.
func (s *Store) AuthorizeMQTT(pin string, p Perm, now time.Time) Decision {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.globalLocked(now) {
		return Decision{Reason: "locked"}
	}
	// No shared code configured: the alarm is operated with no PIN, so there
	// is nothing to verify and nothing to require — always allow, and never
	// lock the owner out. Any PIN that may have been typed is ignored.
	if len(s.users) == 0 {
		return Decision{Allowed: true}
	}
	if pin == "" {
		if !p.CodeRequired {
			return Decision{Allowed: true} // anonymous, no code required
		}
		return Decision{Reason: "invalid_code"}
	}

	u := s.byLookup[s.lookupHMAC(pin)]
	if u == nil || !verifyPIN(pin, u.PINHash, u.PINSalt) {
		s.recordGlobalFailure(now)
		_ = s.save()
		return Decision{Reason: "invalid_code"}
	}
	return s.decideLocked(u, p, now, true)
}

// AuthorizeUser authorizes a command on the ingress path, where the Home
// Assistant user identity is already trusted (the non-spoofable
// X-Remote-User-Id header is present). Because the login establishes who
// the actor is, AuthorizeUser enforces only the shared code: an action
// that does not require a code is allowed outright, otherwise the entered
// PIN must match the configured shared code. The caller supplies the real
// HA identity as the actor name. This is identical to AuthorizeMQTT — both
// gate solely on the single shared code — and is kept as a separate name
// to document the trusted-identity call site.
func (s *Store) AuthorizeUser(pin string, p Perm, now time.Time) Decision {
	return s.AuthorizeMQTT(pin, p, now)
}

// SetCode replaces the single shared alarm code, the only credential in the
// shared-code model. An empty code removes any stored code, so the alarm
// can be controlled with no PIN (the authenticated Home Assistant user is
// the identity). The code is stored exactly like a PIN — PBKDF2-hashed with
// an HMAC lookup index — so the existing constant-time verification and
// brute-force lockout paths apply unchanged. The persisted lockout counters
// are preserved across the rewrite.
func (s *Store) SetCode(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users = nil
	s.byID = map[string]*User{}
	s.byLookup = map[string]*User{}
	if normalizePIN(code) == "" {
		return s.save()
	}
	now := time.Now()
	u := &User{
		ID:        newID(),
		Name:      "AegisHA",
		Enabled:   true,
		Role:      RoleAdmin,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.setPINLocked(u, code); err != nil {
		return err
	}
	s.users = append(s.users, u)
	s.byID[u.ID] = u
	return s.save()
}

// decideLocked applies the post-verification policy (duress, expiry,
// role, one-time consume, success bookkeeping). Caller holds s.mu.
func (s *Store) decideLocked(u *User, p Perm, now time.Time, global bool) Decision {
	if u.IsDuress {
		// A duress PIN looks like success to the attacker: clear counters
		// and let the bridge perform a silent disarm + duress event.
		s.recordSuccess(u, global, now)
		_ = s.save()
		return Decision{Allowed: true, Duress: true, User: u}
	}
	if !u.Active(now) {
		return Decision{Reason: "expired", User: u}
	}
	if !u.Can(p.Action, p.Mode) {
		return Decision{Reason: "not_allowed", User: u}
	}
	if u.OneTime {
		u.UsedAt = now
	}
	s.recordSuccess(u, global, now)
	_ = s.save()
	return Decision{Allowed: true, User: u}
}

// --- lockout bookkeeping (caller holds s.mu) ---

func (s *Store) globalLocked(now time.Time) bool {
	return !s.global.LockedUntil.IsZero() && now.Before(s.global.LockedUntil)
}

func (s *Store) userLocked(u *User, now time.Time) bool {
	return !u.LockedUntil.IsZero() && now.Before(u.LockedUntil)
}

func (s *Store) recordGlobalFailure(now time.Time) {
	s.global.FailedAttempts++
	if s.policy.LockoutThreshold > 0 && s.global.FailedAttempts >= s.policy.LockoutThreshold {
		s.global.LockedUntil = now.Add(s.policy.LockoutDuration)
	}
}

func (s *Store) recordSuccess(u *User, global bool, _ time.Time) {
	u.FailedAttempts = 0
	u.LockedUntil = time.Time{}
	if global {
		s.global.FailedAttempts = 0
		s.global.LockedUntil = time.Time{}
	}
}

// LockoutActive reports whether any lockout (global or per-user) is
// currently engaged — used to drive the lockout binary_sensor.
func (s *Store) LockoutActive(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.globalLocked(now) {
		return true
	}
	for _, u := range s.users {
		if s.userLocked(u, now) {
			return true
		}
	}
	return false
}

// ClearLockout clears the global lockout and every per-user lockout.
func (s *Store) ClearLockout() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.global.FailedAttempts = 0
	s.global.LockedUntil = time.Time{}
	for _, u := range s.users {
		u.FailedAttempts = 0
		u.LockedUntil = time.Time{}
	}
	return s.save()
}

// --- CRUD ---

// Bootstrap is one entry from the options users[] bootstrap list.
type Bootstrap struct {
	Name            string
	HAUserID        string
	PIN             string
	Role            string
	AllowedArmModes []string
}

// ImportBootstrap seeds the store from the options list. It is a no-op if
// the store already has users. Returns the number imported.
func (s *Store) ImportBootstrap(list []Bootstrap, defaultRole string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.users) > 0 {
		return 0, nil
	}
	n := 0
	for _, b := range list {
		if b.Name == "" || b.PIN == "" {
			continue
		}
		role := b.Role
		if role == "" {
			role = defaultRole
		}
		now := time.Now()
		u := &User{
			ID:              newID(),
			Name:            b.Name,
			HAUserID:        b.HAUserID,
			Enabled:         true,
			Role:            role,
			AllowedArmModes: b.AllowedArmModes,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := s.setPINLocked(u, b.PIN); err != nil {
			return n, err
		}
		s.users = append(s.users, u)
		s.byID[u.ID] = u
		n++
	}
	if n > 0 {
		if err := s.save(); err != nil {
			return n, err
		}
	}
	return n, nil
}

// AddUser creates a new user with the given PIN.
func (s *Store) AddUser(u User, pin string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	nu := u
	nu.ID = newID()
	nu.Enabled = true
	if nu.Role == "" {
		nu.Role = RoleUser
	}
	nu.CreatedAt = now
	nu.UpdatedAt = now
	if err := s.setPINLocked(&nu, pin); err != nil {
		return nil, err
	}
	s.users = append(s.users, &nu)
	s.byID[nu.ID] = &nu
	if err := s.save(); err != nil {
		return nil, err
	}
	return &nu, nil
}

// SetPIN changes a user's PIN.
func (s *Store) SetPIN(id, pin string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.byID[id]
	if u == nil {
		return ErrUserNotFound
	}
	if err := s.setPINLocked(u, pin); err != nil {
		return err
	}
	u.UpdatedAt = time.Now()
	return s.save()
}

// SetEnabled enables or disables a user.
func (s *Store) SetEnabled(id string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.byID[id]
	if u == nil {
		return ErrUserNotFound
	}
	u.Enabled = enabled
	u.UpdatedAt = time.Now()
	return s.save()
}

// DeleteUser removes a user.
func (s *Store) DeleteUser(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.byID[id]
	if u == nil {
		return ErrUserNotFound
	}
	delete(s.byID, id)
	if u.LookupHMAC != "" {
		delete(s.byLookup, u.LookupHMAC)
	}
	s.users = slicesDelete(s.users, u)
	return s.save()
}

// BindHAUser associates an HA user id with a user (first-ingress-visit
// correlation), if not already set.
func (s *Store) BindHAUser(id, haUserID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.byID[id]
	if u == nil {
		return ErrUserNotFound
	}
	u.HAUserID = haUserID
	u.UpdatedAt = time.Now()
	return s.save()
}

// List returns a copy of the users (without PIN material exposed beyond
// what the User struct already carries; callers should not serialize PIN
// fields to clients).
func (s *Store) List() []User {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]User, len(s.users))
	for i, u := range s.users {
		out[i] = *u
	}
	return out
}

// UserByHAID returns a copy of the user bound to the given Home Assistant
// user id, or nil. Keyed on X-Remote-User-Id (the non-spoofable ingress
// identity).
func (s *Store) UserByHAID(haID string) *User {
	if haID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if u.HAUserID == haID {
			cp := *u
			return &cp
		}
	}
	return nil
}

// Count returns the number of users.
func (s *Store) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.users)
}

func slicesDelete(users []*User, target *User) []*User {
	out := users[:0]
	for _, u := range users {
		if u != target {
			out = append(out, u)
		}
	}
	return out
}
