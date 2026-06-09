// Package config loads AegisHA app options from the Supervisor.
//
// Options are read once at startup from /data/options.json (the real HA
// app path). When that file is absent — as in the CI smoke-test harness,
// which boots the container against a mock Supervisor — we fall back to
// the Supervisor REST API at /addons/self/options/config. Either way the
// result is normalized through applyDefaults so a partial or empty options
// object still yields a usable configuration.
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

// Options is the full AegisHA configuration surface. JSON tags match the
// keys in config.yaml's options/schema blocks exactly.
type Options struct {
	LogLevel string `json:"log_level"`

	// UniFi Protect
	UniFiHost      string `json:"unifi_host"`
	UniFiAPIKey    string `json:"unifi_api_key"`
	UniFiVerifySSL bool   `json:"unifi_verify_ssl"`
	ProtectMode    string `json:"protect_mode"` // auto | local | app-managed

	// Protect Alarm Manager webhook trigger IDs (Global-mode actuation):
	// AegisHA POSTs /v1/alarm-manager/webhook/<id> to fire the bound Protect
	// alarm's actions when it arms / disarms / triggers. Works in Global mode
	// (unlike arm profiles). Create the alarms + webhook triggers in the
	// Protect app and paste the IDs here.
	UniFiWebhookArm     string `json:"unifi_webhook_arm"`
	UniFiWebhookDisarm  string `json:"unifi_webhook_disarm"`
	UniFiWebhookTrigger string `json:"unifi_webhook_trigger"`
	// ExitDelaySource: "app" (AegisHA owns the exit-delay countdown and fires
	// the ARM webhook when fully armed) or "unifi" (fire the ARM webhook when
	// arming begins, so the Protect alarm's own activation delay governs).
	ExitDelaySource string `json:"exit_delay_source"`

	// Alarm behavior
	ArmModes                   StringList `json:"arm_modes"`
	ExitDelay                  int        `json:"exit_delay"`
	EntryDelay                 int        `json:"entry_delay"`
	TriggerTime                int        `json:"trigger_time"`
	DisarmAfterTrigger         bool       `json:"disarm_after_trigger"`
	IgnoreBlockingAfterTrigger bool       `json:"ignore_blocking_sensors_after_trigger"`

	// Code / identity model. Code is a single optional shared PIN; when it is
	// empty no code is ever required and the authenticated Home Assistant user
	// (ingress identity) is the actor. The require_* toggles only matter when
	// a code is set.
	Code                string `json:"code"`
	RequireCodeToArm    bool   `json:"require_code_to_arm"`
	RequireCodeToDisarm bool   `json:"require_code_to_disarm"`

	// Brute-force protection on the shared code.
	LockoutThreshold int `json:"lockout_threshold"`
	LockoutDuration  int `json:"lockout_duration"`

	// MQTT
	MQTTTopicPrefix string `json:"mqtt_topic_prefix"`

	// Web UI / card / zones
	EnableWebUI         bool `json:"enable_web_ui"`
	EnableCompanionCard bool `json:"enable_companion_card"`
	ExposeZoneEntities  bool `json:"expose_zone_entities"`
}

const (
	optionsFile      = "/data/options.json"
	supervisorOptURL = "http://supervisor/addons/self/options/config"
)

// Load reads and normalizes the app options.
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
	if o.ProtectMode == "" {
		o.ProtectMode = "auto"
	}
	if o.ExitDelaySource == "" {
		o.ExitDelaySource = "app"
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
	if o.LockoutThreshold == 0 {
		o.LockoutThreshold = 5
	}
	if o.LockoutDuration == 0 {
		o.LockoutDuration = 300
	}
}

// Redacted returns a copy of the options safe for logging: secrets are
// masked.
func (o *Options) Redacted() Options {
	c := *o
	if c.UniFiAPIKey != "" {
		c.UniFiAPIKey = "***"
	}
	if c.Code != "" {
		c.Code = "***"
	}
	return c
}
