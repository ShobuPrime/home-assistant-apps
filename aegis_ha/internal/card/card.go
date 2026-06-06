// Package card delivers the companion AegisHA Lovelace card. The card JS is
// embedded in the binary and written to the Home Assistant config www
// directory (mapped via homeassistant_config:rw), where HA serves it at
// the stable /local/ URL. AegisHA logs the exact Lovelace resource line to
// add — this works in both storage and YAML Lovelace modes, with no extra
// dependency. (An add-on cannot reliably self-register a Lovelace resource
// over ingress, so a one-time manual resource add is the honest contract.)
package card

import (
	_ "embed"
	"log/slog"
	"os"
	"path/filepath"
)

//go:embed aegis_ha-card.js
var cardJS []byte

const wwwDir = "/config/www/aegis_ha"

// Deploy writes the card to /config/www/aegis_ha and logs the resource URL.
func Deploy(version string, log *slog.Logger) {
	if err := os.MkdirAll(wwwDir, 0o755); err != nil {
		log.Warn("card: cannot create www directory (is homeassistant_config:rw mapped?)", "err", err)
		return
	}
	path := filepath.Join(wwwDir, "aegis_ha-card.js")
	if err := os.WriteFile(path, cardJS, 0o644); err != nil {
		log.Warn("card: failed to write card file", "err", err)
		return
	}
	url := "/local/aegis_ha/aegis_ha-card.js?v=" + version
	log.Info("card: companion Lovelace card deployed", "served_at", url)
	log.Info("card: add it once as a Lovelace resource — Settings ▸ Dashboards ▸ Resources ▸ Add Resource",
		"url", url, "resource_type", "JavaScript Module")
	log.Info("card: then use it on a dashboard with `type: custom:aegis_ha-card` and `entity: alarm_control_panel.aegis_ha`")
}
