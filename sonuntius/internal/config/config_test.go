package config

import (
	"os"
	"testing"
)

func TestHARESTBaseURLOverride(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opt  Options
		want string
	}{
		{"default", Options{}, "http://supervisor/core/api"},
		{"explicit override", Options{HABaseURL: "http://homeassistant.local:8123"}, "http://homeassistant.local:8123"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.opt.HARESTBaseURL(); got != tc.want {
				t.Errorf("HARESTBaseURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHARESTTokenFallbackToSupervisor(t *testing.T) {
	t.Setenv("SUPERVISOR_TOKEN", "supervisor-injected")
	t.Cleanup(func() { _ = os.Unsetenv("SUPERVISOR_TOKEN") })

	if got := (Options{}).HARESTToken(); got != "supervisor-injected" {
		t.Errorf("HARESTToken() default = %q, want %q", got, "supervisor-injected")
	}
	if got := (Options{HAToken: "user-llt"}).HARESTToken(); got != "user-llt" {
		t.Errorf("HARESTToken() override = %q, want %q", got, "user-llt")
	}
}

func TestHAWebSocketURLDerivation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opt  Options
		want string
	}{
		{
			name: "default supervisor proxy",
			opt:  Options{},
			want: "ws://supervisor/core/websocket",
		},
		{
			name: "http base url",
			opt:  Options{HABaseURL: "http://homeassistant.local:8123"},
			want: "ws://homeassistant.local:8123/core/websocket",
		},
		{
			name: "https base url",
			opt:  Options{HABaseURL: "https://ha.example.com"},
			want: "wss://ha.example.com/core/websocket",
		},
		{
			name: "base url already has /core/api suffix",
			opt:  Options{HABaseURL: "http://homeassistant.local:8123/core/api"},
			want: "ws://homeassistant.local:8123/core/websocket",
		},
		{
			name: "base url has trailing slash",
			opt:  Options{HABaseURL: "http://homeassistant.local:8123/"},
			want: "ws://homeassistant.local:8123/core/websocket",
		},
		{
			name: "user supplied ws:// passthrough",
			opt:  Options{HABaseURL: "ws://custom.local/api/websocket"},
			want: "ws://custom.local/api/websocket",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.opt.HAWebSocketURL(); got != tc.want {
				t.Errorf("HAWebSocketURL() = %q, want %q", got, tc.want)
			}
		})
	}
}
