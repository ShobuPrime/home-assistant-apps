// Package config loads AegisHA add-on options from the Supervisor.
//
// Options are read once at startup from /data/options.json (the real HA
// add-on path). When that file is absent — as in the CI smoke-test
// harness, which boots the container against a mock Supervisor — we fall
// back to the Supervisor REST API at /addons/self/options/config. Either
// way the result is normalized through applyDefaults so a partial or
// empty options object still yields a usable configuration.
package config

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode"
)

// StringList is a []string that tolerates being decoded from a JSON
// array, a JSON string, or null. The CI smoke-test's mock Supervisor has
// a naive YAML parser that serializes list options as bare strings
// (e.g. "" or "[]") rather than arrays; real HA always sends arrays.
// Either shape must boot the daemon rather than crash it.
type StringList []string

// UnmarshalJSON implements lenient decoding for StringList.
func (s *StringList) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*s = nil
		return nil
	}
	switch b[0] {
	case '[':
		var arr []string
		if err := json.Unmarshal(b, &arr); err != nil {
			return err
		}
		*s = arr
	case '"':
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		str = strings.TrimSpace(str)
		if str == "" || str == "[]" {
			*s = nil
			return nil
		}
		*s = strings.FieldsFunc(str, func(r rune) bool {
			return r == ',' || unicode.IsSpace(r)
		})
	default:
		return fmt.Errorf("config: cannot decode %s into a string list", b)
	}
	return nil
}

// UserList is a []User that tolerates the mock Supervisor serializing an
// empty/complex option as a bare string.
type UserList []User

// UnmarshalJSON implements lenient decoding for UserList.
func (u *UserList) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" || (len(b) > 0 && b[0] == '"') {
		*u = nil
		return nil
	}
	var arr []User
	if err := json.Unmarshal(b, &arr); err != nil {
		return err
	}
	*u = arr
	return nil
}

// User is a single keypad user/profile entry from the bootstrap options
// list. The plaintext PIN here is imported once into the hashed store and
// the user is then advised to clear it from options (see internal/store).
type User struct {
	Name            string   `json:"name"`
	HAUserID        string   `json:"ha_user_id,omitempty"`
	PIN             string   `json:"pin,omitempty"`
	Role            string     `json:"role,omitempty"`
	AllowedArmModes StringList `json:"allowed_arm_modes,omitempty"`
}

// SensorOverride is an Alarmo-style per-sensor configuration, matched to a
// discovered UniFi Protect sensor by name (case-insensitive).
type SensorOverride struct {
	Name               string     `json:"name"`
	Modes              StringList `json:"modes,omitempty"`
	AlwaysOn           bool       `json:"always_on,omitempty"`
	Immediate          bool       `json:"immediate,omitempty"`
	UseExitDelay       bool       `json:"use_exit_delay,omitempty"`
	AutoBypass         bool       `json:"auto_bypass,omitempty"`
	AllowOpen          bool       `json:"allow_open,omitempty"`
	TriggerUnavailable bool       `json:"trigger_unavailable,omitempty"`
	Group              string     `json:"group,omitempty"`
}

// SensorGroupCfg defines a sensor-group debounce rule.
type SensorGroupCfg struct {
	Name       string `json:"name"`
	EventCount int    `json:"event_count"`
	Timeout    int    `json:"timeout"` // seconds
}

// SensorOverrideList tolerates the mock Supervisor serializing an empty
// list option as a bare string (see StringList/UserList).
type SensorOverrideList []SensorOverride

// UnmarshalJSON implements lenient decoding for SensorOverrideList.
func (l *SensorOverrideList) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" || (len(b) > 0 && b[0] == '"') {
		*l = nil
		return nil
	}
	var arr []SensorOverride
	if err := json.Unmarshal(b, &arr); err != nil {
		return err
	}
	*l = arr
	return nil
}

// SensorGroupList tolerates the mock Supervisor's bare-string empty list.
type SensorGroupList []SensorGroupCfg

// UnmarshalJSON implements lenient decoding for SensorGroupList.
func (l *SensorGroupList) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" || (len(b) > 0 && b[0] == '"') {
		*l = nil
		return nil
	}
	var arr []SensorGroupCfg
	if err := json.Unmarshal(b, &arr); err != nil {
		return err
	}
	*l = arr
	return nil
}

// Options is the full AegisHA configuration surface. JSON tags match the
// keys in config.yaml's options/schema blocks exactly.
type Options struct {
	LogLevel string `json:"log_level"`

	// UniFi Protect
	UniFiHost      string `json:"unifi_host"`
	UniFiAPIKey    string `json:"unifi_api_key"`
	UniFiVerifySSL bool   `json:"unifi_verify_ssl"`
	UniFiSite      string `json:"unifi_site"`
	ProtectMode    string `json:"protect_mode"` // auto | local | app-managed

	// Alarm behavior
	ArmModes                   StringList `json:"arm_modes"`
	ExitDelay                  int      `json:"exit_delay"`
	EntryDelay                 int      `json:"entry_delay"`
	TriggerTime                int      `json:"trigger_time"`
	ArmingRequiresCode         bool     `json:"arming_requires_code"`
	DisarmRequiresCode         bool     `json:"disarm_requires_code"`
	TriggerRequiresCode        bool     `json:"trigger_requires_code"`
	DisarmAfterTrigger         bool     `json:"disarm_after_trigger"`
	IgnoreBlockingAfterTrigger bool     `json:"ignore_blocking_sensors_after_trigger"`

	// MQTT
	MQTTTopicPrefix string `json:"mqtt_topic_prefix"`
	MQTTCodeFormat  string `json:"mqtt_code_format"` // number | text

	// PIN / lockout policy
	LockoutThreshold int    `json:"lockout_threshold"`
	LockoutDuration  int    `json:"lockout_duration"`
	PINMinLength     int    `json:"pin_min_length"`
	PINMaxLength     int    `json:"pin_max_length"`
	DefaultRole      string `json:"default_role"`

	// Web UI / card / admin
	EnableWebUI         bool       `json:"enable_web_ui"`
	EnableCompanionCard bool       `json:"enable_companion_card"`
	ExposeZoneEntities  bool       `json:"expose_zone_entities"`
	AdminUsernames      StringList `json:"admin_usernames"`
	Users               UserList   `json:"users"`

	// Sensor model (Alarmo-style per-sensor overrides + group debounce)
	Sensors      SensorOverrideList `json:"sensors"`
	SensorGroups SensorGroupList    `json:"sensor_groups"`
}

const (
	optionsFile      = "/data/options.json"
	supervisorOptURL = "http://supervisor/addons/self/options/config"
)

// Load reads and normalizes the add-on options.
func Load(ctx context.Context) (*Options, error) {
	raw, err := loadRaw(ctx)
	if err != nil {
		return nil, err
	}
	var o Options
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &o); err != nil {
			return nil, fmt.Errorf("config: parse options: %w", err)
		}
	}
	o.applyDefaults()
	return &o, nil
}

// loadRaw returns the options JSON object bytes, preferring the on-disk
// file and falling back to the Supervisor REST API.
func loadRaw(ctx context.Context) ([]byte, error) {
	if b, err := os.ReadFile(optionsFile); err == nil {
		return b, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("config: read %s: %w", optionsFile, err)
	}
	return loadFromSupervisor(ctx)
}

func loadFromSupervisor(ctx context.Context) ([]byte, error) {
	token := os.Getenv("SUPERVISOR_TOKEN")
	if token == "" {
		// No file and no token: run with pure defaults rather than
		// crashing, so the binary is still exercisable standalone.
		return nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, supervisorOptURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("config: supervisor options request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		snip, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("config: supervisor options HTTP %d: %s", resp.StatusCode, snip)
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("config: decode supervisor options: %w", err)
	}
	return envelope.Data, nil
}

// applyDefaults fills zero values that must be non-zero for safe
// operation. Real HA always supplies the config.yaml defaults, but the
// smoke-test mock Supervisor and standalone runs may not.
func (o *Options) applyDefaults() {
	if o.LogLevel == "" {
		o.LogLevel = "info"
	}
	if o.UniFiSite == "" {
		o.UniFiSite = "default"
	}
	if o.ProtectMode == "" {
		o.ProtectMode = "auto"
	}
	if len(o.ArmModes) == 0 {
		o.ArmModes = []string{"away", "home", "night"}
	}
	if o.TriggerTime == 0 {
		o.TriggerTime = 1800
	}
	if o.MQTTTopicPrefix == "" {
		o.MQTTTopicPrefix = "aegis_ha"
	}
	if o.MQTTCodeFormat == "" {
		o.MQTTCodeFormat = "number"
	}
	if o.LockoutThreshold == 0 {
		o.LockoutThreshold = 5
	}
	if o.LockoutDuration == 0 {
		o.LockoutDuration = 300
	}
	if o.PINMinLength == 0 {
		o.PINMinLength = 4
	}
	if o.PINMaxLength == 0 {
		o.PINMaxLength = 8
	}
	if o.DefaultRole == "" {
		o.DefaultRole = "user"
	}
}

// Redacted returns a copy of the options safe for logging: secrets are
// masked and the bootstrap users' PINs are stripped.
func (o *Options) Redacted() Options {
	c := *o
	if c.UniFiAPIKey != "" {
		c.UniFiAPIKey = "***"
	}
	c.Users = make(UserList, len(o.Users))
	for i, u := range o.Users {
		u.PIN = ""
		c.Users[i] = u
	}
	return c
}
