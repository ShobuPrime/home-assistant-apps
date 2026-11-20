package main

// What counts as managed by Home Assistant (root CLAUDE.md, "Telling Home
// Assistant's containers from the user's"). Three marks, and never a list of
// io.hass.type values: that is an image label, and its values get added to.
const (
	// Set by the Supervisor on every container it starts, with an empty value.
	supervisorLabel = "supervisor_managed"
	// An image label. Its value `supervisor` marks the Supervisor's own
	// container, which HAOS starts.
	typeLabel = "io.hass.type"
	// Set to "true" by an app on the containers it starts for itself. The
	// Supervisor does not know those, and stopping one breaks the app.
	appManagedLabel = "io.github.shobuprime.app-managed"
)

// The one label a prune is told to spare. dockerd reads several label!
// values as "has all of them", so a second one would protect less, not more.
const (
	containerPruneLabel = supervisorLabel
	imagePruneLabel     = typeLabel
)

// hiddenContainer reports whether a container with these labels is managed
// by Home Assistant.
func hiddenContainer(labels map[string]string) bool {
	if _, ok := labels[supervisorLabel]; ok {
		return true
	}
	return labels[typeLabel] == "supervisor" || labels[appManagedLabel] == "true"
}

// hiddenImage reports whether an image with these labels is Home Assistant's.
// Images never carry supervisor_managed, so any io.hass.type counts.
func hiddenImage(labels map[string]string) bool {
	_, ok := labels[typeLabel]
	return ok
}
