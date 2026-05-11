// Package config loads the addon's user options.
//
// /data/options.json is preferred (populated by the real HA Supervisor)
// with a fallback to the Supervisor REST API at
// http://supervisor/addons/self/options/config — which is what the
// smoke-test mock supervisor exposes.
package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// Options is the parsed addon configuration. Fields default to the
// zero value; the dispatcher checks Required() before issuing calls.
//
// HABaseURL / HAToken / MAWsURL / MAToken are optional overrides. When
// left empty the bridge talks to Home Assistant via the Supervisor
// proxy using the addon's auto-injected SUPERVISOR_TOKEN, which is the
// canonical "MA-is-an-HA-addon" setup. Set them only when the user
// wants to act as a named HA user (long-lived token), point at an HA
// or MA instance outside this Supervisor, or both.
type Options struct {
	LogLevel              string         `json:"log_level"`
	MAPlayerID            string         `json:"ma_player_id"`
	FriendlyNameYouTube   string         `json:"friendly_name_youtube"`
	FriendlyNameTidal     string         `json:"friendly_name_tidal"`
	EnableYouTube         bool           `json:"enable_youtube"`
	EnableTidalProxy      bool           `json:"enable_tidal_proxy"`
	CastCertPath          string         `json:"cast_cert_path"`
	CastKeyPath           string         `json:"cast_key_path"`
	HABaseURL             string         `json:"ha_base_url"`
	HAToken               string         `json:"ha_token"`
	MAWsURL               string         `json:"ma_ws_url"`
	MAToken               string         `json:"ma_token"`
	TidalFallback         TidalFallback  `json:"tidal_fallback"`
}

// TidalFallback is the opt-in iFi Tidal Connect binary configuration.
type TidalFallback struct {
	Enabled            bool   `json:"enabled"`
	BinaryTarballPath  string `json:"binary_tarball_path"`
	CertFilename       string `json:"cert_filename"`
	FriendlyName       string `json:"friendly_name"`
	SendspinServerURL  string `json:"sendspin_server_url"`
}

const (
	defaultOptionsPath = "/data/options.json"
	supervisorBaseURL  = "http://supervisor"
)

// OptionsPath returns the resolved options.json path (env override or default).
func OptionsPath() string {
	if v := os.Getenv("SONUNTIUS_OPTIONS"); v != "" {
		return v
	}
	return defaultOptionsPath
}

// SupervisorToken returns the SUPERVISOR_TOKEN env var; empty if unset.
func SupervisorToken() string {
	return os.Getenv("SUPERVISOR_TOKEN")
}

// Load attempts to read options from disk first, then from the
// Supervisor API. An empty Options is returned (with a logged warning)
// if neither path produces a usable result.
func Load(ctx context.Context, logger *slog.Logger) (Options, error) {
	if opts, ok := loadFromFile(OptionsPath(), logger); ok {
		logger.Info("config: loaded from file", "path", OptionsPath())
		return opts, nil
	}
	token := SupervisorToken()
	if token != "" {
		if opts, ok := loadFromSupervisor(ctx, token, logger); ok {
			logger.Info("config: loaded from supervisor API")
			return opts, nil
		}
	}
	logger.Warn("config: no options source available, using defaults")
	return Options{}, nil
}

func loadFromFile(path string, logger *slog.Logger) (Options, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			logger.Warn("config: file read failed", "path", path, "err", err)
		}
		return Options{}, false
	}
	var opts Options
	if err := json.Unmarshal(data, &opts); err != nil {
		logger.Warn("config: file parse failed", "path", path, "err", err)
		return Options{}, false
	}
	return opts, true
}

func loadFromSupervisor(ctx context.Context, token string, logger *slog.Logger) (Options, bool) {
	url := supervisorBaseURL + "/addons/self/options/config"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		logger.Warn("config: supervisor request build failed", "err", err)
		return Options{}, false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	cli := &http.Client{Timeout: 5 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		logger.Debug("config: supervisor request failed", "err", err)
		return Options{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		logger.Debug("config: supervisor non-200", "status", resp.StatusCode, "body", string(body))
		return Options{}, false
	}
	var envelope struct {
		Data Options `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		logger.Warn("config: supervisor decode failed", "err", err)
		return Options{}, false
	}
	return envelope.Data, true
}

// ResolveLogLevel maps the textual log_level option to a slog level.
// Unknown values fall back to Info.
func ResolveLogLevel(name string) slog.Level {
	switch name {
	case "trace", "debug":
		return slog.LevelDebug
	case "info", "notice", "":
		return slog.LevelInfo
	case "warning":
		return slog.LevelWarn
	case "error", "fatal":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Validate returns an error if any required field is missing.
func (o Options) Validate() error {
	if o.MAPlayerID == "" {
		return fmt.Errorf("ma_player_id is empty")
	}
	return nil
}

// HARESTBaseURL returns the effective HA REST base URL: the user-supplied
// override if set, otherwise the Supervisor proxy default.
func (o Options) HARESTBaseURL() string {
	if o.HABaseURL != "" {
		return o.HABaseURL
	}
	return "http://supervisor/core/api"
}

// HARESTToken returns the effective HA REST token: the user-supplied
// override if set, otherwise the auto-injected SUPERVISOR_TOKEN.
func (o Options) HARESTToken() string {
	if o.HAToken != "" {
		return o.HAToken
	}
	return SupervisorToken()
}

// HAWebSocketURL returns the effective HA core WS URL. We derive it from
// the REST base URL override when present (swapping the scheme and the
// /core/api suffix for /core/websocket) so a single ha_base_url setting
// covers both endpoints. Empty REST override → Supervisor default.
func (o Options) HAWebSocketURL() string {
	if o.HABaseURL == "" {
		return "ws://supervisor/core/websocket"
	}
	base := o.HABaseURL
	// http(s)://host[:port][/...] → ws(s)://host[:port]/core/websocket
	switch {
	case len(base) >= 8 && base[:8] == "https://":
		return "wss://" + trimTrailingSlash(base[8:]) + "/core/websocket"
	case len(base) >= 7 && base[:7] == "http://":
		return "ws://" + trimTrailingSlash(base[7:]) + "/core/websocket"
	default:
		// User supplied a ws:// or wss:// URL already, or something
		// unusual. Pass it through.
		return base
	}
}

func trimTrailingSlash(s string) string {
	// Drop any /core/api or trailing slash so we can append /core/websocket.
	for {
		if len(s) == 0 {
			return s
		}
		if s[len(s)-1] == '/' {
			s = s[:len(s)-1]
			continue
		}
		const suffix = "/core/api"
		if len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix {
			s = s[:len(s)-len(suffix)]
			continue
		}
		return s
	}
}
