// Package unifi is a small client for the UniFi Protect Integration API
// (https://<gateway>/proxy/protect/integration/v1), authenticated with a
// UniFi OS API key sent as the X-API-KEY header.
//
// The honest design constraint: when the Protect Alarm Manager is in
// "Global" mode, the NVR refuses local arm/disarm — GET /v1/nvrs reports
// armMode:null and POST /v1/arm-profiles/enable returns HTTP 400 whose
// body contains "global alarm manager". An API key cannot bypass this, so
// the client only ever DETECTS the capability (non-destructively) and the
// caller decides whether to mirror arm/disarm or run app-managed. The
// InsecureSkipVerify transport here is isolated to the configured UniFi
// host and is never shared with the Supervisor/MQTT/Core clients.
package unifi

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/websocket"
)

// Mode is the detected Alarm Manager capability.
type Mode string

const (
	ModeLocal       Mode = "local"       // Alarm Manager local: arm/disarm can be mirrored
	ModeGlobal      Mode = "global"      // Alarm Manager global: local arm/disarm blocked
	ModeAppManaged  Mode = "app-managed" // unsupported/old firmware: AegisHA owns arm fully
	ModeUnavailable Mode = "unavailable" // gateway unreachable / bad key
)

// Sentinel errors.
var (
	ErrGlobalMode = errors.New("unifi: alarm manager is in global mode (local control blocked)")
	ErrNotFound   = errors.New("unifi: endpoint not found")
)

// Client talks to one UniFi Protect Integration API.
type Client struct {
	base   string
	host   string
	apiKey string
	tls    *tls.Config
	http   *http.Client
	log    *slog.Logger
}

// New builds a Client for the given gateway host. verifySSL=false (the
// default for self-signed gateways) installs an InsecureSkipVerify
// transport that is unique to THIS client.
func New(host, apiKey string, verifySSL bool, log *slog.Logger) *Client {
	if log == nil {
		log = slog.Default()
	}
	tlsCfg := &tls.Config{InsecureSkipVerify: !verifySSL}
	return &Client{
		base:   "https://" + host + "/proxy/protect/integration",
		host:   host,
		apiKey: apiKey,
		tls:    tlsCfg,
		http:   &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{TLSClientConfig: tlsCfg}},
		log:    log,
	}
}

// DialEvents opens the Protect device-event WebSocket. It is used as a
// low-latency change signal (the manager re-polls the authoritative REST
// sensors on any event); the wire format of individual events is not
// parsed, which keeps it robust across firmware revisions.
func (c *Client) DialEvents() (*websocket.Conn, error) {
	wsURL := "wss://" + c.host + "/proxy/protect/integration/v1/subscribe/devices"
	cfg, err := websocket.NewConfig(wsURL, "https://"+c.host)
	if err != nil {
		return nil, err
	}
	cfg.Header.Set("X-API-KEY", c.apiKey)
	cfg.TlsConfig = c.tls
	return websocket.DialConfig(cfg)
}

// NVR is the subset of an NVR record we need.
type NVR struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	ArmMode *string `json:"armMode"`
}

// ArmProfile is a Protect arm profile.
type ArmProfile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Sensor is the subset of a Protect sensor we need for breach detection.
type Sensor struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	IsOpen  bool   `json:"isOpened"`
	Mounted bool   `json:"isMounted"`
}

// GetNVRs returns the gateway's NVR records.
func (c *Client) GetNVRs(ctx context.Context) ([]NVR, error) {
	var nvrs []NVR
	if err := c.get(ctx, "/v1/nvrs", &nvrs); err == nil {
		return nvrs, nil
	} else if !errors.Is(err, errArrayDecode) {
		return nil, err
	}
	// Some firmwares return a single object rather than an array.
	var single NVR
	if err := c.get(ctx, "/v1/nvrs", &single); err != nil {
		return nil, err
	}
	return []NVR{single}, nil
}

// GetArmProfiles returns the configured arm profiles.
func (c *Client) GetArmProfiles(ctx context.Context) ([]ArmProfile, error) {
	var profiles []ArmProfile
	if err := c.get(ctx, "/v1/arm-profiles", &profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}

// GetSensors returns the gateway's sensors.
func (c *Client) GetSensors(ctx context.Context) ([]Sensor, error) {
	var sensors []Sensor
	if err := c.get(ctx, "/v1/sensors", &sensors); err != nil {
		return nil, err
	}
	return sensors, nil
}

// DetectMode determines the Alarm Manager capability without mutating
// state (it never calls enable/disable).
func (c *Client) DetectMode(ctx context.Context) (Mode, error) {
	nvrs, err := c.GetNVRs(ctx)
	if err != nil {
		return ModeUnavailable, err
	}
	if len(nvrs) == 0 {
		return ModeUnavailable, nil
	}
	if m := nvrs[0].ArmMode; m != nil && *m != "" {
		return ModeLocal, nil
	}
	// armMode is null: arm-profiles present => Alarm Manager is Global;
	// absent/unsupported => firmware too old, AegisHA runs app-managed.
	if _, err := c.GetArmProfiles(ctx); err != nil {
		return ModeAppManaged, nil
	}
	return ModeGlobal, nil
}

// Arm mirrors an armed state to the NVR (local mode only). If profileID
// is set, the active arm profile is selected first. Returns ErrGlobalMode
// when the gateway refuses local control.
func (c *Client) Arm(ctx context.Context, profileID string) error {
	if profileID != "" {
		if err := c.send(ctx, http.MethodPatch, "/v1/arm-profiles/settings", map[string]any{"armProfileId": profileID}); err != nil {
			return err
		}
	}
	return c.send(ctx, http.MethodPost, "/v1/arm-profiles/enable", nil)
}

// Disarm clears the NVR's armed state (local mode only).
func (c *Client) Disarm(ctx context.Context) error {
	return c.send(ctx, http.MethodPost, "/v1/arm-profiles/disable", nil)
}

// TriggerSiren plays a camera/chime siren for durationMs (app-managed
// actuation on a real alarm trigger).
func (c *Client) TriggerSiren(ctx context.Context, id string, durationMs int) error {
	return c.send(ctx, http.MethodPost, "/v1/sirens/"+id, map[string]any{"durationMs": durationMs})
}

// FireWebhook fires an Alarm Manager webhook trigger, which works
// regardless of arm mode.
func (c *Client) FireWebhook(ctx context.Context, triggerID string) error {
	return c.send(ctx, http.MethodPost, "/v1/alarm-manager/webhook/"+triggerID, nil)
}

// --- HTTP plumbing ---

var errArrayDecode = errors.New("unifi: response was not a JSON array")

func (c *Client) get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

func (c *Client) send(ctx context.Context, method, path string, body any) error {
	return c.do(ctx, method, path, body, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-KEY", c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("unifi: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode >= 400 {
		snip, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if resp.StatusCode == http.StatusBadRequest && strings.Contains(strings.ToLower(string(snip)), "global alarm manager") {
			return ErrGlobalMode
		}
		return fmt.Errorf("unifi: %s %s: HTTP %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(snip)))
	}
	if out == nil {
		return nil
	}
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(out); err != nil {
		// Distinguish an array/object mismatch so GetNVRs can retry.
		if _, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
			return errArrayDecode
		}
		return fmt.Errorf("unifi: decode %s: %w", path, err)
	}
	return nil
}
