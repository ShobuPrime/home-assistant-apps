// Package ha is a thin client for the Home Assistant REST API routed
// through the addon Supervisor proxy.
//
// Music Assistant exposes its players as media_player.* entities in HA,
// so play_media + transport + volume can all be issued via REST
// regardless of whether the MA WebSocket is reachable.
package ha

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL = "http://supervisor/core/api"
	defaultTimeout = 10 * time.Second
)

// Client wraps the HA REST API.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
	Logger  *slog.Logger
}

// NewClient returns a Client with sensible defaults (Supervisor proxy
// base URL).
func NewClient(token string, logger *slog.Logger) *Client {
	return NewClientWithBaseURL("", token, logger)
}

// NewClientWithBaseURL builds a Client with an optional base-URL override.
// Pass baseURL == "" to use the Supervisor proxy default. The token is
// always required; the caller is expected to resolve overrides (e.g.
// config.Options.HARESTToken) before calling.
func NewClientWithBaseURL(baseURL, token string, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTP:    &http.Client{Timeout: defaultTimeout},
		Logger:  logger,
	}
}

// PlayMedia calls media_player.play_media for entityID with the given
// MA-flavored content URI.
func (c *Client) PlayMedia(ctx context.Context, entityID, contentID, contentType string) error {
	if contentType == "" {
		contentType = "music"
	}
	body := map[string]any{
		"entity_id":          entityID,
		"media_content_id":   contentID,
		"media_content_type": contentType,
	}
	c.Logger.Info("ha: play_media", "entity_id", entityID, "content_id", contentID)
	return c.postService(ctx, "media_player/play_media", body)
}

// MediaPlayerCommand issues media_player.<service> with data merged on
// top of {entity_id}.
func (c *Client) MediaPlayerCommand(ctx context.Context, entityID, service string, data map[string]any) error {
	body := map[string]any{"entity_id": entityID}
	maps.Copy(body, data)
	c.Logger.Info("ha: media_player command", "service", service, "entity_id", entityID, "data", data)
	return c.postService(ctx, "media_player/"+service, body)
}

func (c *Client) postService(ctx context.Context, path string, body map[string]any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := c.BaseURL + "/services/" + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("ha: POST %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		snip, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("ha: POST %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(snip)))
	}
	return nil
}

// State is the partial shape we care about from /states/<entity>.
type State struct {
	EntityID   string         `json:"entity_id"`
	State      string         `json:"state"`
	Attributes map[string]any `json:"attributes"`
}

// GetState fetches the current entity state; returns nil on 404.
func (c *Client) GetState(ctx context.Context, entityID string) (*State, error) {
	url := c.BaseURL + "/states/" + entityID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		snip, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("ha: GET state %s: HTTP %d: %s", entityID, resp.StatusCode, strings.TrimSpace(string(snip)))
	}
	var s State
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// FindMAAddonHostname looks up the music_assistant addon's hostname via
// Supervisor. Returns "" if the addon is not installed or the call
// fails — callers should treat that as "MA WS not reachable directly".
//
// The Supervisor /addons endpoint requires the addon to have
// hassio_role: manager (or admin); without it the call returns 403 and
// we cannot auto-discover MA's hostname. We surface the HTTP status at
// warn level so the cause is visible in the addon log instead of being
// silently swallowed.
func (c *Client) FindMAAddonHostname(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://supervisor/addons", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		c.Logger.Warn("ha: addon list request failed — MA auto-discovery skipped",
			"status", resp.StatusCode,
			"hint", "addon may need hassio_role: manager in config.yaml")
		return "", nil
	}
	var envelope struct {
		Data struct {
			Addons []struct {
				Slug     string `json:"slug"`
				Hostname string `json:"hostname"`
			} `json:"addons"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return "", err
	}
	for _, a := range envelope.Data.Addons {
		if strings.Contains(a.Slug, "music_assistant") {
			return a.Hostname, nil
		}
	}
	return "", nil
}
