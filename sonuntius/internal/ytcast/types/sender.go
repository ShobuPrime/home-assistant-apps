// Maps to: src/lib/app/Sender.ts
//
// Sender holds the metadata about a connected sender (the phone / desktop on
// the other side of the lounge channel). Upstream constructs it from the
// payload of `remoteConnected` / `remoteDisconnected` messages; we expose the
// same Parse helper plus the same SupportsAutoplay / SupportsMute helpers.
package types

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/shobuprime/sonuntius/internal/ytcast/yterrors"
)

// SenderUser mirrors the upstream `user?: { name, thumbnail }` field.
type SenderUser struct {
	Name      string `json:"name"`
	Thumbnail string `json:"thumbnail"`
}

// Sender ports the upstream `class Sender`.
type Sender struct {
	ID                     string         `json:"id"`
	Name                   string         `json:"name"`
	App                    string         `json:"app,omitempty"`
	Client                 *Client        `json:"client,omitempty"`
	Capabilities           []string       `json:"capabilities"`
	Device                 map[string]any `json:"device"`
	User                   *SenderUser    `json:"user,omitempty"`
	ObfuscatedGaiaID       string         `json:"obfuscatedGaiaId,omitempty"`
	OwnerObfuscatedGaiaID  string         `json:"ownerObfuscatedGaiaId,omitempty"`
}

// senderRaw is the loose shape the lounge protocol delivers. Fields are
// optional because upstream silently tolerates missing ones; Parse below
// applies the same fallbacks.
type senderRaw struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	ClientName            string `json:"clientName"`
	App                   string `json:"app"`
	Theme                 string `json:"theme"`
	Capabilities          string `json:"capabilities"`
	Device                string `json:"device"`
	User                  string `json:"user"`
	UserAvatarURI         string `json:"userAvatarUri"`
	ObfuscatedGaiaID      string `json:"obfuscatedGaiaId"`
	OwnerObfuscatedGaiaID string `json:"ownerObfuscatedGaiaId"`
}

// Parse ports the static `Sender.parse(data)` factory. It rejects payloads
// missing `id`, both `name` and `clientName`, or with a `theme` that doesn't
// match a known Client. Returns a *DataError on rejection (matching upstream
// `throw new DataError('Invalid data', undefined, data)`).
func Parse(data json.RawMessage) (*Sender, error) {
	var raw senderRaw
	if len(data) > 0 {
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, yterrors.NewDataError("Invalid data", err, json.RawMessage(append([]byte{}, data...)))
		}
	}

	_, themeOK := ClientByTheme(raw.Theme)
	if raw.ID == "" || (raw.Name == "" && raw.ClientName == "") || !themeOK {
		return nil, yterrors.NewDataError("Invalid data", nil, json.RawMessage(append([]byte{}, data...)))
	}
	return newSender(raw), nil
}

// newSender ports the upstream constructor — same field ordering, same
// fallback rules.
func newSender(raw senderRaw) *Sender {
	s := &Sender{
		ID:                    raw.ID,
		Name:                  firstNonEmpty(raw.Name, raw.ClientName),
		Capabilities:          splitCSV(raw.Capabilities),
		App:                   raw.App,
		ObfuscatedGaiaID:      raw.ObfuscatedGaiaID,
		OwnerObfuscatedGaiaID: raw.OwnerObfuscatedGaiaID,
	}
	if c, ok := ClientByTheme(raw.Theme); ok {
		client := c
		s.Client = &client
	}
	if raw.User != "" {
		s.User = &SenderUser{Name: raw.User, Thumbnail: raw.UserAvatarURI}
	}
	if raw.Device != "" {
		var device map[string]any
		if err := json.Unmarshal([]byte(raw.Device), &device); err == nil {
			s.Device = device
		} else {
			s.Device = map[string]any{}
		}
	} else {
		s.Device = map[string]any{}
	}
	return s
}

// SupportsAutoplay ports `supportsAutoplay()` — sender capabilities include
// the `atp` token.
func (s *Sender) SupportsAutoplay() bool {
	return slices.Contains(s.Capabilities, "atp")
}

// SupportsMute ports `supportsMute()` — desktop senders (`youtube-desktop`,
// `youtube.m-desktop`) honour mute commands.
func (s *Sender) SupportsMute() bool {
	return s.App != "" && strings.HasSuffix(s.App, "-desktop")
}

// firstNonEmpty returns the first non-empty string in the argument list.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// splitCSV mirrors the upstream `data.capabilities?.split(',') || []`.
func splitCSV(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, ",")
}
