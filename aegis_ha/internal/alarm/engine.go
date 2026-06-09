// Package alarm implements AegisHA's authoritative alarm state machine.
//
// The design mirrors Alarmo's behavioral model: two distinct countdown
// states — ARMING (exit/leave delay) and PENDING (entry delay) — plus a
// TRIGGERED state bounded by trigger_time. All state lives in a single
// owner goroutine (Run); callers interact through synchronous command
// methods (Arm/Disarm/Trigger/SkipDelay) and observe changes through
// immutable Snapshots delivered to Subscribe() channels. Delay timers run
// as time.AfterFunc callbacks that post a TimerFired command back to the
// owner, so no state is ever mutated off the owner goroutine.
//
// Authorization (PIN validation, roles, lockout) is NOT done here — the
// caller (the MQTT bridge or web UI) resolves and authorizes the Actor
// before submitting a command. The engine enforces only alarm logic
// (valid transitions, open-sensor gating, timing).
package alarm

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// State is an alarm_control_panel state string. The values match exactly
// what Home Assistant's MQTT alarm_control_panel expects on its state
// topic, so the MQTT bridge can publish a Snapshot.State verbatim.
type State string

const (
	StateDisarmed          State = "disarmed"
	StateArming            State = "arming"
	StatePending           State = "pending"
	StateTriggered         State = "triggered"
	StateArmedAway         State = "armed_away"
	StateArmedHome         State = "armed_home"
	StateArmedNight        State = "armed_night"
	StateArmedVacation     State = "armed_vacation"
	StateArmedCustomBypass State = "armed_custom_bypass"
)

// Actor identifies who initiated a command. It is already authorized by
// the caller; the engine only records it for changed_by reporting.
type Actor struct {
	Name   string
	UserID string
	Role   string
}

// Config holds the timing and policy knobs the engine needs.
type Config struct {
	ExitDelay           time.Duration
	EntryDelay          time.Duration
	TriggerTime         time.Duration // 0 == indefinite
	ArmModes            []string
	DisarmAfterTrigger  bool
	RestoreAfterTrigger bool // ignore_blocking_sensors_after_trigger

	// RestoreArmMode, if set to a valid arm mode, starts the engine in the
	// corresponding armed_* state (used to restore a committed armed state
	// across an app restart). Transient countdown states are never
	// restored — they fail safe to disarmed.
	RestoreArmMode string
	// OnCommit is called with the arm mode whenever the engine settles into
	// a committed armed_* state, and with "" when it settles into disarmed.
	// It is the persistence hook for RestoreArmMode. Never called for the
	// transient arming/pending/triggered states.
	OnCommit func(armMode string)
}

// Snapshot is an immutable view of the alarm at a point in time.
type Snapshot struct {
	State           State    `json:"state"`
	ArmMode         string   `json:"arm_mode,omitempty"`
	PriorArmMode    string   `json:"prior_arm_mode,omitempty"`
	ChangedBy       string   `json:"changed_by,omitempty"`
	ChangedByUserID string   `json:"changed_by_user_id,omitempty"`
	OpenSensors     []string `json:"open_sensors"`
	BypassedSensors []string `json:"bypassed_sensors"`
	DelayTotal      int      `json:"delay_total"`        // seconds, 0 if no countdown
	DelayEndsUnix   int64    `json:"delay_ends,omitempty"` // unix seconds the countdown ends
	ReadyToArm      bool     `json:"ready_to_arm"`
	Sequence        uint64   `json:"sequence"`
}

type cmdType int

const (
	cmdArm cmdType = iota
	cmdDisarm
	cmdTrigger
	cmdSkipDelay
	cmdTimerFired
	cmdSetConfig
	cmdConfigureSensors
	cmdSensorEvent
	cmdSetBypass
)

type command struct {
	typ       cmdType
	mode      string
	immediate bool
	force     bool
	actor     Actor
	cfg       *Config
	sensorCfg []SensorConfig
	groups    []SensorGroup
	sensorID  string
	sensorEvt SensorEventKind
	bypass    bool
	gen       uint64
	reply     chan Result
}

// Result is the outcome of a submitted command.
type Result struct {
	Accepted bool
	Reason   string // e.g. open_sensors, invalid_mode, not_disarmed, no_delay_active
	Snapshot Snapshot
}

// Engine is the alarm state machine. Construct with New, then run Run in
// its own goroutine.
type Engine struct {
	log  *slog.Logger
	cmds chan command
	cfg  Config

	state           State
	armMode         string
	priorArmMode    string
	changedBy       string
	changedByUserID string

	sensors   map[string]*sensorRuntime
	groups    []SensorGroup
	groupHits map[string][]time.Time

	timer      *time.Timer
	gen        uint64
	delayTotal int
	delayEnds  time.Time
	seq        uint64
	lastCommit string

	latest atomic.Pointer[Snapshot]

	mu   sync.Mutex
	subs []chan Snapshot
}

// New creates an Engine in the disarmed state.
func New(cfg Config, log *slog.Logger) *Engine {
	if log == nil {
		log = slog.Default()
	}
	e := &Engine{
		log:       log,
		cmds:      make(chan command, 32),
		cfg:       cfg,
		state:     StateDisarmed,
		sensors:   map[string]*sensorRuntime{},
		groupHits: map[string][]time.Time{},
	}
	if st, ok := armedStateForMode(cfg.RestoreArmMode); ok {
		e.state = st
		e.armMode = cfg.RestoreArmMode
		e.changedBy = "restored"
		e.lastCommit = cfg.RestoreArmMode
	}
	e.publish()
	return e
}

// Run owns all state. It blocks until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			e.clearTimer()
			return
		case c := <-e.cmds:
			e.handle(c)
		}
	}
}

// --- public command API (synchronous) ---

// Arm requests an armed mode. force bypasses open sensors.
func (e *Engine) Arm(mode string, actor Actor, force bool) Result {
	return e.submit(command{typ: cmdArm, mode: mode, actor: actor, force: force})
}

// Disarm returns to the disarmed state from any state.
func (e *Engine) Disarm(actor Actor) Result {
	return e.submit(command{typ: cmdDisarm, actor: actor})
}

// Trigger fires the alarm. immediate skips the entry delay (panic);
// otherwise an armed system enters the entry-delay PENDING state first.
func (e *Engine) Trigger(immediate bool, actor Actor) Result {
	return e.submit(command{typ: cmdTrigger, immediate: immediate, actor: actor})
}

// SkipDelay collapses an active ARMING or PENDING countdown immediately.
func (e *Engine) SkipDelay(actor Actor) Result {
	return e.submit(command{typ: cmdSkipDelay, actor: actor})
}

// SetConfig swaps the live timing/policy configuration.
func (e *Engine) SetConfig(cfg Config) {
	e.submit(command{typ: cmdSetConfig, cfg: &cfg})
}

// Subscribe returns a channel that receives a Snapshot on every change,
// primed immediately with the current snapshot.
func (e *Engine) Subscribe() <-chan Snapshot {
	ch := make(chan Snapshot, 16)
	e.mu.Lock()
	e.subs = append(e.subs, ch)
	e.mu.Unlock()
	if s := e.latest.Load(); s != nil {
		ch <- *s
	}
	return ch
}

// Current returns the latest snapshot without blocking on the owner.
func (e *Engine) Current() Snapshot {
	if s := e.latest.Load(); s != nil {
		return *s
	}
	return Snapshot{State: StateDisarmed}
}

func (e *Engine) submit(c command) Result {
	c.reply = make(chan Result, 1)
	e.cmds <- c
	return <-c.reply
}

// --- owner-goroutine handlers ---

func (e *Engine) handle(c command) {
	switch c.typ {
	case cmdArm:
		e.handleArm(c)
	case cmdDisarm:
		e.handleDisarm(c)
	case cmdTrigger:
		e.handleTrigger(c)
	case cmdSkipDelay:
		e.handleSkipDelay(c)
	case cmdTimerFired:
		e.handleTimerFired(c)
	case cmdSetConfig:
		e.cfg = *c.cfg
		e.reply(c, Result{Accepted: true, Snapshot: e.snapshot()})
	case cmdConfigureSensors:
		e.handleConfigureSensors(c)
	case cmdSensorEvent:
		e.handleSensorEvent(c)
	case cmdSetBypass:
		e.handleSetBypass(c)
	}
}

func (e *Engine) handleArm(c command) {
	if !e.modeAllowed(c.mode) {
		e.reply(c, Result{Reason: "invalid_mode", Snapshot: e.snapshot()})
		return
	}
	if e.state == StateTriggered || e.state == StatePending {
		e.reply(c, Result{Reason: "not_disarmed", Snapshot: e.snapshot()})
		return
	}
	if !c.force {
		for _, s := range e.sensors {
			if e.sensorBlocksArming(s, c.mode) {
				e.reply(c, Result{Reason: "open_sensors", Snapshot: e.snapshot()})
				return
			}
		}
	}
	e.armMode = c.mode
	e.changedBy = c.actor.Name
	e.changedByUserID = c.actor.UserID
	e.prepareArmSession(c.mode)
	if e.cfg.ExitDelay <= 0 {
		st, _ := armedStateForMode(c.mode)
		e.clearTimer()
		e.setState(st)
		e.publish()
	} else {
		e.setState(StateArming)
		e.startTimer(e.cfg.ExitDelay)
		e.publish()
	}
	e.reply(c, Result{Accepted: true, Snapshot: e.snapshot()})
}

func (e *Engine) handleDisarm(c command) {
	if e.state == StateDisarmed {
		e.reply(c, Result{Accepted: true, Reason: "already_disarmed", Snapshot: e.snapshot()})
		return
	}
	e.armMode = ""
	e.priorArmMode = ""
	e.clearTimer()
	e.clearArmSession()
	e.changedBy = c.actor.Name
	e.changedByUserID = c.actor.UserID
	e.setState(StateDisarmed)
	e.publish()
	e.reply(c, Result{Accepted: true, Snapshot: e.snapshot()})
}

func (e *Engine) handleTrigger(c command) {
	if e.state == StateTriggered {
		e.reply(c, Result{Accepted: true, Snapshot: e.snapshot()})
		return
	}
	if !c.immediate && isArmed(e.state) && e.cfg.EntryDelay > 0 {
		e.priorArmMode = e.armMode
		e.changedBy = c.actor.Name
		e.changedByUserID = c.actor.UserID
		e.setState(StatePending)
		e.startTimer(e.cfg.EntryDelay)
		e.publish()
	} else {
		e.toTriggered(c.actor)
	}
	e.reply(c, Result{Accepted: true, Snapshot: e.snapshot()})
}

func (e *Engine) handleSkipDelay(c command) {
	switch e.state {
	case StateArming:
		st, _ := armedStateForMode(e.armMode)
		e.clearTimer()
		e.setState(st)
		e.publish()
		e.reply(c, Result{Accepted: true, Snapshot: e.snapshot()})
	case StatePending:
		e.toTriggered(c.actor)
		e.reply(c, Result{Accepted: true, Snapshot: e.snapshot()})
	default:
		e.reply(c, Result{Reason: "no_delay_active", Snapshot: e.snapshot()})
	}
}

func (e *Engine) handleTimerFired(c command) {
	if c.gen != e.gen {
		return // stale fire from a cancelled/replaced timer
	}
	switch e.state {
	case StateArming:
		st, _ := armedStateForMode(e.armMode)
		e.clearTimer()
		e.setState(st)
		e.publish()
	case StatePending:
		e.toTriggered(Actor{Name: e.changedBy, UserID: e.changedByUserID})
	case StateTriggered:
		e.resolveTrigger()
	}
}

func (e *Engine) toTriggered(actor Actor) {
	if e.priorArmMode == "" && isArmed(e.state) {
		e.priorArmMode = e.armMode
	}
	e.changedBy = actor.Name
	e.changedByUserID = actor.UserID
	e.clearTimer()
	e.setState(StateTriggered)
	if e.cfg.TriggerTime > 0 {
		e.startTimer(e.cfg.TriggerTime)
	}
	e.publish()
}

func (e *Engine) resolveTrigger() {
	e.clearTimer()
	if e.cfg.DisarmAfterTrigger || e.priorArmMode == "" {
		e.armMode = ""
		e.setState(StateDisarmed)
	} else {
		e.armMode = e.priorArmMode
		st, _ := armedStateForMode(e.priorArmMode)
		e.setState(st)
	}
	e.priorArmMode = ""
	e.publish()
}

// --- helpers ---

func (e *Engine) setState(s State) {
	e.state = s
	e.seq++
}

func (e *Engine) startTimer(d time.Duration) {
	e.clearTimer()
	gen := e.gen
	e.delayTotal = int(d / time.Second)
	e.delayEnds = time.Now().Add(d)
	e.timer = time.AfterFunc(d, func() {
		e.cmds <- command{typ: cmdTimerFired, gen: gen}
	})
}

// clearTimer stops any pending timer and bumps the generation so an
// already-in-flight fire is recognized as stale and ignored.
func (e *Engine) clearTimer() {
	if e.timer != nil {
		e.timer.Stop()
		e.timer = nil
	}
	e.gen++
	e.delayTotal = 0
	e.delayEnds = time.Time{}
}

func (e *Engine) publish() {
	snap := e.snapshot()
	e.latest.Store(&snap)
	e.commitIfSettled()
	e.mu.Lock()
	subs := append([]chan Snapshot(nil), e.subs...)
	e.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- snap:
		default: // slow subscriber: drop; it will catch up on the next change
		}
	}
}

// commitIfSettled persists the committed arm mode when the engine reaches
// an armed_* or disarmed state, deduping unchanged values. Transient
// states (arming/pending/triggered) keep the last committed value.
func (e *Engine) commitIfSettled() {
	var committed string
	switch {
	case isArmed(e.state):
		committed = e.armMode
	case e.state == StateDisarmed:
		committed = ""
	default:
		return // transient state — leave persisted value untouched
	}
	if committed == e.lastCommit {
		return
	}
	e.lastCommit = committed
	if e.cfg.OnCommit != nil {
		e.cfg.OnCommit(committed)
	}
}

func (e *Engine) snapshot() Snapshot {
	var ends int64
	if !e.delayEnds.IsZero() {
		ends = e.delayEnds.Unix()
	}
	return Snapshot{
		State:           e.state,
		ArmMode:         e.armMode,
		PriorArmMode:    e.priorArmMode,
		ChangedBy:       e.changedBy,
		ChangedByUserID: e.changedByUserID,
		OpenSensors:     e.openSensorNames(),
		BypassedSensors: e.bypassedSensorNames(),
		DelayTotal:      e.delayTotal,
		DelayEndsUnix:   ends,
		ReadyToArm:      e.readyToArm(),
		Sequence:        e.seq,
	}
}

func (e *Engine) reply(c command, r Result) {
	if c.reply != nil {
		c.reply <- r
	}
}

func (e *Engine) modeAllowed(mode string) bool {
	if _, ok := armedStateForMode(mode); !ok {
		return false
	}
	if len(e.cfg.ArmModes) == 0 {
		return true
	}
	return slices.Contains(e.cfg.ArmModes, mode)
}

func armedStateForMode(mode string) (State, bool) {
	switch mode {
	case "away":
		return StateArmedAway, true
	case "home":
		return StateArmedHome, true
	case "night":
		return StateArmedNight, true
	case "vacation":
		return StateArmedVacation, true
	case "custom", "custom_bypass":
		return StateArmedCustomBypass, true
	}
	return "", false
}

func isArmed(s State) bool {
	switch s {
	case StateArmedAway, StateArmedHome, StateArmedNight, StateArmedVacation, StateArmedCustomBypass:
		return true
	}
	return false
}
