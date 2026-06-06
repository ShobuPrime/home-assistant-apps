package alarm

import (
	"slices"
	"time"
)

// SensorConfig is the per-sensor override model, mirroring Alarmo's sensor
// options. Zero values are the permissive defaults (active in every arm
// mode, entry+exit delays apply, blocks arming when open).
type SensorConfig struct {
	ID    string
	Name  string
	Type  string   // door | window | motion | smoke | water | tamper | ...
	Modes []string // arm modes this sensor is active in; empty == all modes

	AlwaysOn           bool // active even when disarmed; triggers immediately (fire/tamper/water)
	Immediate          bool // trips skip the entry delay (instant trigger when armed)
	UseExitDelay       bool // exempt from triggering during the ARMING (exit) countdown
	AutoBypass         bool // if open at arm time, silently bypass for that armed session
	AllowOpen          bool // arm-on-close: may arm while open; not live until it next closes
	TriggerUnavailable bool // treat an "unavailable" sensor as a trip while armed
	Group              string
}

// SensorGroup debounces false positives: a grouped sensor only triggers
// once EventCount distinct sensors in the group have tripped within
// Timeout of each other.
type SensorGroup struct {
	Name       string
	EventCount int
	Timeout    time.Duration
}

// SensorEventKind is an inbound sensor transition.
type SensorEventKind string

const (
	SensorClosed      SensorEventKind = "closed"
	SensorOpen        SensorEventKind = "open"
	SensorUnavailable SensorEventKind = "unavailable"
)

type sensorRuntime struct {
	cfg          SensorConfig
	open         bool
	unavailable  bool
	bypassed     bool // manual bypass (a switch entity)
	autoBypassed bool // auto-bypassed for the current armed session
	pendingClose bool // AllowOpen sensor that was open at arm; not live until it closes
}

// --- public API (synchronous, funneled through the owner goroutine) ---

// ConfigureSensors replaces the sensor + group configuration, preserving
// the live open/unavailable/bypass state of sensors whose IDs persist.
func (e *Engine) ConfigureSensors(sensors []SensorConfig, groups []SensorGroup) {
	e.submit(command{typ: cmdConfigureSensors, sensorCfg: sensors, groups: groups})
}

// SensorEvent reports a sensor transition (open/closed/unavailable).
func (e *Engine) SensorEvent(id string, kind SensorEventKind) {
	e.submit(command{typ: cmdSensorEvent, sensorID: id, sensorEvt: kind})
}

// SetBypass manually bypasses (or restores) a sensor.
func (e *Engine) SetBypass(id string, bypass bool) {
	e.submit(command{typ: cmdSetBypass, sensorID: id, bypass: bypass})
}

// --- owner-goroutine handlers ---

func (e *Engine) handleConfigureSensors(c command) {
	next := make(map[string]*sensorRuntime, len(c.sensorCfg))
	for _, cfg := range c.sensorCfg {
		if cfg.ID == "" {
			continue
		}
		rt := &sensorRuntime{cfg: cfg}
		if prev, ok := e.sensors[cfg.ID]; ok {
			rt.open, rt.unavailable, rt.bypassed = prev.open, prev.unavailable, prev.bypassed
			rt.autoBypassed, rt.pendingClose = prev.autoBypassed, prev.pendingClose
		}
		next[cfg.ID] = rt
	}
	e.sensors = next
	e.groups = c.groups
	e.publish()
	e.reply(c, Result{Accepted: true, Snapshot: e.snapshot()})
}

func (e *Engine) handleSetBypass(c command) {
	if s := e.sensors[c.sensorID]; s != nil {
		s.bypassed = c.bypass
		e.publish()
	}
	e.reply(c, Result{Accepted: true, Snapshot: e.snapshot()})
}

func (e *Engine) handleSensorEvent(c command) {
	s := e.sensors[c.sensorID]
	if s == nil {
		// Unknown sensor: auto-register with permissive defaults so UniFi
		// discovery does not silently drop events.
		s = &sensorRuntime{cfg: SensorConfig{ID: c.sensorID, Name: c.sensorID}}
		e.sensors[c.sensorID] = s
	}
	wasTripped := s.open || s.unavailable
	switch c.sensorEvt {
	case SensorOpen:
		s.open = true
		s.unavailable = false
	case SensorUnavailable:
		s.unavailable = true
	case SensorClosed:
		s.open = false
		s.unavailable = false
		if s.pendingClose {
			s.pendingClose = false // an AllowOpen sensor closed → now live
		}
	}
	tripped := s.open || (s.unavailable && s.cfg.TriggerUnavailable)

	// Only act on a fresh trip (closed→open), not on repeated open reports.
	if tripped && !wasTripped {
		e.evaluateTrip(s)
	}
	e.publish()
	e.reply(c, Result{Accepted: true, Snapshot: e.snapshot()})
}

// evaluateTrip decides what a freshly-tripped sensor does given the
// current alarm state and the sensor's configuration.
func (e *Engine) evaluateTrip(s *sensorRuntime) {
	if s.bypassed || s.autoBypassed || s.pendingClose {
		return
	}
	if e.state == StateTriggered {
		return
	}

	// always_on sensors (fire/tamper/water) trigger immediately in ANY state.
	if s.cfg.AlwaysOn {
		e.groupedTrigger(s, true)
		return
	}

	switch {
	case isArmed(e.state):
		if !e.sensorActiveInMode(s, e.armMode) {
			return
		}
		e.groupedTrigger(s, s.cfg.Immediate)
	case e.state == StateArming:
		// During the exit countdown only immediate (instant) sensors fire;
		// regular sensors are exempt while the user leaves.
		if s.cfg.Immediate && e.sensorActiveInMode(s, e.armMode) {
			e.groupedTrigger(s, true)
		}
	case e.state == StatePending:
		// Already in the entry countdown; an immediate sensor escalates.
		if s.cfg.Immediate && e.sensorActiveInMode(s, e.armMode) {
			e.groupedTrigger(s, true)
		}
	}
}

// groupedTrigger applies sensor-group debouncing before triggering.
func (e *Engine) groupedTrigger(s *sensorRuntime, immediate bool) {
	if g := e.group(s.cfg.Group); g != nil && g.EventCount > 1 {
		if !e.groupSatisfied(s.cfg.Group, g) {
			return // not enough sensors in the group within the window yet
		}
	}
	e.sensorTrigger(immediate, s.cfg.Name)
}

func (e *Engine) group(name string) *SensorGroup {
	if name == "" {
		return nil
	}
	for i := range e.groups {
		if e.groups[i].Name == name {
			return &e.groups[i]
		}
	}
	return nil
}

// groupSatisfied records a hit for the group and reports whether the
// EventCount threshold is met within the group's timeout window.
func (e *Engine) groupSatisfied(name string, g *SensorGroup) bool {
	now := time.Now()
	hits := append(e.groupHits[name], now)
	if g.Timeout > 0 {
		cutoff := now.Add(-g.Timeout)
		hits = slices.DeleteFunc(hits, func(t time.Time) bool { return t.Before(cutoff) })
	}
	e.groupHits[name] = hits
	if len(hits) >= g.EventCount {
		delete(e.groupHits, name) // reset after firing
		return true
	}
	return false
}

// sensorTrigger drives the state machine from a sensor trip: immediate
// goes straight to TRIGGERED, otherwise an armed system enters the
// entry-delay PENDING state.
func (e *Engine) sensorTrigger(immediate bool, by string) {
	if !immediate && isArmed(e.state) && e.cfg.EntryDelay > 0 {
		e.priorArmMode = e.armMode
		e.changedBy = by
		e.changedByUserID = ""
		e.setState(StatePending)
		e.startTimer(e.cfg.EntryDelay)
		e.publish()
		return
	}
	e.toTriggered(Actor{Name: by, Role: "sensor"})
}

func (e *Engine) sensorActiveInMode(s *sensorRuntime, mode string) bool {
	if len(s.cfg.Modes) == 0 {
		return true
	}
	return slices.Contains(s.cfg.Modes, mode)
}

// sensorBlocksArming reports whether an open sensor prevents arming the
// given mode (auto-bypass and allow-open sensors never block).
func (e *Engine) sensorBlocksArming(s *sensorRuntime, mode string) bool {
	if s.bypassed || s.cfg.AutoBypass || s.cfg.AllowOpen {
		return false
	}
	tripped := s.open || (s.unavailable && s.cfg.TriggerUnavailable)
	if !tripped {
		return false
	}
	return s.cfg.AlwaysOn || e.sensorActiveInMode(s, mode)
}

// prepareArmSession applies auto-bypass and arm-on-close to open sensors
// as the system arms a mode.
func (e *Engine) prepareArmSession(mode string) {
	for _, s := range e.sensors {
		if !(s.open || (s.unavailable && s.cfg.TriggerUnavailable)) {
			continue
		}
		if !(s.cfg.AlwaysOn || e.sensorActiveInMode(s, mode)) {
			continue
		}
		switch {
		case s.cfg.AutoBypass:
			s.autoBypassed = true
		case s.cfg.AllowOpen:
			s.pendingClose = true
		}
	}
}

// clearArmSession resets transient per-session sensor flags on disarm.
func (e *Engine) clearArmSession() {
	for _, s := range e.sensors {
		s.autoBypassed = false
		s.pendingClose = false
	}
	clear(e.groupHits)
}

func (e *Engine) openSensorNames() []string {
	var out []string
	for _, s := range e.sensors {
		if (s.open || s.unavailable) && !s.bypassed && !s.autoBypassed {
			out = append(out, s.cfg.Name)
		}
	}
	slices.Sort(out)
	return out
}

func (e *Engine) bypassedSensorNames() []string {
	var out []string
	for _, s := range e.sensors {
		if s.bypassed || s.autoBypassed {
			out = append(out, s.cfg.Name)
		}
	}
	slices.Sort(out)
	return out
}

// readyToArm reports whether no sensor would block arming in any
// configured arm mode (a coarse, mode-agnostic readiness for the panel
// attribute; handleArm does the authoritative per-mode check).
func (e *Engine) readyToArm() bool {
	for _, s := range e.sensors {
		if s.bypassed || s.cfg.AutoBypass || s.cfg.AllowOpen {
			continue
		}
		if s.open || (s.unavailable && s.cfg.TriggerUnavailable) {
			return false
		}
	}
	return true
}
