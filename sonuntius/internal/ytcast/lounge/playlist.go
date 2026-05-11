// Maps to: src/lib/app/Playlist.ts
//
// Playlist is the in-memory queue of videos the receiver is playing through.
// Upstream's class is an EventEmitter; the Go port preserves the same
// observable mutations through a callback-based handler (PlaylistListener)
// plus an event-bus publication for the playlist-level events.
//
// The previous/next computation is delegated to a PlaylistRequestHandler
// (see playlistreq.go). When the current track moves, Playlist asks the
// handler for the neighbouring videos with an abortable context — a new
// move cancels the previous request before issuing a new one.
package lounge

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/shobuprime/sonuntius/internal/ytcast/constants"
	"github.com/shobuprime/sonuntius/internal/ytcast/logger"
	"github.com/shobuprime/sonuntius/internal/ytcast/types"
)

// PlaylistEventType ports the upstream `PLAYLIST_EVENT_TYPES` string set.
type PlaylistEventType string

const (
	PlaylistEventVideoSelected   PlaylistEventType = "videoSelected"
	PlaylistEventVideoAdded      PlaylistEventType = "videoAdded"
	PlaylistEventVideoRemoved    PlaylistEventType = "videoRemoved"
	PlaylistEventPlaylistSet     PlaylistEventType = "playlistSet"
	PlaylistEventPlaylistAdded   PlaylistEventType = "playlistAdded"
	PlaylistEventPlaylistCleared PlaylistEventType = "playlistCleared"
	PlaylistEventPlaylistUpdated PlaylistEventType = "playlistUpdated"
)

// PlaylistState ports upstream's `interface PlaylistState`.
type PlaylistState struct {
	ID       string       `json:"id,omitempty"`
	VideoIDs []string     `json:"videoIds"`
	Previous *types.Video `json:"previous,omitempty"`
	Current  *types.Video `json:"current,omitempty"`
	Next     *types.Video `json:"next,omitempty"`
	Autoplay *types.Video `json:"autoplay,omitempty"`
}

// PlaylistEvent ports upstream's `interface PlaylistEvent`.
type PlaylistEvent struct {
	Type     PlaylistEventType  `json:"type"`
	VideoID  string             `json:"videoId,omitempty"`
	VideoIDs []string           `json:"videoIds,omitempty"`
	User     *PlaylistEventUser `json:"user,omitempty"`
}

// PlaylistEventUser ports upstream's `{name, thumbnail}` shape inside the
// playlist event payload.
type PlaylistEventUser struct {
	Name      string `json:"name"`
	Thumbnail string `json:"thumbnail"`
}

// AutoplayModeChange ports the `(previous, current)` pair upstream emits
// on autoplay-mode changes. The lounge package surfaces autoplay
// transitions through PlaylistListener.OnAutoplayModeChange so the
// orchestrator can react and re-render the autoplay button.
type AutoplayModeChange struct {
	Previous constants.AutoplayMode
	Current  constants.AutoplayMode
}

// PlaylistListener is the Go-idiomatic translation of upstream's
// per-event EventEmitter API. Implementations may set zero or more
// callbacks; nil callbacks are skipped.
type PlaylistListener struct {
	OnAutoplayModeChange func(change AutoplayModeChange)
	OnPlaylistEvent      func(event PlaylistEvent)
}

// Playlist ports the upstream `class Playlist`.
type Playlist struct {
	mu sync.Mutex

	id           string
	videoIDs     []string
	current      *types.Video
	previous     *types.Video
	next         *types.Video
	autoplayMode constants.AutoplayMode
	log          logger.Logger
	handler      PlaylistRequestHandler
	listener     PlaylistListener

	// previousNextCancel cancels any in-flight neighbour-refresh
	// request. A new refresh sets a new cancel before issuing the
	// request, so subsequent reads after a move see the freshest
	// neighbours.
	previousNextCancel context.CancelFunc
	refreshing         bool
}

// NewPlaylist constructs an empty Playlist with the default autoplay mode
// of UNSUPPORTED — matching upstream's constructor.
func NewPlaylist() *Playlist {
	return &Playlist{
		autoplayMode: constants.AutoplayModeUnsupported,
	}
}

// SetLogger ports `setLogger`.
func (p *Playlist) SetLogger(l logger.Logger) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.log = l
}

// SetRequestHandler ports `setRequestHandler`.
func (p *Playlist) SetRequestHandler(h PlaylistRequestHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handler = h
}

// SetListener attaches a listener. Replaces any previously set listener.
func (p *Playlist) SetListener(l PlaylistListener) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.listener = l
}

// SetAutoplayMode ports `setAutoplayMode(value)`.
func (p *Playlist) SetAutoplayMode(ctx context.Context, value constants.AutoplayMode) error {
	p.mu.Lock()
	old := p.autoplayMode
	p.autoplayMode = value
	listener := p.listener
	p.mu.Unlock()

	if old != value {
		if err := p.refreshPreviousNext(ctx); err != nil {
			return err
		}
		if listener.OnAutoplayModeChange != nil {
			listener.OnAutoplayModeChange(AutoplayModeChange{Previous: old, Current: value})
		}
	}
	return nil
}

// UpdateByMessage ports `updateByMessage(message, client)`. Returns the
// derived PlaylistEvent that was emitted, mostly for tests; production
// callers can ignore it.
func (p *Playlist) UpdateByMessage(ctx context.Context, message *Message, client types.Client) (*PlaylistEvent, error) {
	if message == nil {
		return nil, nil
	}
	if message.Name != "setPlaylist" && message.Name != "updatePlaylist" {
		return nil, nil
	}
	payload := message.PayloadAsMap()
	if payload == nil {
		return nil, nil
	}

	listID, _ := payload["listId"].(string)
	if listID == "" {
		p.mu.Lock()
		p.id = ""
		p.current = nil
		p.next = nil
		p.previous = nil
		p.videoIDs = nil
		listener := p.listener
		p.mu.Unlock()
		evt := &PlaylistEvent{Type: PlaylistEventPlaylistCleared}
		if listener.OnPlaylistEvent != nil {
			listener.OnPlaylistEvent(*evt)
		}
		return evt, nil
	}

	p.mu.Lock()
	p.id = listID
	if rawIDs, _ := payload["videoIds"].(string); rawIDs != "" {
		p.videoIDs = strings.Split(rawIDs, ",")
	} else {
		p.videoIDs = nil
	}

	switch message.Name {
	case "setPlaylist":
		idxStr, _ := payload["currentIndex"].(string)
		videoID, _ := payload["videoId"].(string)
		// upstream: `if (data.currentIndex && data.videoId)`. JS-truthy
		// treats "0" as truthy but 0 as falsy — and currentIndex is sent
		// as a string. Mirror that by requiring a non-empty string.
		if idxStr != "" && videoID != "" {
			idx, err := strconv.Atoi(idxStr)
			if err == nil {
				ctx := &types.VideoContext{
					PlaylistID: listID,
				}
				idxCopy := idx
				ctx.Index = &idxCopy
				if ctt, ok := payload["ctt"].(string); ok && ctt != "" {
					ctx.CTT = ctt
				}
				if params, ok := payload["params"].(string); ok && params != "" {
					ctx.Params = params
				}
				p.current = &types.Video{
					ID:      videoID,
					Client:  client,
					Context: ctx,
				}
			} else {
				p.current = nil
			}
		} else {
			p.current = nil
		}

	case "updatePlaylist":
		if p.current != nil {
			idx := slices.IndexFunc(p.videoIDs, func(v string) bool { return v == p.current.ID })
			if idx >= 0 {
				if p.current.Context == nil {
					p.current.Context = &types.VideoContext{}
				}
				p.current.Context.PlaylistID = listID
				ic := idx
				p.current.Context.Index = &ic
				if params, ok := payload["params"].(string); ok && params != "" {
					p.current.Context.Params = params
				} else {
					p.current.Context.Params = ""
				}
			} else {
				p.current = nil
			}
		}
	}
	p.mu.Unlock()

	if err := p.refreshPreviousNext(ctx); err != nil {
		return nil, err
	}

	// Build the emitted event from `eventDetails` if present, else fall
	// back to the `setPlaylist` / `updatePlaylist` default.
	var event *PlaylistEvent
	if raw, ok := payload["eventDetails"].(string); ok && raw != "" {
		event = parseEventDetails(raw, p.log)
	}
	if event == nil {
		t := PlaylistEventPlaylistUpdated
		if message.Name == "setPlaylist" {
			t = PlaylistEventPlaylistSet
		}
		p.mu.Lock()
		ids := append([]string(nil), p.videoIDs...)
		p.mu.Unlock()
		event = &PlaylistEvent{Type: t, VideoIDs: ids}
	}

	p.mu.Lock()
	listener := p.listener
	p.mu.Unlock()
	if listener.OnPlaylistEvent != nil {
		listener.OnPlaylistEvent(*event)
	}
	return event, nil
}

// eventDetailsRaw mirrors the JSON shape upstream parses out of the
// `eventDetails` field of a setPlaylist / updatePlaylist payload.
type eventDetailsRaw struct {
	EventType     string   `json:"eventType"`
	User          string   `json:"user"`
	UserAvatarURI string   `json:"userAvatarUri"`
	VideoID       string   `json:"videoId"`
	VideoIDs      []string `json:"videoIds"`
}

func parseEventDetails(raw string, l logger.Logger) *PlaylistEvent {
	var details eventDetailsRaw
	if err := json.Unmarshal([]byte(raw), &details); err != nil {
		if l != nil {
			l.Error("[yt-cast-receiver] Failed to parse playlist eventDetails:", raw, err)
		}
		return nil
	}
	var emit PlaylistEventType
	switch details.EventType {
	case "PLAYLIST_ADDED":
		emit = PlaylistEventPlaylistAdded
	case "PLAYLIST_CLEARED":
		emit = PlaylistEventPlaylistCleared
	case "PLAYLIST_SET":
		emit = PlaylistEventPlaylistSet
	case "VIDEO_ADDED":
		emit = PlaylistEventVideoAdded
	case "VIDEO_REMOVED":
		emit = PlaylistEventVideoRemoved
	case "VIDEO_SELECTED":
		emit = PlaylistEventVideoSelected
	default:
		return nil
	}
	evt := &PlaylistEvent{Type: emit}
	if details.User != "" {
		evt.User = &PlaylistEventUser{Name: details.User, Thumbnail: details.UserAvatarURI}
	}
	if details.VideoID != "" {
		evt.VideoID = details.VideoID
	}
	if len(details.VideoIDs) > 0 {
		evt.VideoIDs = details.VideoIDs
	}
	return evt
}

// refreshPreviousNext ports `#refreshPreviousNext`. It is safe to call
// repeatedly — a previous in-flight refresh is cancelled.
func (p *Playlist) refreshPreviousNext(parent context.Context) error {
	p.mu.Lock()
	if p.previousNextCancel != nil {
		p.previousNextCancel()
		p.previousNextCancel = nil
	}
	p.previous = nil
	p.next = nil
	cur := p.current
	handler := p.handler
	p.mu.Unlock()

	idx := -1
	if cur != nil && cur.Context != nil && cur.Context.Index != nil {
		idx = *cur.Context.Index
	}
	if idx < 0 || cur == nil || handler == nil {
		return nil
	}

	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	p.mu.Lock()
	p.previousNextCancel = cancel
	p.refreshing = true
	p.mu.Unlock()

	nav, err := GetPreviousNextVideosAbortable(ctx, handler, *cur, p)

	p.mu.Lock()
	p.previousNextCancel = nil
	p.refreshing = false
	if err != nil {
		if errors.Is(err, ErrPlaylistRequestAborted) || errors.Is(err, context.Canceled) {
			p.mu.Unlock()
			return nil
		}
		p.mu.Unlock()
		return err
	}
	p.previous = nav.Previous
	if !p.isLastLocked() || p.autoplayMode == constants.AutoplayModeEnabled {
		p.next = nav.Next
	}
	p.mu.Unlock()
	return nil
}

// Reset ports `reset()`.
func (p *Playlist) Reset() {
	p.mu.Lock()
	p.id = ""
	p.videoIDs = nil
	p.current = nil
	p.previous = nil
	p.autoplayMode = constants.AutoplayModeUnsupported
	handler := p.handler
	p.mu.Unlock()
	if handler != nil {
		handler.Reset()
	}
}

// ID returns the current playlist id.
func (p *Playlist) ID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.id
}

// VideoIDs returns the current ordered list of video ids. A copy is
// returned so callers can mutate freely.
func (p *Playlist) VideoIDs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.videoIDs) == 0 {
		return nil
	}
	return append([]string(nil), p.videoIDs...)
}

// Length returns the number of videos in the playlist.
func (p *Playlist) Length() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.videoIDs)
}

// Previous ports `previous()`. Moves current to the cached previous and
// refreshes neighbours.
func (p *Playlist) Previous(ctx context.Context) (*types.Video, error) {
	p.mu.Lock()
	if p.previous == nil {
		p.mu.Unlock()
		return nil, nil
	}
	p.current = p.previous
	cur := p.current
	p.mu.Unlock()
	if err := p.refreshPreviousNext(ctx); err != nil {
		return nil, err
	}
	return cur, nil
}

// Next ports `next()`.
func (p *Playlist) Next(ctx context.Context) (*types.Video, error) {
	p.mu.Lock()
	if !p.hasNextLocked() {
		p.mu.Unlock()
		return nil, nil
	}
	p.current = p.next
	cur := p.current
	p.mu.Unlock()
	if err := p.refreshPreviousNext(ctx); err != nil {
		return nil, err
	}
	return cur, nil
}

// GetState ports `getState()`.
func (p *Playlist) GetState() PlaylistState {
	p.mu.Lock()
	defer p.mu.Unlock()
	state := PlaylistState{
		ID:       p.id,
		VideoIDs: append([]string(nil), p.videoIDs...),
		Previous: p.previous,
		Current:  p.current,
	}
	if p.hasNextLocked() {
		state.Next = p.next
	}
	state.Autoplay = p.autoplayLocked()
	return state
}

// SetAsCurrent ports `setAsCurrent(video)`.
func (p *Playlist) SetAsCurrent(video types.Video) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current != nil && p.current.ID == video.ID {
		return
	}
	if a := p.autoplayLocked(); a != nil && a.ID == video.ID {
		p.next = nil
	}
	v := video
	p.current = &v
}

// Current returns the current video.
func (p *Playlist) Current() *types.Video {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current
}

// AutoplayMode returns the configured autoplay mode.
func (p *Playlist) AutoplayMode() constants.AutoplayMode {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.autoplayMode
}

// Autoplay returns the autoplay candidate if any.
func (p *Playlist) Autoplay() *types.Video {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.autoplayLocked()
}

// HasPrevious returns whether a previous neighbour exists.
func (p *Playlist) HasPrevious() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.previous != nil
}

// HasNext returns whether a next neighbour exists (and we are not on the
// last queue index — the autoplay neighbour does not count as "next").
func (p *Playlist) HasNext() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hasNextLocked()
}

// IsLast returns whether the current index is the last queue position.
func (p *Playlist) IsLast() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.isLastLocked()
}

// IsUpdating reports whether a neighbour-refresh is in flight.
func (p *Playlist) IsUpdating() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.refreshing
}

// RequestHandler exposes the registered handler — mostly for tests.
func (p *Playlist) RequestHandler() PlaylistRequestHandler {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.handler
}

func (p *Playlist) isLastLocked() bool {
	idx := -1
	if p.current != nil && p.current.Context != nil && p.current.Context.Index != nil {
		idx = *p.current.Context.Index
	}
	return idx < 0 || idx == len(p.videoIDs)-1
}

func (p *Playlist) hasNextLocked() bool {
	if p.next == nil {
		return false
	}
	if p.isLastLocked() {
		return false
	}
	return true
}

func (p *Playlist) autoplayLocked() *types.Video {
	if p.isLastLocked() {
		return p.next
	}
	return nil
}
