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

func TestUserListUnmarshalTolerantOfMockSupervisor(t *testing.T) {
	// The CI mock Supervisor serializes an empty users option as "[]".
	var u UserList
	if err := json.Unmarshal([]byte(`"[]"`), &u); err != nil {
		t.Fatalf("unmarshal mock users: %v", err)
	}
	if u != nil {
		t.Fatalf("want nil, got %v", u)
	}

	var u2 UserList
	if err := json.Unmarshal([]byte(`[{"name":"Anthony","pin":"1234","role":"admin"}]`), &u2); err != nil {
		t.Fatalf("unmarshal real users: %v", err)
	}
	if len(u2) != 1 || u2[0].Name != "Anthony" || u2[0].Role != "admin" {
		t.Fatalf("unexpected users: %+v", u2)
	}
}

// TestMockSupervisorOptionsShape reproduces exactly what the CI mock
// Supervisor returns (list/object options as bare strings) and asserts the
// whole Options object decodes and normalizes without error.
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
		"mqtt_topic_prefix": "aegis_ha",
		"admin_usernames": "[]",
		"users": "[]"
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
	if o.PINMinLength != 4 || o.LockoutThreshold != 5 {
		t.Errorf("policy defaults wrong: min=%d lockout=%d", o.PINMinLength, o.LockoutThreshold)
	}
}

func TestApplyDefaults(t *testing.T) {
	var o Options
	o.applyDefaults()
	if o.LogLevel != "info" || o.UniFiSite != "default" || o.ProtectMode != "auto" {
		t.Errorf("defaults not applied: %+v", o)
	}
	if o.MQTTCodeFormat != "number" || o.DefaultRole != "user" {
		t.Errorf("defaults not applied: %+v", o)
	}
}

func TestRedactedMasksSecrets(t *testing.T) {
	o := &Options{UniFiAPIKey: "supersecret", Users: UserList{{Name: "a", PIN: "1234"}}}
	r := o.Redacted()
	if r.UniFiAPIKey != "***" {
		t.Errorf("api key not masked: %q", r.UniFiAPIKey)
	}
	if r.Users[0].PIN != "" {
		t.Errorf("user pin not stripped: %q", r.Users[0].PIN)
	}
	if o.UniFiAPIKey != "supersecret" || o.Users[0].PIN != "1234" {
		t.Errorf("Redacted mutated the original options")
	}
}
