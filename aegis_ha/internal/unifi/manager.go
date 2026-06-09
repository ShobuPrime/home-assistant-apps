package unifi

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/websocket"

	"github.com/shobuprime/aegis_ha/internal/alarm"
)

// Publisher is what the manager needs to expose Protect state as HA
// entities. It is satisfied structurally by the MQTT bridge (so the unifi
// package does not import mqtt, avoiding an import cycle).
type Publisher interface {
	EnableProtect()
	AnnounceZone(id, name string)
	PublishProtectStatus(mode string, connected bool)
	PublishZone(id string, open bool)
}

// Config tunes the manager.
type Config struct {
	PreferMode   string // auto | local | app-managed
	PollInterval time.Duration
	Profiles     map[string]string // arm mode -> UniFi armProfileId (local mirror)
	SirenIDs     []string
	SirenMillis  int

	// ArmModes is the set of arm modes AegisHA offers; the first (preferring
	// "away") is used as the target when mirroring an externally-initiated
	// Protect arm into the engine.
	ArmModes []string

	// SensorOverrides maps a lowercased sensor name to its Alarmo-style
	// per-sensor configuration (modes, always_on, immediate, group, …);
	// Groups defines the sensor-group debounce rules. Sensors without an
	// override use permissive defaults.
	SensorOverrides map[string]alarm.SensorConfig
	Groups          []alarm.SensorGroup

	// ExposeZones controls whether per-sensor binary_sensor + bypass switch
	// entities are published to HA. Off by default to avoid duplicating the
	// official UniFi Protect integration's sensors; the engine still uses
	// the sensor states internally for readiness + breach regardless.
	ExposeZones bool

	// Protect Alarm Manager webhook trigger IDs, fired on the corresponding
	// AegisHA transition. These work in Global mode (where arm profiles are
	// blocked), letting Protect react — e.g. sound a siren on a breach.
	WebhookArm     string
	WebhookDisarm  string
	WebhookTrigger string

	// WebhookArmAtStart fires the ARM webhook when arming begins (delay owned
	// by the Protect alarm) instead of when AegisHA finishes arming (delay
	// owned by the app). Set from the exit_delay_source option.
	WebhookArmAtStart bool
}

// Manager reconciles the alarm engine with a UniFi Protect gateway: it
// detects the Alarm Manager mode, mirrors arm/disarm in local mode,
// polls sensors for readiness + breach detection, and actuates sirens on
// a trigger.
type Manager struct {
	client *Client
	engine *alarm.Engine
	pub    Publisher
	cfg    Config
	log    *slog.Logger

	mode           Mode
	connected      bool
	prevState      map[string]alarm.SensorEventKind // last event sent per sensor id
	lastSensorSet  string                           // detects when the sensor set/config changes
	lastState      alarm.State
	lastMirror     alarm.State
	lastProtectArm string // last observed Protect armMode.status (for read-sync)
	wake           chan struct{}
}

// mirrorActor is the changed_by attribution used when AegisHA mirrors an
// externally-initiated Protect arm/disarm. onSnapshot recognizes it and
// skips firing the arm/disarm webhook back at Protect, so a mirror can
// never echo into a loop.
const mirrorActor = "UniFi Protect"

// NewManager builds a Manager.
func NewManager(client *Client, engine *alarm.Engine, pub Publisher, cfg Config, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 10 * time.Second
	}
	if cfg.SirenMillis <= 0 {
		cfg.SirenMillis = 30000
	}
	return &Manager{
		client:   client,
		engine:   engine,
		pub:      pub,
		cfg:       cfg,
		log:       log,
		prevState: map[string]alarm.SensorEventKind{},
		wake:      make(chan struct{}, 1),
	}
}

// Run blocks until ctx is cancelled.
func (m *Manager) Run(ctx context.Context) {
	m.pub.EnableProtect()
	m.detect(ctx)
	m.poll(ctx)

	// Low-latency change signal over the Protect event WebSocket; the
	// periodic poll below remains the authoritative fallback.
	go m.runEvents(ctx)

	snaps := m.engine.Subscribe()
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case snap := <-snaps:
			m.onSnapshot(ctx, snap)
		case <-m.wake:
			m.poll(ctx)
		case <-ticker.C:
			m.detect(ctx)
			m.poll(ctx)
		}
	}
}

// runEvents maintains the Protect device-event WebSocket and pokes the
// poll loop on every event. Any event triggers a re-poll of the REST
// sensors (the source of truth), so individual event payloads are never
// parsed — robust across firmware revisions and unverified wire formats.
func (m *Manager) runEvents(ctx context.Context) {
	backoff := 2 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		conn, err := m.client.DialEvents()
		if err != nil {
			m.log.Debug("unifi: event stream connect failed (polling continues)", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 60*time.Second {
				backoff *= 2
			}
			continue
		}
		m.log.Info("unifi: event stream connected")
		backoff = 2 * time.Second
		go func() { <-ctx.Done(); _ = conn.Close() }()
		var msg string
		for {
			if err := websocket.Message.Receive(conn, &msg); err != nil {
				break
			}
			select {
			case m.wake <- struct{}{}:
			default:
			}
		}
		_ = conn.Close()
		if ctx.Err() != nil {
			return
		}
		m.log.Debug("unifi: event stream dropped — reconnecting")
	}
}

// effectiveMode resolves the operating mode from the detected capability
// and the user's preference.
func (m *Manager) effectiveMode() Mode {
	switch m.cfg.PreferMode {
	case "local":
		return ModeLocal
	case "app-managed":
		return ModeAppManaged
	default:
		return m.mode
	}
}

func (m *Manager) detect(ctx context.Context) {
	mode, err := m.client.DetectMode(ctx)
	connected := err == nil && mode != ModeUnavailable
	if err != nil {
		m.log.Warn("unifi: capability detection failed", "err", err)
	}
	if mode != m.mode {
		m.log.Info("unifi: alarm manager mode", "mode", mode)
	}
	m.mode, m.connected = mode, connected
	m.pub.PublishProtectStatus(string(m.effectiveMode()), connected)
}

func (m *Manager) poll(ctx context.Context) {
	m.syncArmState(ctx)

	sensors, err := m.client.GetSensors(ctx)
	if err != nil {
		if m.connected {
			m.log.Warn("unifi: sensor poll failed", "err", err)
		}
		m.connected = false
		m.pub.PublishProtectStatus(string(m.effectiveMode()), false)
		return
	}

	// Build the engine sensor configuration (UniFi identity + any per-sensor
	// override) and re-publish each zone entity.
	cfgs := make([]alarm.SensorConfig, 0, len(sensors))
	var setKey strings.Builder
	for _, s := range sensors {
		// Only mirror per-zone entities into HA when explicitly enabled —
		// otherwise rely on the official UniFi Protect integration and avoid
		// duplicate sensor entities. The engine still gets the config+events.
		if m.cfg.ExposeZones {
			m.pub.AnnounceZone(s.ID, s.Name)
			m.pub.PublishZone(s.ID, s.Tripped())
		}
		sc := alarm.SensorConfig{ID: s.ID, Name: s.Name, Type: s.MountType}
		if ov, ok := m.cfg.SensorOverrides[strings.ToLower(s.Name)]; ok {
			sc.Modes = ov.Modes
			sc.AlwaysOn = ov.AlwaysOn
			sc.Immediate = ov.Immediate
			sc.UseExitDelay = ov.UseExitDelay
			sc.AutoBypass = ov.AutoBypass
			sc.AllowOpen = ov.AllowOpen
			sc.TriggerUnavailable = ov.TriggerUnavailable
			sc.Group = ov.Group
		}
		cfgs = append(cfgs, sc)
		setKey.WriteString(s.ID)
		setKey.WriteByte('|')
		setKey.WriteString(s.Name)
		setKey.WriteByte(';')
	}

	// Re-configure the engine only when the sensor set/config changes.
	if k := setKey.String(); k != m.lastSensorSet {
		m.engine.ConfigureSensors(cfgs, m.cfg.Groups)
		m.lastSensorSet = k
	}

	// Feed transitions. The engine owns the breach decision (entry delay,
	// immediate, always-on, groups, bypass) — the manager just reports state.
	for _, s := range sensors {
		kind := alarm.SensorClosed
		if s.Tripped() {
			kind = alarm.SensorOpen
		}
		if m.prevState[s.ID] != kind {
			m.engine.SensorEvent(s.ID, kind)
			m.prevState[s.ID] = kind
		}
	}
}

func (m *Manager) onSnapshot(ctx context.Context, snap alarm.Snapshot) {
	// Protect Alarm Manager webhook actuation on AegisHA transitions. These
	// fire regardless of Global/Local mode (the webhook endpoint is not
	// blocked in Global), so they're the way to drive Protect's native alarm
	// (siren/lights/notifications) from AegisHA while staying in Global.
	//
	// Suppress firing when the transition was itself a mirror of a Protect
	// arm/disarm (changed_by == mirrorActor): Protect is already in that
	// state, so echoing the webhook back would be redundant and risks a loop.
	if snap.ChangedBy != mirrorActor {
		switch {
		case snap.State == alarm.StateTriggered && m.lastState != alarm.StateTriggered:
			m.fireWebhook(ctx, m.cfg.WebhookTrigger)
			for _, id := range m.cfg.SirenIDs {
				if err := m.client.TriggerSiren(ctx, id, m.cfg.SirenMillis); err != nil {
					m.log.Warn("unifi: siren trigger failed", "siren", id, "err", err)
				}
			}
		case m.lastState != "" && armWebhookFires(m.lastState, snap.State, m.cfg.WebhookArmAtStart):
			m.fireWebhook(ctx, m.cfg.WebhookArm)
		case snap.State == alarm.StateDisarmed && m.lastState != alarm.StateDisarmed && m.lastState != "":
			m.fireWebhook(ctx, m.cfg.WebhookDisarm)
		}
	}

	// Local-mirror arm/disarm.
	if m.effectiveMode() == ModeLocal {
		switch {
		case isArmed(snap.State) && snap.State != m.lastMirror:
			if err := m.client.Arm(ctx, m.cfg.Profiles[snap.ArmMode]); err != nil {
				if errors.Is(err, ErrGlobalMode) {
					m.log.Warn("unifi: arm refused — alarm manager is in global mode; switching to app-managed")
					m.mode = ModeGlobal
					m.pub.PublishProtectStatus(string(m.effectiveMode()), m.connected)
				} else {
					m.log.Warn("unifi: mirror arm failed", "err", err)
				}
			}
			m.lastMirror = snap.State
		case snap.State == alarm.StateDisarmed && m.lastMirror != alarm.StateDisarmed:
			if err := m.client.Disarm(ctx); err != nil && !errors.Is(err, ErrGlobalMode) {
				m.log.Warn("unifi: mirror disarm failed", "err", err)
			}
			m.lastMirror = alarm.StateDisarmed
		}
	}

	m.lastState = snap.State
}

func isArmed(s alarm.State) bool {
	return strings.HasPrefix(string(s), "armed_")
}

// armWebhookFires reports whether a *fresh* arm cycle (starting from
// disarmed) warrants firing the ARM webhook on this transition. atStart
// fires the moment arming begins (Protect owns the delay); otherwise it
// fires when AegisHA reaches a committed armed_* state (app owns the
// delay). A trigger resolving back to an armed state does not re-fire.
func armWebhookFires(prev, cur alarm.State, atStart bool) bool {
	if atStart {
		return prev == alarm.StateDisarmed && (cur == alarm.StateArming || isArmed(cur))
	}
	return isArmed(cur) && (prev == alarm.StateArming || prev == alarm.StateDisarmed)
}

// syncArmState mirrors an externally-initiated Protect arm/disarm into the
// engine, so the AegisHA panel reflects reality when you arm or disarm from
// the UniFi Protect app (the bidirectional read-sync). It reads the NVR's
// armMode.status — which is readable even in Global mode, where AegisHA
// cannot write arm profiles — and acts only on a *change* in that status,
// and only when the engine's own state disagrees. App-managed mode opts out
// (AegisHA is the sole source of truth there). Mirror commands are tagged
// with mirrorActor so onSnapshot won't echo a webhook back to Protect.
func (m *Manager) syncArmState(ctx context.Context) {
	if m.effectiveMode() == ModeAppManaged {
		return
	}
	nvrs, err := m.client.GetNVRs(ctx)
	if err != nil || len(nvrs) == 0 || nvrs[0].ArmMode == nil {
		return
	}
	status := strings.ToLower(strings.TrimSpace(nvrs[0].ArmMode.Status))
	if status == "" || status == m.lastProtectArm {
		return // no externally-observed change
	}
	prev := m.lastProtectArm
	m.lastProtectArm = status
	if prev == "" {
		return // first observation: adopt as baseline, never act on startup
	}

	cur := m.engine.Current().State
	actor := alarm.Actor{Name: mirrorActor, Role: "system"}
	if protectArmed(status) {
		// Protect armed externally — mirror only if AegisHA is fully disarmed
		// (don't override an in-progress arming/pending/triggered cycle).
		if cur == alarm.StateDisarmed {
			mode := m.mirrorArmMode()
			m.log.Info("unifi: mirroring external Protect arm into AegisHA", "protect_status", status, "mode", mode)
			if r := m.engine.Arm(mode, actor, true); r.Accepted {
				m.engine.SkipDelay(actor) // reflect "armed now"; the external arm already happened
			}
		}
	} else {
		// Protect disarmed externally — mirror only if AegisHA is armed or
		// counting down (leave a real triggered alarm alone).
		if isArmed(cur) || cur == alarm.StateArming || cur == alarm.StatePending {
			m.log.Info("unifi: mirroring external Protect disarm into AegisHA", "protect_status", status)
			m.engine.Disarm(actor)
		}
	}
}

// protectArmed reports whether a Protect armMode.status string represents an
// armed (or arming/breach) state, as opposed to disarmed.
func protectArmed(status string) bool {
	switch status {
	case "", "disabled", "disarmed", "off", "idle":
		return false
	}
	return true
}

// mirrorArmMode picks the engine arm mode to use when mirroring an external
// Protect arm: "away" when available, else the first configured mode, else
// "away" as a last resort.
func (m *Manager) mirrorArmMode() string {
	if slices.Contains(m.cfg.ArmModes, "away") {
		return "away"
	}
	if len(m.cfg.ArmModes) > 0 {
		return m.cfg.ArmModes[0]
	}
	return "away"
}

// fireWebhook POSTs a Protect Alarm Manager webhook trigger (async, so it
// never blocks snapshot handling). A no-op when id is empty.
func (m *Manager) fireWebhook(ctx context.Context, id string) {
	if id == "" {
		return
	}
	go func() {
		if err := m.client.FireWebhook(ctx, id); err != nil {
			m.log.Warn("unifi: webhook fire failed (is this trigger ID configured in the Protect Alarm Manager?)",
				"id", id, "err", err)
			return
		}
		m.log.Info("unifi: fired Protect Alarm Manager webhook", "id", id)
	}()
}
