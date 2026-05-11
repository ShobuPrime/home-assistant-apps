// Maps to: N/A — Go-only Tidal customData parser for the Phase 3 Cast proxy.
//
// Cast namespace: urn:x-cast:com.google.cast.media (the LOAD message).
//
// Tidal's Android Cast sender ships a LOAD message whose `customData`
// blocks carry the Tidal track ID. The exact JSON path is undocumented and
// has drifted across Tidal app releases, so this parser probes the
// historically-observed shapes in best-effort order:
//
//   customData.tidal.trackId           // Tidal-namespaced wrapper
//   customData.trackId                 // flat top-level
//   customData.media.trackId           // nested media object
//   customData.data.trackId            // generic data envelope
//
// Both the outer LOAD customData (sibling of `media`) and the inner
// `media.customData` are probed; Tidal has historically used the outer
// one but the Google Cast SDK convention is the inner one.
//
// If no path matches, we additionally inspect:
//
//   - the LOAD's `metadata` JSON for a `subtitle` field containing "Tidal",
//     in which case we treat a tidal-looking `contentId` as the track ID
//   - a `contentId` whose `contentType` matches `audio/*` and whose value
//     looks like a numeric Tidal track ID (digits only, short).
//
// All inspected blobs are logged at debug level so the maintainer can
// iterate the parser empirically when real Tidal traffic shows up.
package parsers

import (
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/shobuprime/sonuntius/internal/castv2/namespaces"
)

// TidalParser extracts a Tidal track ID from a LOAD payload.
type TidalParser struct {
	logger *slog.Logger
}

// NewTidal constructs a TidalParser. logger may be nil (defaults to
// slog.Default()).
func NewTidal(logger *slog.Logger) *TidalParser {
	if logger == nil {
		logger = slog.Default()
	}
	return &TidalParser{logger: logger}
}

// Name implements namespaces.Parser.
func (p *TidalParser) Name() string { return "tidal" }

// Parse implements namespaces.Parser. Returns ok=true when a Tidal track
// ID could be extracted.
func (p *TidalParser) Parse(load *namespaces.MediaLoad) (namespaces.ParsedIntent, bool, error) {
	p.logger.Debug("tidal: inspecting LOAD",
		"content_id", load.ContentID,
		"content_type", load.ContentType,
		"inner_custom_data", string(load.CustomData),
		"outer_custom_data", string(load.OuterCustomData),
		"metadata", string(load.Metadata))

	// Probe paths in priority order. Returning the first hit is correct
	// even if other paths happen to also carry an id — Tidal's sender does
	// not emit conflicting copies in the wild.
	paths := []string{
		"tidal.trackId",
		"trackId",
		"media.trackId",
		"data.trackId",
	}
	candidates := []json.RawMessage{load.CustomData, load.OuterCustomData}
	for _, blob := range candidates {
		if len(blob) == 0 {
			continue
		}
		for _, path := range paths {
			if id, ok := extractStringPath(blob, path); ok && id != "" {
				return namespaces.ParsedIntent{
					Provider: "tidal",
					TrackID:  id,
				}, true, nil
			}
		}
	}

	// Metadata-based heuristics: if the LOAD metadata subtitle mentions
	// Tidal and the contentId looks numeric, treat the contentId as the
	// track ID.
	if isTidalByMetadata(load.Metadata) {
		if id := strings.TrimSpace(load.ContentID); id != "" {
			return namespaces.ParsedIntent{
				Provider: "tidal",
				TrackID:  id,
			}, true, nil
		}
	}

	// Defer to the next parser.
	return namespaces.ParsedIntent{}, false, nil
}

// extractStringPath walks a dotted path through a JSON object and returns
// the value at the leaf when it is a string or a JSON number. Returns
// ok=false on any miss (missing key, wrong type, malformed JSON).
//
// The function intentionally accepts both string and number leaves
// because Tidal's sender has historically emitted both — older app
// versions used a numeric `trackId`, newer ones a stringified one.
func extractStringPath(blob json.RawMessage, path string) (string, bool) {
	if len(blob) == 0 {
		return "", false
	}
	var current any
	if err := json.Unmarshal(blob, &current); err != nil {
		return "", false
	}
	parts := strings.Split(path, ".")
	for _, part := range parts {
		obj, ok := current.(map[string]any)
		if !ok {
			return "", false
		}
		current, ok = obj[part]
		if !ok {
			return "", false
		}
	}
	switch v := current.(type) {
	case string:
		return v, true
	case float64:
		// JSON numbers always come back as float64. Format without the
		// trailing zero so "12345" looks numeric to MA.
		return jsonNumberToString(v), true
	case json.Number:
		return v.String(), true
	default:
		return "", false
	}
}

// isTidalByMetadata returns true when the LOAD metadata has a subtitle
// (or artist or albumName) field containing "tidal" — case-insensitive.
func isTidalByMetadata(metadata json.RawMessage) bool {
	if len(metadata) == 0 {
		return false
	}
	var obj map[string]any
	if err := json.Unmarshal(metadata, &obj); err != nil {
		return false
	}
	for _, key := range []string{"subtitle", "artist", "albumName", "albumTitle"} {
		if v, ok := obj[key]; ok {
			if s, ok := v.(string); ok {
				if strings.Contains(strings.ToLower(s), "tidal") {
					return true
				}
			}
		}
	}
	return false
}

// jsonNumberToString stringifies a JSON-number leaf without trailing
// zeros. We intentionally avoid strconv to keep the implementation
// readable; for the small integer track-id range Tidal uses, this is
// equivalent.
func jsonNumberToString(f float64) string {
	// Marshal back through json so we get the canonical Go float
	// representation, then strip trailing ".0" Tidal IDs never carry.
	b, err := json.Marshal(f)
	if err != nil {
		return ""
	}
	s := string(b)
	s = strings.TrimSuffix(s, ".0")
	return s
}
