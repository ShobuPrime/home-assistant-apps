// Maps to: N/A — Go-only Phase 4 Default Media Receiver fallback parser.
//
// Cast namespace: urn:x-cast:com.google.cast.media (the LOAD message).
//
// The generic parser claims a LOAD whose `contentType` starts with
// `audio/` (the Default Media Receiver convention for streaming an
// arbitrary audio URL) and emits a `url`-provider intent so the
// dispatcher can hand the URL to Music Assistant's URL provider. We
// also claim a LOAD whose `contentType` is empty but whose `contentId`
// looks like a public http(s) URL — some Cast SDK clients omit the
// content type when handing in a URL whose extension already implies
// the codec.
//
// The Tidal parser runs first; this one is the "anything else with an
// audio URL" backstop. The LogOnly parser is registered last and never
// claims, so this parser is the last *useful* parser in the chain.
package parsers

import (
	"log/slog"
	"net/url"
	"strings"

	"github.com/shobuprime/sonuntius/internal/castv2/namespaces"
)

// GenericParser emits a `url`-provider intent for generic audio LOADs.
type GenericParser struct {
	logger *slog.Logger
}

// NewGeneric constructs a GenericParser. logger may be nil.
func NewGeneric(logger *slog.Logger) *GenericParser {
	if logger == nil {
		logger = slog.Default()
	}
	return &GenericParser{logger: logger}
}

// Name implements namespaces.Parser.
func (p *GenericParser) Name() string { return "url" }

// Parse implements namespaces.Parser. Returns ok=true when the LOAD
// looks like an audio URL we can hand to MA's URL provider.
func (p *GenericParser) Parse(load *namespaces.MediaLoad) (namespaces.ParsedIntent, bool, error) {
	contentID := strings.TrimSpace(load.ContentID)
	if contentID == "" {
		return namespaces.ParsedIntent{}, false, nil
	}

	ct := strings.ToLower(strings.TrimSpace(load.ContentType))
	switch {
	case strings.HasPrefix(ct, "audio/"):
		// Standard Default Media Receiver path. Accept any URL the sender
		// hands us; MA's URL provider can negotiate the actual stream.
		if isURL(contentID) {
			p.logger.Debug("generic: claiming audio URL",
				"content_type", ct, "url", contentID)
			return namespaces.ParsedIntent{
				Provider: "url",
				URL:      contentID,
			}, true, nil
		}
		// audio/* with a non-URL contentId is unusual; emit it anyway —
		// downstream can refuse if it can't resolve.
		p.logger.Debug("generic: audio contentType but non-URL contentId — claiming",
			"content_id", contentID)
		return namespaces.ParsedIntent{
			Provider: "url",
			URL:      contentID,
		}, true, nil

	case ct == "":
		// No content type, but the contentId is a plain http(s) URL —
		// treat as audio.
		if isURL(contentID) {
			p.logger.Debug("generic: empty contentType, URL-shaped contentId — claiming",
				"url", contentID)
			return namespaces.ParsedIntent{
				Provider: "url",
				URL:      contentID,
			}, true, nil
		}
	}

	return namespaces.ParsedIntent{}, false, nil
}

// isURL reports whether s parses as an absolute http(s) URL with a host.
func isURL(s string) bool {
	if !strings.HasPrefix(strings.ToLower(s), "http://") &&
		!strings.HasPrefix(strings.ToLower(s), "https://") {
		return false
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return u.Host != ""
}
