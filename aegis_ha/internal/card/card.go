// Package card delivers the companion AegisHA Lovelace card. The card JS is
// embedded in the binary and written to the Home Assistant config www
// directory (mapped via homeassistant_config:rw), where HA serves it at the
// stable /local/ URL. The caller then auto-registers it as a Lovelace
// resource over the Supervisor Core-WebSocket (storage mode) — see
// ha.RegisterLovelaceResource — falling back to a logged manual snippet on
// YAML-mode dashboards.
package card

import (
	_ "embed"
	"log/slog"
	"os"
	"path/filepath"
)

//go:embed aegis_ha-card.js
var cardJS []byte

// wwwDir is Home Assistant's config www directory as seen from inside the
// add-on. The homeassistant_config:rw map mounts HA's config dir at
// /homeassistant (NOT /config — that's the add-on's own throwaway space), so
// HA's /config/www — what the /local/ URL serves — is /homeassistant/www here.
const wwwDir = "/homeassistant/www/aegis_ha"

// ResourceURL is the stable /local URL the card is served at (with a
// version cache-buster).
func ResourceURL(version string) string {
	return "/local/aegis_ha/aegis_ha-card.js?v=" + version
}

// Deploy writes the card to /config/www/aegis_ha and returns the Lovelace
// resource URL to register (empty string if the write failed).
func Deploy(version string, log *slog.Logger) string {
	if err := os.MkdirAll(wwwDir, 0o755); err != nil {
		log.Warn("card: cannot create www directory (is homeassistant_config:rw mapped?)", "err", err)
		return ""
	}
	path := filepath.Join(wwwDir, "aegis_ha-card.js")
	if err := os.WriteFile(path, cardJS, 0o644); err != nil {
		log.Warn("card: failed to write card file", "path", path, "err", err)
		return ""
	}
	// Confirm the file is actually present at the HA www path before claiming
	// success — a wrong/absent mapping would otherwise log "deployed" for a URL
	// that 404s. Only a verified file gets its resource registered.
	if _, err := os.Stat(path); err != nil {
		log.Warn("card: card file not found after write — skipping registration", "path", path, "err", err)
		return ""
	}
	url := ResourceURL(version)
	log.Info("card: companion Lovelace card deployed", "path", path, "served_at", url)
	return url
}
