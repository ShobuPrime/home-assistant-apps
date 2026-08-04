package card

import (
	"strings"
	"testing"
)

func TestCardEmbeddedWithContract(t *testing.T) {
	if len(cardJS) == 0 {
		t.Fatal("aegis_ha-card.js was not embedded")
	}
	s := string(cardJS)
	for _, want := range []string{
		`customElements.define("aegis_ha-card"`,
		"alarm_control_panel",
		"supported_features",
		"window.customCards",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("card missing expected contract: %q", want)
		}
	}
	// Rough balance sanity (no JS engine available locally).
	if strings.Count(s, "{") != strings.Count(s, "}") {
		t.Errorf("unbalanced braces: %d open, %d close", strings.Count(s, "{"), strings.Count(s, "}"))
	}
}
