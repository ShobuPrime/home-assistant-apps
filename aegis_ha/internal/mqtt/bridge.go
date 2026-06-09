package mqtt

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shobuprime/aegis_ha/internal/alarm"
	"github.com/shobuprime/aegis_ha/internal/store"
)

// Config configures the discovery/bridge layer.
type Config struct {
	Prefix              string
	CodeFormat          string // number | text
	ArmModes            []string
	ArmingRequiresCode  bool
	DisarmRequiresCode  bool
	TriggerRequiresCode bool
	Version             string
}

// Bridge connects the MQTT client, the alarm engine, and the PIN store:
// it publishes Home Assistant discovery + retained state, and authorizes
// inbound keypad/entity commands before driving the engine.
type Bridge struct {
	client *Client
	engine *alarm.Engine
	store  *store.Store
	cfg    Config
	log    *slog.Logger

	cfgMu    sync.Mutex
	alarmCfg alarm.Config

	protectMu      sync.Mutex
	protectEnabled bool
	zones          []zoneInfo
	bypassByObj    map[string]string // bypass switch object_id -> sensor id

	eventSink func(eventType string, data map[string]any)
	lastPub   alarm.State
}

// SetEventSink registers a callback used to fire HA bus events. The
// callback should be non-blocking (AegisHA wraps it in a goroutine).
func (b *Bridge) SetEventSink(fn func(string, map[string]any)) {
	b.eventSink = fn
}

func (b *Bridge) fire(event string, data map[string]any) {
	if b.eventSink != nil {
		b.eventSink(b.cfg.Prefix+"_"+event, data)
	}
}

type zoneInfo struct {
	id, name string
}

// NewBridge wires the bridge onto the client (setting its OnConnect hook
// and registering command subscriptions) and returns it. Start it with
// Run, and start the client with client.Run.
func NewBridge(client *Client, engine *alarm.Engine, st *store.Store, cfg Config, alarmCfg alarm.Config, log *slog.Logger) *Bridge {
	if log == nil {
		log = slog.Default()
	}
	if cfg.Prefix == "" {
		cfg.Prefix = "aegis_ha"
	}
	b := &Bridge{client: client, engine: engine, store: st, cfg: cfg, log: log, alarmCfg: alarmCfg, bypassByObj: map[string]string{}}
	client.opts.OnConnect = b.announce
	if client.opts.Will == nil {
		client.opts.Will = &Message{Topic: b.statusTopic(), Payload: []byte("offline"), Retain: true}
	}
	b.registerSubscriptions()
	return b
}

func (b *Bridge) registerSubscriptions() {
	_ = b.client.Subscribe(b.topic("panel", "cmd"), b.handlePanelCmd)
	_ = b.client.Subscribe(b.cfg.Prefix+"/+/set", b.handleSet)
	_ = b.client.Subscribe("homeassistant/status", b.handleHAStatus)
}

// Run publishes retained state whenever the alarm engine changes.
func (b *Bridge) Run(ctx context.Context) {
	ch := b.engine.Subscribe()
	for {
		select {
		case <-ctx.Done():
			return
		case snap := <-ch:
			b.publishState(snap)
		}
	}
}

// announce is the OnConnect hook: mark online, publish discovery, then
// publish current state. Called on every (re)connect.
func (b *Bridge) announce(c *Client) {
	_ = c.Publish(b.statusTopic(), []byte("online"), true)
	for _, m := range b.allDiscovery() {
		_ = c.Publish(m.topic, m.payload, true)
	}
	b.publishState(b.engine.Current())
	b.publishNumbers()
}

func (b *Bridge) handleHAStatus(m Message) {
	if strings.EqualFold(strings.TrimSpace(string(m.Payload)), "online") {
		b.log.Info("mqtt: home assistant came online — re-announcing discovery")
		b.announce(b.client)
	}
}

// --- state publishing ---

func (b *Bridge) publishState(snap alarm.Snapshot) {
	if snap.State == alarm.StateTriggered && b.lastPub != alarm.StateTriggered {
		b.fire("triggered", map[string]any{"arm_mode": snap.ArmMode, "open_sensors": snap.OpenSensors})
	}
	if strings.HasPrefix(string(snap.State), "armed_") && b.lastPub != snap.State {
		b.fire("command_success", map[string]any{"action": "armed", "arm_mode": snap.ArmMode})
	}
	b.lastPub = snap.State

	_ = b.client.Publish(b.topic("panel", "state"), []byte(snap.State), true)

	// Attributes deliberately avoid HA's reserved alarm attribute names
	// (changed_by, code_format, code_arm_required, supported_features);
	// changed_by is exposed as a separate sensor instead.
	attrs := map[string]any{
		"arm_mode":          snap.ArmMode,
		"prior_arm_mode":    snap.PriorArmMode,
		"open_sensors":      snap.OpenSensors,
		"open_sensor_count": len(snap.OpenSensors),
		"bypassed_sensors":  snap.BypassedSensors,
		"ready_to_arm":      snap.ReadyToArm,
		"delay_total":       snap.DelayTotal,
		"armed_by":          snap.ChangedBy,
	}
	if snap.DelayEndsUnix > 0 {
		attrs["delay_ends"] = snap.DelayEndsUnix
	}
	if buf, err := json.Marshal(attrs); err == nil {
		_ = b.client.Publish(b.topic("panel", "attrs"), buf, true)
	}

	changedBy := snap.ChangedBy
	if changedBy == "" {
		changedBy = "unknown"
	}
	_ = b.client.Publish(b.topic("changed_by", "state"), []byte(changedBy), true)
	_ = b.client.Publish(b.topic("open_sensors", "state"), []byte(strconv.Itoa(len(snap.OpenSensors))), true)
	b.publishLockout()
}

func (b *Bridge) publishLockout() {
	state := "OFF"
	if b.store != nil && b.store.LockoutActive(time.Now()) {
		state = "ON"
	}
	_ = b.client.Publish(b.topic("lockout_active", "state"), []byte(state), true)
}

func (b *Bridge) publishNumbers() {
	b.cfgMu.Lock()
	cfg := b.alarmCfg
	b.cfgMu.Unlock()
	_ = b.client.Publish(b.topic("exit_delay", "state"), []byte(strconv.Itoa(int(cfg.ExitDelay/time.Second))), true)
	_ = b.client.Publish(b.topic("entry_delay", "state"), []byte(strconv.Itoa(int(cfg.EntryDelay/time.Second))), true)
	_ = b.client.Publish(b.topic("trigger_time", "state"), []byte(strconv.Itoa(int(cfg.TriggerTime/time.Second))), true)
}

// --- inbound command handling ---

type panelCommand struct {
	Action string `json:"action"`
	Code   string `json:"code"`
}

func (b *Bridge) handlePanelCmd(m Message) {
	var pc panelCommand
	if err := json.Unmarshal(m.Payload, &pc); err != nil {
		// Tolerate a bare action string (e.g. a manual publish).
		pc.Action = strings.TrimSpace(string(m.Payload))
	}
	action, mode, ok := mapAction(pc.Action)
	if !ok {
		b.log.Warn("mqtt: unknown panel action", "action", pc.Action)
		return
	}

	perm := store.Perm{Action: action, Mode: mode, CodeRequired: b.codeRequired(action)}
	dec := b.store.AuthorizeMQTT(pc.Code, perm, time.Now())

	switch {
	case dec.Reason == "locked":
		b.log.Warn("mqtt: command rejected — lockout active", "action", action)
		b.fire("command_not_allowed", map[string]any{"action": action, "reason": "locked"})
		b.publishLockout()
		return
	case dec.Duress:
		actor := actorFor(dec)
		b.log.Warn("aegis_ha: DURESS code used — silent disarm", "user", actor.Name)
		b.fire("duress", map[string]any{"user": actor.Name})
		b.engine.Disarm(actor)
		return
	case !dec.Allowed:
		b.log.Warn("mqtt: command not authorized", "action", action, "reason", dec.Reason)
		b.fire("command_not_allowed", map[string]any{"action": action, "reason": dec.Reason})
		b.publishLockout()
		return
	}

	actor := actorFor(dec)
	switch action {
	case "arm":
		if r := b.engine.Arm(mode, actor, false); !r.Accepted {
			b.log.Warn("aegis_ha: arm rejected", "mode", mode, "reason", r.Reason)
			b.fire("failed_to_arm", map[string]any{"mode": mode, "reason": r.Reason, "open_sensors": r.Snapshot.OpenSensors})
		} else {
			b.fire("command_success", map[string]any{"action": "arm", "mode": mode, "user": actor.Name})
		}
	case "disarm":
		b.engine.Disarm(actor)
		b.fire("command_success", map[string]any{"action": "disarm", "user": actor.Name})
	case "trigger":
		b.engine.Trigger(true, actor)
		b.fire("command_success", map[string]any{"action": "trigger", "user": actor.Name})
	}
}

func (b *Bridge) handleSet(m Message) {
	parts := strings.Split(m.Topic, "/")
	if len(parts) != 3 {
		return
	}
	obj := parts[1]
	val := strings.TrimSpace(string(m.Payload))
	sys := alarm.Actor{Name: "automation", Role: "system"}

	// Per-zone bypass switches: obj == "bypass_zone_<sanitized id>".
	if strings.HasPrefix(obj, "bypass_") {
		b.protectMu.Lock()
		id := b.bypassByObj[obj]
		b.protectMu.Unlock()
		if id != "" {
			on := strings.EqualFold(val, "ON")
			b.engine.SetBypass(id, on)
			_ = b.client.Publish(b.topic(obj, "state"), []byte(boolOnOff(on)), true)
		}
		return
	}

	switch obj {
	case "panic":
		b.engine.Trigger(true, sys)
	case "skip_delay":
		b.engine.SkipDelay(sys)
	case "clear_lockout":
		_ = b.store.ClearLockout()
		b.publishLockout()
	case "exit_delay", "entry_delay", "trigger_time":
		secs, err := strconv.Atoi(val)
		if err != nil || secs < 0 {
			return
		}
		b.cfgMu.Lock()
		switch obj {
		case "exit_delay":
			b.alarmCfg.ExitDelay = time.Duration(secs) * time.Second
		case "entry_delay":
			b.alarmCfg.EntryDelay = time.Duration(secs) * time.Second
		case "trigger_time":
			b.alarmCfg.TriggerTime = time.Duration(secs) * time.Second
		}
		cfg := b.alarmCfg
		b.cfgMu.Unlock()
		b.engine.SetConfig(cfg)
		b.publishNumbers()
	}
}

func (b *Bridge) codeRequired(action string) bool {
	switch action {
	case "arm":
		return b.cfg.ArmingRequiresCode
	case "disarm":
		return b.cfg.DisarmRequiresCode
	case "trigger":
		return b.cfg.TriggerRequiresCode
	}
	return false
}

func actorFor(dec store.Decision) alarm.Actor {
	if dec.User != nil {
		return alarm.Actor{Name: dec.User.Name, UserID: dec.User.HAUserID, Role: dec.User.Role}
	}
	return alarm.Actor{Name: "keypad"}
}

// mapAction maps an HA alarm command payload to (action, mode).
func mapAction(a string) (action, mode string, ok bool) {
	switch strings.ToUpper(strings.TrimSpace(a)) {
	case "ARM_AWAY":
		return "arm", "away", true
	case "ARM_HOME":
		return "arm", "home", true
	case "ARM_NIGHT":
		return "arm", "night", true
	case "ARM_VACATION":
		return "arm", "vacation", true
	case "ARM_CUSTOM_BYPASS":
		return "arm", "custom", true
	case "DISARM":
		return "disarm", "", true
	case "TRIGGER":
		return "trigger", "", true
	}
	return "", "", false
}

// --- topic helpers ---

func (b *Bridge) topic(obj, suffix string) string {
	return b.cfg.Prefix + "/" + obj + "/" + suffix
}

func (b *Bridge) statusTopic() string {
	return b.cfg.Prefix + "/status"
}

func boolOnOff(b bool) string {
	if b {
		return "ON"
	}
	return "OFF"
}
