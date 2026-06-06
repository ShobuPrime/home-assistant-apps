package unifi

import (
	"context"
	"errors"
	"log/slog"
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

	mode        Mode
	connected   bool
	prevOpen    map[string]bool // sensor id -> open, last poll
	prevOpenSet string
	lastState   alarm.State
	lastMirror  alarm.State
	wake        chan struct{}
}

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
		cfg:      cfg,
		log:      log,
		prevOpen: map[string]bool{},
		wake:     make(chan struct{}, 1),
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
	sensors, err := m.client.GetSensors(ctx)
	if err != nil {
		if m.connected {
			m.log.Warn("unifi: sensor poll failed", "err", err)
		}
		m.connected = false
		m.pub.PublishProtectStatus(string(m.effectiveMode()), false)
		return
	}

	var openNames []string
	newlyOpen := false
	cur := map[string]bool{}
	for _, s := range sensors {
		m.pub.AnnounceZone(s.ID, s.Name)
		m.pub.PublishZone(s.ID, s.IsOpen)
		cur[s.ID] = s.IsOpen
		if s.IsOpen {
			openNames = append(openNames, s.Name)
			if !m.prevOpen[s.ID] {
				newlyOpen = true
			}
		}
	}

	// Only push to the engine when the open set actually changed, to avoid
	// republishing the panel state every poll.
	if key := strings.Join(openNames, "|"); key != m.prevOpenSet {
		m.engine.SetOpenSensors(openNames)
		m.prevOpenSet = key
	}

	// App-managed breach: a sensor opening while the system is armed starts
	// the entry-delay countdown (or triggers immediately if entry delay 0).
	if newlyOpen && isArmed(m.lastState) {
		m.log.Info("unifi: breach while armed — triggering entry sequence")
		m.engine.Trigger(false, alarm.Actor{Name: "unifi", Role: "system"})
	}
	m.prevOpen = cur
}

func (m *Manager) onSnapshot(ctx context.Context, snap alarm.Snapshot) {
	// Siren actuation on a fresh trigger (both modes).
	if snap.State == alarm.StateTriggered && m.lastState != alarm.StateTriggered {
		for _, id := range m.cfg.SirenIDs {
			if err := m.client.TriggerSiren(ctx, id, m.cfg.SirenMillis); err != nil {
				m.log.Warn("unifi: siren trigger failed", "siren", id, "err", err)
			}
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
