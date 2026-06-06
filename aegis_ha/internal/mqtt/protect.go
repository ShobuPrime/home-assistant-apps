package mqtt

import "strings"

// This file implements the unifi.Publisher contract (defined structurally
// in the unifi package): the UniFi manager calls these to expose the
// Protect link mode, connectivity, and per-zone sensors as native HA
// entities under the same AegisHA device.

// EnableProtect turns on the Protect entity set (link_mode + connected)
// and (re)announces it on the next connect. Safe to call once at startup.
func (b *Bridge) EnableProtect() {
	b.protectMu.Lock()
	b.protectEnabled = true
	b.protectMu.Unlock()
	for _, m := range b.protectDiscovery() {
		_ = b.client.Publish(m.topic, m.payload, true)
	}
}

// AnnounceZone registers a per-Protect-sensor binary_sensor and publishes
// its discovery config. Idempotent for a given id.
func (b *Bridge) AnnounceZone(id, name string) {
	b.protectMu.Lock()
	for _, z := range b.zones {
		if z.id == id {
			b.protectMu.Unlock()
			return
		}
	}
	b.zones = append(b.zones, zoneInfo{id: id, name: name})
	b.bypassByObj["bypass_"+zoneObj(id)] = id
	b.protectMu.Unlock()

	zm := b.zoneDiscovery(id, name)
	_ = b.client.Publish(zm.topic, zm.payload, true)
	bm := b.bypassDiscovery(id, name)
	_ = b.client.Publish(bm.topic, bm.payload, true)
	_ = b.client.Publish(b.topic("bypass_"+zoneObj(id), "state"), []byte("OFF"), true)
}

// PublishProtectStatus publishes the detected link mode and connectivity.
func (b *Bridge) PublishProtectStatus(mode string, connected bool) {
	_ = b.client.Publish(b.topic("protect_link_mode", "state"), []byte(mode), true)
	state := "OFF"
	if connected {
		state = "ON"
	}
	_ = b.client.Publish(b.topic("protect_connected", "state"), []byte(state), true)
}

// PublishZone publishes a zone's open/closed state.
func (b *Bridge) PublishZone(id string, open bool) {
	state := "OFF"
	if open {
		state = "ON"
	}
	_ = b.client.Publish(b.topic(zoneObj(id), "state"), []byte(state), true)
}

// protectDiscovery returns the Protect discovery configs (empty when
// Protect is not enabled). Included in allDiscovery so reconnects and HA
// restarts re-announce everything.
func (b *Bridge) protectDiscovery() []discoveryMsg {
	b.protectMu.Lock()
	enabled := b.protectEnabled
	zones := append([]zoneInfo(nil), b.zones...)
	b.protectMu.Unlock()
	if !enabled {
		return nil
	}
	msgs := []discoveryMsg{
		b.sensorDiscovery("protect_link_mode", "AegisHA Protect Link Mode", "mdi:shield-link-variant", "", ""),
		b.binarySensorDiscovery("protect_connected", "AegisHA Protect Connected", "mdi:lan-connect", "connectivity"),
	}
	for _, z := range zones {
		msgs = append(msgs, b.zoneDiscovery(z.id, z.name), b.bypassDiscovery(z.id, z.name))
	}
	return msgs
}

// bypassDiscovery is the per-zone manual-bypass switch (a bidirectional
// entity: HA toggles it, the engine excludes the sensor from triggering).
func (b *Bridge) bypassDiscovery(id, name string) discoveryMsg {
	obj := "bypass_" + zoneObj(id)
	return b.disco("switch", obj, map[string]any{
		"name":          name + " Bypass",
		"unique_id":     b.cfg.Prefix + "_" + obj,
		"object_id":     b.cfg.Prefix + "_" + obj,
		"command_topic": b.topic(obj, "set"),
		"state_topic":   b.topic(obj, "state"),
		"payload_on":    "ON",
		"payload_off":   "OFF",
		"icon":          "mdi:shield-off-outline",
	})
}

func (b *Bridge) zoneDiscovery(id, name string) discoveryMsg {
	obj := zoneObj(id)
	return b.disco("binary_sensor", obj, map[string]any{
		"name":         name,
		"unique_id":    b.cfg.Prefix + "_" + obj,
		"object_id":    b.cfg.Prefix + "_" + obj,
		"state_topic":  b.topic(obj, "state"),
		"payload_on":   "ON",
		"payload_off":  "OFF",
		"device_class": "opening",
		"icon":         "mdi:door",
	})
}

func zoneObj(id string) string {
	return "zone_" + sanitize(id)
}

func sanitize(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	return sb.String()
}
