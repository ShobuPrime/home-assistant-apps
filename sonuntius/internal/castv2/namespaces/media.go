// Maps to: urn:x-cast:com.google.cast.media
//
// The media namespace is the namespace Sonuntius cares about most: it
// carries the LOAD message in which the sender tells the (fake) receiver
// what to play. From a LOAD payload we extract the provider-specific
// track identifier, hand it back to the cmd binary as an Intent, and let
// Music Assistant do the actual playing on the Sendspin speaker.
//
// Wire shape (the subset Phase 3a cares about):
//
//	→ {"type":"LOAD","requestId":N,"sessionId":"<id>",
//	   "media":{
//	     "contentId":"<id>",
//	     "contentType":"<mime>",
//	     "metadata":{...},
//	     "customData":{...}
//	   },
//	   "autoplay":true, "currentTime":0,
//	   "customData":{...}
//	  }
//
//	← {"type":"MEDIA_STATUS","requestId":N,"status":[{
//	     "mediaSessionId":1, "playbackRate":1, "playerState":"PLAYING",
//	     "currentTime":0, "supportedMediaCommands":15, "volume":{...},
//	     "media":{...},
//	     "customData":{...}
//	  }]}
//
// Other inbound types (PAUSE, PLAY, STOP, SEEK, GET_STATUS, QUEUE_*) Phase
// 3a echoes with the same MEDIA_STATUS shape — Phase 3b will route them
// through to the dispatcher as TransportCommand events.
//
// Concrete parsers (Tidal `customData.tidal.trackId` extraction, generic
// `audio/*` URL extraction for the Default Media Receiver fallback) are
// **not** implemented in this file. Phase 3b lands them as Parser
// implementations registered via Server.RegisterParser.
//
// Reference: https://developers.google.com/cast/docs/reference/messages
package namespaces

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
)

// MediaLoad is the decoded form of a LOAD message's `media` field plus the
// top-level `customData`. We surface both flavours of customData (per-load
// and per-media) because Tidal historically embeds the trackId in the
// outer one while Cast SDK convention puts it in the inner one — either
// can carry the identifier and Phase 3b's parser will probe both.
type MediaLoad struct {
	ContentID   string          `json:"contentId"`
	ContentType string          `json:"contentType"`
	StreamType  string          `json:"streamType,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	CustomData  json.RawMessage `json:"customData,omitempty"`

	// OuterCustomData is the LOAD message's top-level `customData`, which
	// Tidal's sender has historically used. Populated by the handler, not
	// by JSON unmarshal directly.
	OuterCustomData json.RawMessage `json:"-"`
}

// Parser is implemented by per-provider extractors. Phase 3b lands Tidal +
// generic-URL implementations; Phase 3a only defines the interface and a
// LogOnlyParser stub that records the payload for debugging.
//
// Parse should return ok=false when the parser does not recognise the
// LOAD (e.g. Tidal parser invoked on a YouTube payload) — the Server
// will try the next registered parser in order.
type Parser interface {
	// Name uniquely identifies the parser in logs ("tidal", "url", ...).
	Name() string
	// Parse inspects load and returns an Intent if the parser recognised
	// the payload. ok=false means "not my responsibility, try the next
	// parser". Returning a non-nil error means "the payload looked like
	// mine but was malformed" and the Server logs it and moves on.
	Parse(load *MediaLoad) (intent ParsedIntent, ok bool, err error)
}

// ParsedIntent is the parser-output shape. We define it here (rather than
// pulling in the parent package's castv2.Intent) so this sub-package has
// zero dependency on the parent — the Server bridges between the two.
type ParsedIntent struct {
	Provider string
	TrackID  string
	URL      string
	Metadata map[string]any
}

// LogOnlyParser is the Phase 3a stub: it never claims a payload, but it
// logs the LOAD content at debug so the maintainer can capture real Tidal
// payloads in the wild and refine the proper Tidal parser in Phase 3b.
type LogOnlyParser struct {
	logger *slog.Logger
}

// NewLogOnlyParser constructs a LogOnlyParser.
func NewLogOnlyParser(logger *slog.Logger) *LogOnlyParser {
	if logger == nil {
		logger = slog.Default()
	}
	return &LogOnlyParser{logger: logger}
}

// Name implements Parser.
func (p *LogOnlyParser) Name() string { return "logonly" }

// Parse implements Parser. Always returns ok=false so the next registered
// parser still gets a shot.
func (p *LogOnlyParser) Parse(load *MediaLoad) (ParsedIntent, bool, error) {
	p.logger.Debug("media: LOAD payload",
		"content_id", load.ContentID,
		"content_type", load.ContentType,
		"stream_type", load.StreamType,
		"metadata_bytes", len(load.Metadata),
		"inner_custom_data_bytes", len(load.CustomData),
		"outer_custom_data_bytes", len(load.OuterCustomData))
	return ParsedIntent{}, false, nil
}

// IntentHandler is the callback the Server invokes when a Parser claims a
// LOAD message. Phase 3b's cmd binary supplies this to translate the
// ParsedIntent into an events.PlayIntent and push it onto the IPC bus.
type IntentHandler func(ctx context.Context, source string, intent ParsedIntent)

// Media is the media-namespace handler.
type Media struct {
	logger *slog.Logger

	mu        sync.Mutex
	parsers   []Parser
	onIntent  IntentHandler
	sessionID atomic.Int64 // monotonic media-session id, bumped on each LOAD

	// playerState tracks the last LOAD's content for MEDIA_STATUS echoes.
	currentMedia json.RawMessage
	playerState  string // PLAYING / PAUSED / IDLE / BUFFERING
}

// NewMedia constructs a Media handler. parsers run in registration order
// until one returns ok=true. onIntent is invoked with the parsed result
// (may be nil — Phase 3a tests pass nil; Phase 3b plumbs through the IPC
// emitter).
func NewMedia(parsers []Parser, onIntent IntentHandler, logger *slog.Logger) *Media {
	if logger == nil {
		logger = slog.Default()
	}
	return &Media{
		logger:      logger,
		parsers:     parsers,
		onIntent:    onIntent,
		playerState: "IDLE",
	}
}

// RegisterParser appends p to the parser list. Used by the Server's
// public RegisterParser method.
func (m *Media) RegisterParser(p Parser) {
	m.mu.Lock()
	m.parsers = append(m.parsers, p)
	m.mu.Unlock()
}

// SetIntentHandler replaces the callback. Phase 3b uses this to wire the
// IPC emitter in after the Media handler has been constructed.
func (m *Media) SetIntentHandler(fn IntentHandler) {
	m.mu.Lock()
	m.onIntent = fn
	m.mu.Unlock()
}

// Handle processes one incoming media-namespace message.
func (m *Media) Handle(ctx context.Context, source string, payload json.RawMessage) (Reply, error) {
	var env struct {
		Type      string          `json:"type"`
		RequestID int64           `json:"requestId"`
		Media     json.RawMessage `json:"media,omitempty"`
		Custom    json.RawMessage `json:"customData,omitempty"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		m.logger.Debug("media: malformed payload", "source", source, "err", err)
		return Reply{}, nil
	}

	switch env.Type {
	case "LOAD":
		m.handleLoad(ctx, source, env.Media, env.Custom)
		m.mu.Lock()
		m.playerState = "PLAYING"
		m.mu.Unlock()
		return m.statusReply(env.RequestID), nil
	case "PAUSE":
		m.mu.Lock()
		m.playerState = "PAUSED"
		m.mu.Unlock()
		return m.statusReply(env.RequestID), nil
	case "PLAY":
		m.mu.Lock()
		m.playerState = "PLAYING"
		m.mu.Unlock()
		return m.statusReply(env.RequestID), nil
	case "STOP":
		m.mu.Lock()
		m.playerState = "IDLE"
		m.currentMedia = nil
		m.mu.Unlock()
		return m.statusReply(env.RequestID), nil
	case "GET_STATUS":
		return m.statusReply(env.RequestID), nil
	default:
		m.logger.Debug("media: ignoring message", "source", source, "type", env.Type)
		return Reply{}, nil
	}
}

// handleLoad decodes the LOAD media block, runs the parser chain, and
// fires the IntentHandler on the first match.
func (m *Media) handleLoad(ctx context.Context, source string, media, outerCustom json.RawMessage) {
	m.sessionID.Add(1)

	load := &MediaLoad{OuterCustomData: outerCustom}
	if len(media) > 0 {
		if err := json.Unmarshal(media, load); err != nil {
			m.logger.Warn("media: LOAD media block unparseable", "source", source, "err", err)
			return
		}
		m.mu.Lock()
		m.currentMedia = append([]byte(nil), media...)
		m.mu.Unlock()
	}

	// Snapshot parsers + intent handler under the lock so re-entrant
	// registrations from inside a parser don't deadlock.
	m.mu.Lock()
	parsers := append([]Parser(nil), m.parsers...)
	onIntent := m.onIntent
	m.mu.Unlock()

	for _, p := range parsers {
		intent, ok, err := p.Parse(load)
		if err != nil {
			m.logger.Warn("media: parser error",
				"parser", p.Name(), "source", source, "err", err)
			continue
		}
		if !ok {
			continue
		}
		m.logger.Info("media: LOAD parsed",
			"parser", p.Name(),
			"provider", intent.Provider,
			"track_id", intent.TrackID,
			"url", intent.URL)
		if onIntent != nil {
			onIntent(ctx, source, intent)
		}
		return
	}
	m.logger.Debug("media: no parser claimed LOAD",
		"content_id", load.ContentID,
		"content_type", load.ContentType)
}

// statusReply builds a MEDIA_STATUS Reply.
func (m *Media) statusReply(requestID int64) Reply {
	m.mu.Lock()
	mediaCopy := m.currentMedia
	state := m.playerState
	sessionID := m.sessionID.Load()
	m.mu.Unlock()

	type status struct {
		MediaSessionID         int64           `json:"mediaSessionId"`
		PlaybackRate           int             `json:"playbackRate"`
		PlayerState            string          `json:"playerState"`
		CurrentTime            int             `json:"currentTime"`
		SupportedMediaCommands int             `json:"supportedMediaCommands"`
		Media                  json.RawMessage `json:"media,omitempty"`
	}
	type envelope struct {
		Type      string   `json:"type"`
		RequestID int64    `json:"requestId"`
		Status    []status `json:"status"`
	}
	env := envelope{
		Type:      "MEDIA_STATUS",
		RequestID: requestID,
		Status: []status{{
			MediaSessionID:         sessionID,
			PlaybackRate:           1,
			PlayerState:            state,
			CurrentTime:            0,
			SupportedMediaCommands: 15, // PLAY+PAUSE+STOP+SEEK
			Media:                  mediaCopy,
		}},
	}
	b, _ := json.Marshal(env)
	return Reply{Payload: b}
}
