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

	// Entity names are the role only; HA prepends the device name "AegisHA".
	msgs = append(msgs,
		b.sensorDiscovery("changed_by", "Last Changed By", "mdi:account", "", ""),
		b.sensorDiscovery("open_sensors", "Open Sensors", "mdi:door-open", "", ""),
		b.binarySensorDiscovery("lockout_active", "Lockout Active", "mdi:lock-alert", "problem"),
		b.buttonDiscovery("panic", "Panic", "mdi:alarm-light"),
		b.buttonDiscovery("skip_delay", "Skip Delay", "mdi:debug-step-over"),
		b.buttonDiscovery("clear_lockout", "Clear Lockout", "mdi:lock-reset"),
		b.numberDiscovery("exit_delay", "Exit Delay", "mdi:timer-outline", 0, 600),
		b.numberDiscovery("entry_delay", "Entry Delay", "mdi:timer-sand", 0, 600),
		b.numberDiscovery("trigger_time", "Trigger Time", "mdi:timer-alert", 0, 3600),
	)
	msgs = append(msgs, b.protectDiscovery()...)
	return msgs
}

func (b *Bridge) panelDiscovery() discoveryMsg {
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

	payload := map[string]any{
		// HA composes the friendly name as "<device name> <entity name>", so the
		// entity name is just the role ("Alarm Manager") → "AegisHA Alarm Manager".
		"name":                  "Alarm Manager",
		"unique_id":             b.cfg.Prefix + "_panel",
		"object_id":             b.cfg.Prefix,
		"state_topic":           b.topic("panel", "state"),
		"command_topic":         b.topic("panel", "cmd"),
		"json_attributes_topic": b.topic("panel", "attrs"),
		"supported_features":    feats,
	}
	// Only advertise a PIN field when a shared code is actually configured.
	// REMOTE_CODE forwards the entered PIN to AegisHA (which holds the code);
	// the keypad is numeric. With no code set, omitting `code` means Home
	// Assistant arms/disarms with no prompt, and a bare action payload
	// (e.g. "DISARM") is still handled by handlePanelCmd.
	if b.cfg.CodeConfigured {
		payload["code"] = "REMOTE_CODE"
		payload["command_template"] = `{"action":"{{action}}","code":"{{code}}"}`
		payload["code_arm_required"] = b.cfg.RequireCodeToArm
		payload["code_disarm_required"] = b.cfg.RequireCodeToDisarm
	}
	return b.disco("alarm_control_panel", "panel", payload)
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
