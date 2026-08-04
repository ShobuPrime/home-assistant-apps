// Package ha is a thin client for the Home Assistant Core API, routed
// through the app's Supervisor proxy (http://supervisor/core/api). AegisHA
// uses it to fire bus events (e.g. aegis_ha_command_success, aegis_ha_failed_to_
// arm, aegis_ha_triggered, aegis_ha_duress) so automations can react the same
// way they would to Alarmo's events, in addition to the MQTT entities.
package ha

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const defaultBaseURL = "http://supervisor/core/api"

// Client wraps the HA Core REST API.
type Client struct {
	base  string
	token string
	http  *http.Client
	log   *slog.Logger
}

// New returns a Client using the Supervisor proxy base URL.
func New(token string, log *slog.Logger) *Client {
	if log == nil {
		log = slog.Default()
	}
	return &Client{
		base:  defaultBaseURL,
		token: token,
		http:  &http.Client{Timeout: 10 * time.Second},
		log:   log,
	}
}

// FireEvent posts a custom event to the HA event bus.
func (c *Client) FireEvent(ctx context.Context, eventType string, data map[string]any) error {
	var body io.Reader
	if len(data) > 0 {
		buf, err := json.Marshal(data)
		if err != nil {
			return err
		}
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/events/"+eventType, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("ha: fire %s: %w", eventType, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		snip, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("ha: fire %s: HTTP %d: %s", eventType, resp.StatusCode, snip)
	}
	return nil
}
