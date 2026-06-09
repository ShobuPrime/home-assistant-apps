package config

import (
	"encoding/json"
	"testing"
)

func TestStringListUnmarshal(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"array", `["away","home"]`, []string{"away", "home"}},
		{"empty-string", `""`, nil},
		{"empty-brackets-string", `"[]"`, nil},
		{"csv-string", `"away, home, night"`, []string{"away", "home", "night"}},
		{"null", `null`, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var s StringList
			if err := json.Unmarshal([]byte(c.in), &s); err != nil {
				t.Fatalf("unmarshal %q: %v", c.in, err)
			}
			if len(s) != len(c.want) {
				t.Fatalf("len = %d, want %d (%v)", len(s), len(c.want), s)
			}
			for i := range c.want {
				if s[i] != c.want[i] {
					t.Errorf("[%d] = %q, want %q", i, s[i], c.want[i])
				}
			}
		})
	}
}

// TestMockSupervisorOptionsShape reproduces exactly what the CI mock
// Supervisor returns (the arm_modes list serialized as a bare string) and
// asserts the whole Options object decodes and normalizes without error.
func TestMockSupervisorOptionsShape(t *testing.T) {
	raw := `{
		"log_level": "info",
		"unifi_host": "",
		"unifi_verify_ssl": false,
		"protect_mode": "auto",
		"arm_modes": "",
		"exit_delay": 60,
		"entry_delay": 30,
		"trigger_time": 1800,
		"code": "",
		"mqtt_topic_prefix": "aegis_ha"
	}`
	var o Options
	if err := json.Unmarshal([]byte(raw), &o); err != nil {
		t.Fatalf("decode mock options: %v", err)
	}
	o.applyDefaults()
	if len(o.ArmModes) != 3 {
		t.Errorf("arm_modes defaulted wrong: %v", o.ArmModes)
	}
	if o.MQTTTopicPrefix != "aegis_ha" || o.ProtectMode != "auto" {
		t.Errorf("scalar options wrong: prefix=%q mode=%q", o.MQTTTopicPrefix, o.ProtectMode)
	}
	if o.LockoutThreshold != 5 {
		t.Errorf("policy defaults wrong: lockout=%d", o.LockoutThreshold)
	}
}

func TestApplyDefaults(t *testing.T) {
	var o Options
	o.applyDefaults()
	if o.LogLevel != "info" || o.ProtectMode != "auto" || o.ExitDelaySource != "app" {
		t.Errorf("defaults not applied: %+v", o)
	}
	if o.TriggerTime != 1800 || o.LockoutThreshold != 5 || o.LockoutDuration != 300 {
		t.Errorf("defaults not applied: %+v", o)
	}
}

func TestRedactedMasksSecrets(t *testing.T) {
	o := &Options{UniFiAPIKey: "supersecret", Code: "1234"}
	r := o.Redacted()
	if r.UniFiAPIKey != "***" {
		t.Errorf("api key not masked: %q", r.UniFiAPIKey)
	}
	if r.Code != "***" {
		t.Errorf("code not masked: %q", r.Code)
	}
	if o.UniFiAPIKey != "supersecret" || o.Code != "1234" {
		t.Errorf("Redacted mutated the original options")
	}
}
