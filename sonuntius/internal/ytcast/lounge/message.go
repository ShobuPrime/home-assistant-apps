// Maps to: src/lib/app/Message.ts
//
// Message is the wire-level unit of the lounge protocol. Incoming frames
// from the `bind` long-poll arrive as one or more JSON arrays of the form
//
//	[<AID>,["<name>"[,<payload>]]]
//
// occasionally concatenated on a single line. Upstream parses them with a
// global regex:
//
//	/\[(\d+),\["(.+?)"(?:,(.*?))?\]\]/g
//
// Go's regexp engine is RE2 with no backtracking, but this pattern uses
// only the features RE2 supports (literal chars, capture groups, a lazy
// `.+?` quantifier inside a single capture, and a non-capturing optional
// group). The Go translation below preserves the same capture-group
// semantics:
//
//   - group 1 → AID (decimal digits)
//   - group 2 → name (everything up to the next `"`)
//   - group 3 → optional payload tail; may be absent if the message has no
//     payload (`[123,["loungeStatus"]]` style) or be a JSON value
//     (object, array, number, string, bool, null).
//
// We keep the lazy `.+?` for the payload because the wire frames are
// usually concatenated by the server with no separator — a greedy match
// would swallow the next frame's closing bracket.
package lounge

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/shobuprime/sonuntius/internal/ytcast/constants"
	pkgplayer "github.com/shobuprime/sonuntius/internal/ytcast/player"
)

// Message ports the upstream `class Message`. AID is the action identifier
// the receiver echoes back when responding; a nil pointer represents
// upstream's `null` (no AID associated).
type Message struct {
	AID     *int
	Name    string
	Payload any
}

// NewMessage constructs a Message with the given AID (may be nil), name,
// and payload. A nil payload is normalized to an empty map, matching
// upstream's `payload || {}` fallback.
func NewMessage(aid *int, name string, payload any) *Message {
	if payload == nil {
		payload = map[string]any{}
	}
	return &Message{AID: aid, Name: name, Payload: payload}
}

// messageRegex is the Go port of the upstream
// `/\[(\d+),\["(.+?)"(?:,(.*?))?\]\]/g` pattern. RE2 supports lazy
// quantifiers, so the translation is byte-for-byte modulo the escape
// rules.
var messageRegex = regexp.MustCompile(`\[(\d+),\["([^"]+)"(?:,(.*?))?\]\]`)

// removeNewlines mirrors upstream's `newline-remove` invocation before
// running the regex. We collapse \r and \n and combinations thereof to an
// empty string — the protocol delimits frames with both, and we want the
// regex to see the body as one long line.
func removeNewlines(s string) string {
	if !strings.ContainsAny(s, "\r\n") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\r' || r == '\n' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ParseIncoming ports `Message.parseIncoming`. It returns every well-formed
// message frame found in data, in order. A malformed frame (e.g. payload
// fragment that fails to parse as JSON) is skipped — matching upstream's
// behaviour of advancing the regex past invalid matches instead of
// throwing.
func ParseIncoming(data string) []*Message {
	clean := removeNewlines(data)
	matches := messageRegex.FindAllStringSubmatchIndex(clean, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]*Message, 0, len(matches))
	for _, m := range matches {
		// m is a flat slice of pairs: [matchStart, matchEnd,
		// g1Start, g1End, g2Start, g2End, g3Start, g3End].
		if len(m) < 8 {
			continue
		}
		aidRaw := substr(clean, m[2], m[3])
		name := substr(clean, m[4], m[5])
		aidVal, err := strconv.Atoi(aidRaw)
		if err != nil {
			continue
		}
		aid := aidVal
		msg := &Message{AID: &aid, Name: name, Payload: nil}
		if m[6] != -1 && m[7] != -1 {
			tail := substr(clean, m[6], m[7])
			payload, ok := decodePayload(tail)
			if !ok {
				// Upstream's `JSON.parse` would throw and propagate; we
				// approximate by emitting the message with a nil payload
				// and letting downstream logging surface the malformed
				// tail. Matches upstream behaviour for "no payload"
				// frames more closely than dropping the entire match.
				out = append(out, msg)
				continue
			}
			msg.Payload = payload
		}
		out = append(out, msg)
	}
	return out
}

// substr returns clean[start:end], guarding against the (-1, -1) "no
// match" indicator that FindAllStringSubmatchIndex emits for absent groups.
func substr(s string, start, end int) string {
	if start < 0 || end < 0 || start > end || end > len(s) {
		return ""
	}
	return s[start:end]
}

// decodePayload ports the upstream
//
//	const payload = JSON.parse(`[${m[3]}]`);
//	if (Array.isArray(payload) && payload.length === 1) {
//	  cmd.payload = payload[0];
//	}
//	else {
//	  cmd.payload = payload;
//	}
//
// trick: wrap the matched tail in `[]` so commas in the tail produce a
// JSON array, then unwrap if there's exactly one element. This means a
// trailing-comma free single value (`{"foo":1}`) becomes the parsed
// object, while `1,2,3` becomes `[1,2,3]`.
func decodePayload(tail string) (any, bool) {
	wrapped := "[" + tail + "]"
	var arr []any
	if err := json.Unmarshal([]byte(wrapped), &arr); err != nil {
		return nil, false
	}
	if len(arr) == 1 {
		return arr[0], true
	}
	return arr, true
}

// PayloadAsMap is a convenience accessor for the common case where
// payload is an object. Returns nil if payload is not a map.
func (m *Message) PayloadAsMap() map[string]any {
	if m == nil {
		return nil
	}
	if v, ok := m.Payload.(map[string]any); ok {
		return v
	}
	return nil
}

// PayloadAsArray returns the payload as []any if it is one. Returns nil
// otherwise.
func (m *Message) PayloadAsArray() []any {
	if m == nil {
		return nil
	}
	if v, ok := m.Payload.([]any); ok {
		return v
	}
	return nil
}

// PayloadAsString returns the payload as string if it is one. Returns
// "" otherwise.
func (m *Message) PayloadAsString() string {
	if m == nil {
		return ""
	}
	if v, ok := m.Payload.(string); ok {
		return v
	}
	return ""
}

// String renders a debug-friendly representation. Useful in tests and
// logger.Debug output.
func (m *Message) String() string {
	if m == nil {
		return "<nil Message>"
	}
	aid := "<nil>"
	if m.AID != nil {
		aid = strconv.Itoa(*m.AID)
	}
	return fmt.Sprintf("Message{AID:%s Name:%q Payload:%v}", aid, m.Name, m.Payload)
}

// Serialize renders the message as the wire array
// `[AID,["name",payload]]`. AID nil serializes to `null` to match
// upstream's JSON output. payload `nil` is omitted (matching upstream's
// `payload?` optional position).
func (m *Message) Serialize() (string, error) {
	if m == nil {
		return "", nil
	}
	parts := []any{m.Name}
	if m.Payload != nil {
		parts = append(parts, m.Payload)
	}
	var aid any
	if m.AID != nil {
		aid = *m.AID
	}
	out := []any{aid, parts}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ----- Convenience constructors for outgoing messages ----------------
//
// Upstream nests these as static classes inside the `Message` namespace
// (`Message.NowPlaying`, `Message.OnStateChange`, ...). Go has no nested
// types, so we expose them as factory functions whose names mirror the
// upstream class names.

// PlaylistView is the minimal projection of a Playlist that the outgoing
// `nowPlaying` / `onStateChange` constructors need. We accept this as a
// view rather than a *Playlist to keep this file decoupled from
// playlist.go's concrete state-fetching API (the orchestrator already has
// the snapshot in hand when it builds messages).
type PlaylistView struct {
	Current *Video
}

// Video is the minimal video projection the message constructors need.
// It mirrors the fields upstream reads off `playerState.queue.current`.
type Video struct {
	ID      string
	Context *VideoContext
}

// VideoContext mirrors the fields upstream reads off `current.context`
// when constructing a nowPlaying payload.
type VideoContext struct {
	PlaylistID string
	Index      *int
	CTT        string
	Params     string
}

// PlayerStateView is the minimal projection of the player state the
// outgoing message constructors need.
type PlayerStateView struct {
	Status   constants.PlayerStatus
	Position float64
	Duration float64
	CPN      string
	Queue    PlaylistView
}

// NewNowPlaying ports `Message.NowPlaying`. AID may be nil; pass a non-nil
// pointer to echo an incoming AID. state is the current player snapshot
// (nil for an idle "no video" message — yields an empty payload, matching
// upstream's `{}` default).
func NewNowPlaying(aid *int, state *PlayerStateView) *Message {
	payload := map[string]any{}
	if state != nil && state.Queue.Current != nil {
		cur := state.Queue.Current
		payload["currentTime"] = state.Position
		payload["duration"] = state.Duration
		payload["cpn"] = state.CPN
		payload["loadedTime"] = 0
		payload["videoId"] = cur.ID
		payload["state"] = int(state.Status)
		payload["seekableStartTime"] = 0
		payload["seekableEndTime"] = state.Duration

		if cur.Context != nil {
			if cur.Context.PlaylistID != "" {
				payload["listId"] = cur.Context.PlaylistID
			}
			if cur.Context.Index != nil {
				payload["currentIndex"] = *cur.Context.Index
			}
			if cur.Context.CTT != "" {
				payload["ctt"] = cur.Context.CTT
			}
			if cur.Context.Params != "" {
				payload["params"] = cur.Context.Params
			}
		}
	}
	if state != nil && (state.Status == constants.PlayerStatusPlaying ||
		state.Status == constants.PlayerStatusPaused ||
		state.Status == constants.PlayerStatusLoading) {
		payload["loadedTime"] = state.Duration
	}
	return &Message{AID: aid, Name: "nowPlaying", Payload: payload}
}

// NewOnStateChange ports `Message.OnStateChange`. state must be non-nil
// (matches upstream where the constructor dereferences without a guard).
func NewOnStateChange(aid *int, state PlayerStateView) *Message {
	payload := map[string]any{
		"state":             int(state.Status),
		"currentTime":       state.Position,
		"duration":          state.Duration,
		"loadedTime":        0,
		"seekableStartTime": 0,
		"seekableEndTime":   state.Duration,
		"cpn":               state.CPN,
	}
	if state.Status == constants.PlayerStatusPlaying ||
		state.Status == constants.PlayerStatusPaused ||
		state.Status == constants.PlayerStatusLoading {
		payload["loadedTime"] = state.Duration
	}
	return &Message{AID: aid, Name: "onStateChange", Payload: payload}
}

// NewOnVolumeChanged ports `Message.OnVolumeChanged`.
func NewOnVolumeChanged(aid *int, volume pkgplayer.Volume) *Message {
	return &Message{
		AID:  aid,
		Name: "onVolumeChanged",
		Payload: map[string]any{
			"volume": volume.Level,
			"muted":  volume.Muted,
		},
	}
}

// AutoplayInfo carries either a bare AutoplayMode value or a struct shaped
// like upstream's `PlayerNavInfo` (which also exposes `autoplayMode`). nil
// produces the `UNSUPPORTED` default upstream uses for `info === null`.
type AutoplayInfo struct {
	Mode constants.AutoplayMode
}

// NewOnAutoplayModeChanged ports `Message.OnAutoplayModeChanged`. info
// nil → `UNSUPPORTED`; otherwise info.Mode is used verbatim.
func NewOnAutoplayModeChanged(aid *int, info *AutoplayInfo) *Message {
	mode := constants.AutoplayModeUnsupported
	if info != nil {
		mode = info.Mode
	}
	return &Message{
		AID:  aid,
		Name: "onAutoplayModeChanged",
		Payload: map[string]any{
			"autoplayMode": string(mode),
		},
	}
}

// PlayerNavInfo mirrors upstream's `interface PlayerNavInfo` for the
// subset of fields the outgoing `OnHasPreviousNextChanged` constructor
// needs.
type PlayerNavInfo struct {
	HasPrevious bool
	HasNext     bool
}

// NewOnHasPreviousNextChanged ports `Message.OnHasPreviousNextChanged`.
// nil info collapses to {hasPrevious:false, hasNext:false} — matching
// upstream's `!!playerNavInfo?.hasPrevious` short-circuit.
func NewOnHasPreviousNextChanged(aid *int, info *PlayerNavInfo) *Message {
	hasPrev := false
	hasNext := false
	if info != nil {
		hasPrev = info.HasPrevious
		hasNext = info.HasNext
	}
	return &Message{
		AID:  aid,
		Name: "onHasPreviousNextChanged",
		Payload: map[string]any{
			"hasPrevious": hasPrev,
			"hasNext":     hasNext,
		},
	}
}

// NewAutoplayUpNext ports `Message.AutoplayUpNext`. videoID == "" means
// "no autoplay video" — upstream emits a null payload in that case; we
// emit nil which Message.Serialize will omit.
func NewAutoplayUpNext(aid *int, videoID string) *Message {
	if videoID == "" {
		return &Message{AID: aid, Name: "autoplayUpNext", Payload: nil}
	}
	return &Message{
		AID:  aid,
		Name: "autoplayUpNext",
		Payload: map[string]any{
			"videoId": videoID,
		},
	}
}

// NewLoungeScreenDisconnected ports `Message.LoungeScreenDisconnected`.
// Upstream uses AID null for this message.
func NewLoungeScreenDisconnected() *Message {
	return &Message{AID: nil, Name: "loungeScreenDisconnected", Payload: map[string]any{}}
}
