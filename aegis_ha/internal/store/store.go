// Package store is AegisHA's authoritative PIN / user store.
//
// PINs are hashed at rest with PBKDF2-SHA256 (stdlib crypto/pbkdf2) and a
// per-PIN random salt. A separate server-pepper HMAC of each PIN is kept
// as a deterministic index so a keypad entry resolves to a single
// candidate user in O(1) — this is what lets the unauthenticated MQTT
// command topic validate a PIN with exactly one slow hash, instead of an
// O(N) hash sweep across every user (a CPU-exhaustion vector). The HMAC
// index also enforces global PIN uniqueness, which is required for
// unambiguous user attribution on the identity-less MQTT path.
//
// Lockout counters (a global one for the anonymous MQTT path and a
// per-user one for the identity-bearing ingress path) persist an absolute
// locked_until timestamp, so a restart cannot clear an active lockout.
package store

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	pbkdf2Iters = 64000
	saltLen     = 16
	pepperLen   = 32
)

// Errors returned by mutating operations.
var (
	ErrPINTaken     = errors.New("store: PIN already in use by another user")
	ErrPINLength    = errors.New("store: PIN length out of range")
	ErrUserNotFound = errors.New("store: user not found")
)

// Roles.
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
	RoleGuest = "guest"
)

// User is a single keypad user/profile.
type User struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	HAUserID        string    `json:"ha_user_id,omitempty"`
	Enabled         bool      `json:"enabled"`
	Role            string    `json:"role"`
	AllowedArmModes []string  `json:"allowed_arm_modes,omitempty"`
	IsDuress        bool      `json:"is_duress,omitempty"`
	OneTime         bool      `json:"one_time,omitempty"`
	ExpiresAt       time.Time `json:"expires_at,omitzero"`
	UsedAt          time.Time `json:"used_at,omitzero"`

	// PIN material (never serialized in plaintext).
	PINHash    string `json:"pin_hash,omitempty"`
	PINSalt    string `json:"pin_salt,omitempty"`
	LookupHMAC string `json:"lookup_hmac,omitempty"`

	// Per-user lockout (ingress path).
	FailedAttempts int       `json:"failed_attempts,omitempty"`
	LockedUntil    time.Time `json:"locked_until,omitzero"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Active reports whether the user may be used at time now.
func (u *User) Active(now time.Time) bool {
	if !u.Enabled {
		return false
	}
	if !u.ExpiresAt.IsZero() && now.After(u.ExpiresAt) {
		return false
	}
	if u.OneTime && !u.UsedAt.IsZero() {
		return false
	}
	return true
}

// IsAdmin reports whether the user has the admin role.
func (u *User) IsAdmin() bool { return u.Role == RoleAdmin }

// Can reports whether the user may perform action (arm/disarm/trigger)
// for the given arm mode (mode is ignored for disarm/trigger).
func (u *User) Can(action, mode string) bool {
	switch action {
	case "disarm", "trigger":
		return true // any active user may disarm or raise a panic
	case "arm":
		if len(u.AllowedArmModes) == 0 {
			return true
		}
		return slices.Contains(u.AllowedArmModes, mode)
	}
	return false
}

// global holds the anonymous-path lockout counters.
type global struct {
	FailedAttempts int       `json:"failed_attempts"`
	LockedUntil    time.Time `json:"locked_until,omitzero"`
}

type fileData struct {
	Users  []*User `json:"users"`
	Global global  `json:"global"`
}

// Policy holds lockout and PIN-length rules.
type Policy struct {
	LockoutThreshold int
	LockoutDuration  time.Duration
	PINMin           int
	PINMax           int
}

// Store is the in-memory + on-disk user store. Safe for concurrent use.
type Store struct {
	mu       sync.Mutex
	dir      string
	path     string
	pepper   []byte
	policy   Policy
	users    []*User
	global   global
	byLookup map[string]*User // lookup HMAC -> user
	byID     map[string]*User
}

// Open loads (or initializes) the store under dataDir.
func Open(dataDir string, policy Policy) (*Store, error) {
	if policy.PINMin == 0 {
		policy.PINMin = 4
	}
	if policy.PINMax == 0 {
		policy.PINMax = 8
	}
	s := &Store{
		dir:    dataDir,
		path:   filepath.Join(dataDir, "store.json"),
		policy: policy,
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("store: mkdir: %w", err)
	}
	if err := s.loadPepper(); err != nil {
		return nil, err
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) loadPepper() error {
	p := filepath.Join(s.dir, "pepper")
	if b, err := os.ReadFile(p); err == nil {
		dec, err := hex.DecodeString(strings.TrimSpace(string(b)))
		if err == nil && len(dec) == pepperLen {
			s.pepper = dec
			return nil
		}
	}
	s.pepper = make([]byte, pepperLen)
	if _, err := rand.Read(s.pepper); err != nil {
		return fmt.Errorf("store: generate pepper: %w", err)
	}
	if err := os.WriteFile(p, []byte(hex.EncodeToString(s.pepper)), 0o600); err != nil {
		return fmt.Errorf("store: write pepper: %w", err)
	}
	return nil
}

func (s *Store) load() error {
	s.byLookup = map[string]*User{}
	s.byID = map[string]*User{}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("store: read: %w", err)
	}
	var fd fileData
	if err := json.Unmarshal(b, &fd); err != nil {
		return fmt.Errorf("store: parse: %w", err)
	}
	s.users = fd.Users
	s.global = fd.Global
	s.reindex()
	return nil
}

func (s *Store) reindex() {
	s.byLookup = make(map[string]*User, len(s.users))
	s.byID = make(map[string]*User, len(s.users))
	for _, u := range s.users {
		s.byID[u.ID] = u
		if u.LookupHMAC != "" {
			s.byLookup[u.LookupHMAC] = u
		}
	}
}

// save writes the store atomically (temp file + rename).
func (s *Store) save() error {
	fd := fileData{Users: s.users, Global: s.global}
	b, err := json.MarshalIndent(fd, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("store: write: %w", err)
	}
	return os.Rename(tmp, s.path)
}

// IsEmpty reports whether the store has no users.
func (s *Store) IsEmpty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.users) == 0
}

func (s *Store) lookupHMAC(pin string) string {
	mac := hmac.New(sha256.New, s.pepper)
	mac.Write([]byte(normalizePIN(pin)))
	return hex.EncodeToString(mac.Sum(nil))
}

func hashPIN(pin string) (hashHex, saltHex string, err error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", "", err
	}
	dk, err := pbkdf2.Key(sha256.New, normalizePIN(pin), salt, pbkdf2Iters, 32)
	if err != nil {
		return "", "", err
	}
	return hex.EncodeToString(dk), hex.EncodeToString(salt), nil
}

func verifyPIN(pin, hashHex, saltHex string) bool {
	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(hashHex)
	if err != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, normalizePIN(pin), salt, pbkdf2Iters, 32)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

func normalizePIN(pin string) string { return strings.TrimSpace(pin) }

func (s *Store) setPINLocked(u *User, pin string) error {
	if l := len(normalizePIN(pin)); l < s.policy.PINMin || l > s.policy.PINMax {
		return ErrPINLength
	}
	lk := s.lookupHMAC(pin)
	if other, ok := s.byLookup[lk]; ok && other.ID != u.ID {
		return ErrPINTaken
	}
	h, salt, err := hashPIN(pin)
	if err != nil {
		return err
	}
	if u.LookupHMAC != "" {
		delete(s.byLookup, u.LookupHMAC)
	}
	u.PINHash, u.PINSalt, u.LookupHMAC = h, salt, lk
	s.byLookup[lk] = u
	return nil
}
