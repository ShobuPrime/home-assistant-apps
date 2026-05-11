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

func TestNormalize_TrimsStringFields(t *testing.T) {
	t.Parallel()
	o := Options{
		LogLevel:            "  info  ",
		MAPlayerID:          " media_player.living_room_2 ",
		FriendlyNameYouTube: "  Sonuntius (YouTube)",
		FriendlyNameTidal:   "Sonuntius (Tidal)\n",
		CastCertPath:        "\t/share/sonuntius/airreceiver_cert.pem ",
		CastKeyPath:         "/share/sonuntius/airreceiver_key.pem",
		HABaseURL:           "  http://homeassistant.local:8123  ",
		HAToken:             "\tsecret-token ",
		MAWsURL:             " ws://music-assistant:8095/ws ",
		MAToken:             "  another-secret ",
		TidalFallback: TidalFallback{
			BinaryTarballPath: "  /share/sonuntius/ifi.tar.gz",
			CertFilename:      "IfiAudio_ZenStream.dat\n",
			FriendlyName:      " Sonuntius (Tidal Connect) ",
			SendspinServerURL: "  ws://music-assistant:8095/sendspin ",
		},
	}
	o.normalize()
	wantStrings := map[string]string{
		"LogLevel":                          "info",
		"MAPlayerID":                        "media_player.living_room_2",
		"FriendlyNameYouTube":               "Sonuntius (YouTube)",
		"FriendlyNameTidal":                 "Sonuntius (Tidal)",
		"CastCertPath":                      "/share/sonuntius/airreceiver_cert.pem",
		"CastKeyPath":                       "/share/sonuntius/airreceiver_key.pem",
		"HABaseURL":                         "http://homeassistant.local:8123",
		"HAToken":                           "secret-token",
		"MAWsURL":                           "ws://music-assistant:8095/ws",
		"MAToken":                           "another-secret",
		"TidalFallback.BinaryTarballPath":   "/share/sonuntius/ifi.tar.gz",
		"TidalFallback.CertFilename":        "IfiAudio_ZenStream.dat",
		"TidalFallback.FriendlyName":        "Sonuntius (Tidal Connect)",
		"TidalFallback.SendspinServerURL":   "ws://music-assistant:8095/sendspin",
	}
	got := map[string]string{
		"LogLevel":                          o.LogLevel,
		"MAPlayerID":                        o.MAPlayerID,
		"FriendlyNameYouTube":               o.FriendlyNameYouTube,
		"FriendlyNameTidal":                 o.FriendlyNameTidal,
		"CastCertPath":                      o.CastCertPath,
		"CastKeyPath":                       o.CastKeyPath,
		"HABaseURL":                         o.HABaseURL,
		"HAToken":                           o.HAToken,
		"MAWsURL":                           o.MAWsURL,
		"MAToken":                           o.MAToken,
		"TidalFallback.BinaryTarballPath":   o.TidalFallback.BinaryTarballPath,
		"TidalFallback.CertFilename":        o.TidalFallback.CertFilename,
		"TidalFallback.FriendlyName":        o.TidalFallback.FriendlyName,
		"TidalFallback.SendspinServerURL":   o.TidalFallback.SendspinServerURL,
	}
	for field, want := range wantStrings {
		if got[field] != want {
			t.Errorf("%s = %q, want %q", field, got[field], want)
		}
	}
}

func TestEffectiveListenPorts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		opt      Options
		wantDial int
		wantTLS  int
	}{
		{"defaults", Options{}, 8008, 8009},
		{"user override", Options{YTCastDialPort: 9100, CastReceiverTLSPort: 9101}, 9100, 9101},
		{"only dial override", Options{YTCastDialPort: 9100}, 9100, 8009},
		{"only tls override", Options{CastReceiverTLSPort: 9101}, 8008, 9101},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.opt.EffectiveYTCastDialPort(); got != tc.wantDial {
				t.Errorf("EffectiveYTCastDialPort() = %d, want %d", got, tc.wantDial)
			}
			if got := tc.opt.EffectiveCastReceiverTLSPort(); got != tc.wantTLS {
				t.Errorf("EffectiveCastReceiverTLSPort() = %d, want %d", got, tc.wantTLS)
			}
		})
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
