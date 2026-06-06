package mqtt

import "encoding/json"

// discoveryMsg is one retained discovery config publish.
type discoveryMsg struct {
	topic   string
	payload []byte
}

func (b *Bridge) device() map[string]any {
	return map[string]any{
		"identifiers":  []string{b.cfg.Prefix},
		"name":         "AegisHA",
		"model":        "AegisHA Alarm",
		"manufacturer": "ShobuPrime",
		"sw_version":   b.cfg.Version,
	}
}

func (b *Bridge) discoTopic(component, obj string) string {
	return "homeassistant/" + component + "/" + b.cfg.Prefix + "/" + obj + "/config"
}

func (b *Bridge) disco(component, obj string, payload map[string]any) discoveryMsg {
	payload["availability_topic"] = b.statusTopic()
	payload["payload_available"] = "online"
	payload["payload_not_available"] = "offline"
	payload["device"] = b.device()
	buf, _ := json.Marshal(payload)
	return discoveryMsg{topic: b.discoTopic(component, obj), payload: buf}
}

// allDiscovery returns every entity's retained discovery config.
func (b *Bridge) allDiscovery() []discoveryMsg {
	msgs := []discoveryMsg{b.panelDiscovery()}

	msgs = append(msgs,
		b.sensorDiscovery("changed_by", "AegisHA Last Changed By", "mdi:account", "", ""),
		b.sensorDiscovery("open_sensors", "AegisHA Open Sensors", "mdi:door-open", "", ""),
		b.binarySensorDiscovery("lockout_active", "AegisHA Lockout Active", "mdi:lock-alert", "problem"),
		b.buttonDiscovery("panic", "AegisHA Panic", "mdi:alarm-light"),
		b.buttonDiscovery("skip_delay", "AegisHA Skip Delay", "mdi:debug-step-over"),
		b.buttonDiscovery("clear_lockout", "AegisHA Clear Lockout", "mdi:lock-reset"),
		b.numberDiscovery("exit_delay", "AegisHA Exit Delay", "mdi:timer-outline", 0, 600),
		b.numberDiscovery("entry_delay", "AegisHA Entry Delay", "mdi:timer-sand", 0, 600),
		b.numberDiscovery("trigger_time", "AegisHA Trigger Time", "mdi:timer-alert", 0, 3600),
	)
	msgs = append(msgs, b.protectDiscovery()...)
	return msgs
}

func (b *Bridge) panelDiscovery() discoveryMsg {
	code := "REMOTE_CODE"
	if b.cfg.CodeFormat == "text" {
		code = "REMOTE_CODE_TEXT"
	}
	var feats []string
	for _, m := range b.cfg.ArmModes {
		switch m {
		case "away":
			feats = append(feats, "arm_away")
		case "home":
			feats = append(feats, "arm_home")
		case "night":
			feats = append(feats, "arm_night")
		case "vacation":
			feats = append(feats, "arm_vacation")
		case "custom":
			feats = append(feats, "arm_custom_bypass")
		}
	}
	feats = append(feats, "trigger")

	return b.disco("alarm_control_panel", "panel", map[string]any{
		"name":                  "AegisHA",
		"unique_id":             b.cfg.Prefix + "_panel",
		"object_id":             b.cfg.Prefix,
		"state_topic":           b.topic("panel", "state"),
		"command_topic":         b.topic("panel", "cmd"),
		"json_attributes_topic": b.topic("panel", "attrs"),
		"code":                  code,
		"command_template":      `{"action":"{{action}}","code":"{{code}}"}`,
		"code_arm_required":     b.cfg.ArmingRequiresCode,
		"code_disarm_required":  b.cfg.DisarmRequiresCode,
		"code_trigger_required": b.cfg.TriggerRequiresCode,
		"supported_features":    feats,
	})
}

func (b *Bridge) sensorDiscovery(obj, name, icon, unit, deviceClass string) discoveryMsg {
	p := map[string]any{
		"name":        name,
		"unique_id":   b.cfg.Prefix + "_" + obj,
		"object_id":   b.cfg.Prefix + "_" + obj,
		"state_topic": b.topic(obj, "state"),
	}
	if icon != "" {
		p["icon"] = icon
	}
	if unit != "" {
		p["unit_of_measurement"] = unit
	}
	if deviceClass != "" {
		p["device_class"] = deviceClass
	}
	return b.disco("sensor", obj, p)
}

func (b *Bridge) binarySensorDiscovery(obj, name, icon, deviceClass string) discoveryMsg {
	p := map[string]any{
		"name":        name,
		"unique_id":   b.cfg.Prefix + "_" + obj,
		"object_id":   b.cfg.Prefix + "_" + obj,
		"state_topic": b.topic(obj, "state"),
		"payload_on":  "ON",
		"payload_off": "OFF",
		"icon":        icon,
	}
	if deviceClass != "" {
		p["device_class"] = deviceClass
	}
	return b.disco("binary_sensor", obj, p)
}

func (b *Bridge) buttonDiscovery(obj, name, icon string) discoveryMsg {
	return b.disco("button", obj, map[string]any{
		"name":          name,
		"unique_id":     b.cfg.Prefix + "_" + obj,
		"object_id":     b.cfg.Prefix + "_" + obj,
		"command_topic": b.topic(obj, "set"),
		"payload_press": "PRESS",
		"icon":          icon,
	})
}

func (b *Bridge) numberDiscovery(obj, name, icon string, lo, hi int) discoveryMsg {
	return b.disco("number", obj, map[string]any{
		"name":                name,
		"unique_id":           b.cfg.Prefix + "_" + obj,
		"object_id":           b.cfg.Prefix + "_" + obj,
		"command_topic":       b.topic(obj, "set"),
		"state_topic":         b.topic(obj, "state"),
		"min":                 lo,
		"max":                 hi,
		"step":                1,
		"mode":                "box",
		"unit_of_measurement": "s",
		"icon":                icon,
	})
}
