// Maps to: src/lib/app/YouTubeApp.ts
//
// YouTubeApp is the heart of the receiver: it owns the two lounge
// sessions (one per Client — YT and YTMUSIC), routes the DIAL launch
// signal to the right session, translates incoming lounge messages
// into Player method calls, and broadcasts Player state changes back
// out to the active session.
//
// The Go port preserves the upstream control flow function-by-function.
// Lifecycle: STOPPED → STARTING → RUNNING → STOPPING. One active
// session at a time; switching between sessions resets the player and
// disconnects the previous session's senders (matching upstream's
// `#checkAndSwitchActiveSession`).
package ytcast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/shobuprime/sonuntius/internal/ytcast/constants"
	"github.com/shobuprime/sonuntius/internal/ytcast/datastore"
	"github.com/shobuprime/sonuntius/internal/ytcast/logger"
	"github.com/shobuprime/sonuntius/internal/ytcast/lounge"
	pkgplayer "github.com/shobuprime/sonuntius/internal/ytcast/player"
	"github.com/shobuprime/sonuntius/internal/ytcast/types"
	"github.com/shobuprime/sonuntius/internal/ytcast/yterrors"
)

// AppOptions ports `interface AppOptions` from upstream YouTubeApp.ts.
type AppOptions struct {
	ScreenName                    string
	ScreenApp                     string
	Brand                         string
	Model                         string
	EnableAutoplayOnConnect       *bool
	MutePolicy                    constants.MutePolicy
	ResetPlayerOnDisconnectPolicy constants.ResetPlayerOnDisconnectPolicy
	PlaylistRequestHandler        lounge.PlaylistRequestHandler
	DataStore                     datastore.DataStore
	Logger                        logger.Logger
}

// AppEvent is the closed event taxonomy YouTubeApp emits to its host.
// Upstream uses EventEmitter — we use a typed channel-based bus so the
// host (and `YouTubeCastReceiver`) can subscribe.
type AppEvent interface {
	appEventTag()
}

// SenderConnectedEvent is published when a sender finishes connecting.
type SenderConnectedEvent struct {
	Sender *types.Sender
}

func (SenderConnectedEvent) appEventTag() {}

// SenderDisconnectedEvent is published when a sender disconnects.
// Implicit is true when the disconnect was inferred rather than
// explicitly requested by the sender UI.
type SenderDisconnectedEvent struct {
	Sender   *types.Sender
	Implicit bool
}

func (SenderDisconnectedEvent) appEventTag() {}

// AppErrorEvent is published for non-terminal errors.
type AppErrorEvent struct {
	Err error
}

func (AppErrorEvent) appEventTag() {}

// AppTerminateEvent is published when YouTubeApp hits an irrecoverable
// error and the host receiver should stop.
type AppTerminateEvent struct {
	Err error
}

func (AppTerminateEvent) appEventTag() {}

// AppEventBus is the typed fan-out for AppEvent values. Slow
// subscribers drop events.
type AppEventBus struct {
	mu          sync.RWMutex
	subscribers []chan AppEvent
}

// NewAppEventBus constructs an empty AppEventBus.
func NewAppEventBus() *AppEventBus { return &AppEventBus{} }

// Subscribe returns a buffered channel that receives every event.
func (b *AppEventBus) Subscribe(bufferSize int) <-chan AppEvent {
	if bufferSize < 1 {
		bufferSize = 1
	}
	ch := make(chan AppEvent, bufferSize)
	b.mu.Lock()
	b.subscribers = append(b.subscribers, ch)
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes the subscription identified by `ch`.
func (b *AppEventBus) Unsubscribe(ch <-chan AppEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.subscribers[:0]
	for _, c := range b.subscribers {
		if (<-chan AppEvent)(c) == ch {
			close(c)
			continue
		}
		out = append(out, c)
	}
	b.subscribers = out
}

// Publish broadcasts evt to every subscriber.
func (b *AppEventBus) Publish(evt AppEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, c := range b.subscribers {
		select {
		case c <- evt:
		default:
		}
	}
}

// Close releases every subscriber channel.
func (b *AppEventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, c := range b.subscribers {
		close(c)
	}
	b.subscribers = nil
}

// YouTubeApp ports the upstream class. The receiver embeds an
// `appEventTag()` method via its concrete event types — the App itself
// is wholly internal.
type YouTubeApp struct {
	Name      string
	AllowStop bool

	pid       string
	state     constants.Status
	stateMu   sync.Mutex
	stateLock chan struct{} // serialise lifecycle transitions

	engine       *playerEngine
	sessions     map[types.ClientKey]*lounge.Session
	activeKey    types.ClientKey
	activeMu     sync.Mutex
	dataStore    datastore.DataStore
	logger       logger.Logger
	bus          *AppEventBus
	loungeEvents *lounge.EventBus

	connected            []*types.Sender
	implicitlyDisconn    []*types.Sender
	connMu               sync.Mutex
	autoplayModeOnConn   constants.AutoplayMode
	autoplayBeforeUnsup  constants.AutoplayMode
	autoplayBeforeUnsupV bool
	mutePolicy           constants.MutePolicy
	resetPolicy          constants.ResetPlayerOnDisconnectPolicy

	stateSub <-chan pkgplayer.StateEvent
	stopCh   chan struct{}
}

// NewYouTubeApp constructs a YouTubeApp wrapping the host's player.
// The engine wires the queue, autoplay, and state-event scaffolding.
func NewYouTubeApp(host pkgplayer.Player, opts AppOptions) *YouTubeApp {
	log := opts.Logger
	if log == nil {
		log = logger.NewDefaultLogger(false)
	}

	mutePolicy := opts.MutePolicy
	if mutePolicy == "" {
		mutePolicy = constants.MutePolicyAuto
	}
	resetPolicy := opts.ResetPlayerOnDisconnectPolicy
	if resetPolicy == "" {
		resetPolicy = constants.ResetPlayerOnDisconnectAllDisconnected
	}

	enableAutoplay := true
	if opts.EnableAutoplayOnConnect != nil {
		enableAutoplay = *opts.EnableAutoplayOnConnect
	}
	autoplayOnConn := constants.AutoplayModeDisabled
	if enableAutoplay {
		autoplayOnConn = constants.AutoplayModeEnabled
	}

	engine := newPlayerEngine(host, log)
	playlistHandler := opts.PlaylistRequestHandler
	if playlistHandler == nil {
		// Default stub — upstream uses youtubei.js; we skip metadata.
		playlistHandler = lounge.NewDefaultPlaylistRequestHandler(nil)
	}
	playlistHandler.SetLogger(log)
	engine.Queue().SetRequestHandler(playlistHandler)

	loungeEvents := lounge.NewEventBus()

	app := &YouTubeApp{
		Name:               "YouTube Cast Receiver App",
		AllowStop:          false,
		state:              constants.StatusStopped,
		engine:             engine,
		sessions:           make(map[types.ClientKey]*lounge.Session, 2),
		dataStore:          opts.DataStore,
		logger:             log,
		bus:                NewAppEventBus(),
		loungeEvents:       loungeEvents,
		autoplayModeOnConn: autoplayOnConn,
		mutePolicy:         mutePolicy,
		resetPolicy:        resetPolicy,
		stateLock:          make(chan struct{}, 1),
	}

	// Build per-Client sessions.
	for _, key := range []types.ClientKey{types.ClientKeyYT, types.ClientKeyYTMusic} {
		sess := lounge.NewSession(lounge.SessionOptions{
			Client:     types.Clients[key],
			ScreenName: opts.ScreenName,
			ScreenApp: func() string {
				if opts.ScreenApp != "" {
					return opts.ScreenApp
				}
				return constants.ConfDefaultScreenApp
			}(),
			Brand: func() string {
				if opts.Brand != "" {
					return opts.Brand
				}
				return constants.ConfDefaultBrand
			}(),
			Model: func() string {
				if opts.Model != "" {
					return opts.Model
				}
				return constants.ConfDefaultModel
			}(),
			DataStore: opts.DataStore,
			Logger:    log,
			EventBus:  loungeEvents,
		})
		app.sessions[key] = sess
	}

	return app
}

// Bus exposes the app's event channel.
func (a *YouTubeApp) Bus() *AppEventBus { return a.bus }

// UpcomingVideo returns the engine's best-guess "what plays next" —
// the explicit user-queued Next video if present, otherwise the
// autoplay candidate. Returns nil when no neighbour is available
// (single video, queue tail with autoplay off, etc.).
//
// Used by hosts that want to pre-load the next track somewhere
// downstream (e.g., adding it to Music Assistant's queue so MA
// auto-advances to our pick instead of its own library autoplay
// when the current item ends).
func (a *YouTubeApp) UpcomingVideo() *types.Video {
	state := a.engine.Queue().GetState()
	if state.Next != nil {
		return state.Next
	}
	if state.Autoplay != nil {
		return state.Autoplay
	}
	return nil
}

// LoungeEvents exposes the underlying lounge event bus (for hosts that
// want raw lounge events — primarily diagnostics).
func (a *YouTubeApp) LoungeEvents() *lounge.EventBus { return a.loungeEvents }

// PairingCodeService ports `getPairingCodeRequestService()` — upstream
// always returns the YT-flavoured service.
func (a *YouTubeApp) PairingCodeService() *lounge.PairingCodeRequestService {
	return a.sessions[types.ClientKeyYT].PairingCodeRequestService()
}

// ConnectedSenders returns a copy of the currently-connected senders.
func (a *YouTubeApp) ConnectedSenders() []*types.Sender {
	a.connMu.Lock()
	defer a.connMu.Unlock()
	out := make([]*types.Sender, len(a.connected))
	copy(out, a.connected)
	return out
}

// State returns the lifecycle state.
func (a *YouTubeApp) State() constants.Status {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	return a.state
}

// PID returns the per-session process id assigned at Start.
func (a *YouTubeApp) PID() string {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	return a.pid
}

// SetResetPlayerOnDisconnectPolicy ports
// `setResetPlayerOnDisconnectPolicy(value)`.
func (a *YouTubeApp) SetResetPlayerOnDisconnectPolicy(p constants.ResetPlayerOnDisconnectPolicy) {
	if p == "" {
		p = constants.ResetPlayerOnDisconnectAllDisconnected
	}
	a.stateMu.Lock()
	a.resetPolicy = p
	a.stateMu.Unlock()
}

// EnableAutoplayOnConnect ports `enableAutoplayOnConnect(value)`.
func (a *YouTubeApp) EnableAutoplayOnConnect(value bool) {
	a.stateMu.Lock()
	if value {
		a.autoplayModeOnConn = constants.AutoplayModeEnabled
	} else {
		a.autoplayModeOnConn = constants.AutoplayModeDisabled
	}
	a.stateMu.Unlock()
}

// Start ports `async start()`. Idempotent if already running.
func (a *YouTubeApp) Start(ctx context.Context) error {
	a.stateMu.Lock()
	if a.state != constants.StatusStopped {
		a.stateMu.Unlock()
		return nil
	}
	a.state = constants.StatusStarting
	a.stateMu.Unlock()

	// PID — load from datastore if present, else generate.
	if a.dataStore != nil {
		raw, err := a.dataStore.Get(ctx, "app.pid")
		if err == nil && len(raw) > 0 {
			var pid string
			if jerr := json.Unmarshal(raw, &pid); jerr == nil && pid != "" {
				a.stateMu.Lock()
				a.pid = pid
				a.stateMu.Unlock()
				a.debug("Setting app pid to stored value:", pid)
			}
		}
		if a.pid == "" {
			pid := generateCPN() // 16 hex chars is more than enough; PID is opaque.
			a.stateMu.Lock()
			a.pid = pid
			a.stateMu.Unlock()
			a.debug("Saving generated app pid:", pid)
			if err := a.dataStore.Set(ctx, "app.pid", pid); err != nil {
				a.warn("Failed to persist app pid:", err)
			}
		}
	} else if a.pid == "" {
		a.stateMu.Lock()
		a.pid = generateCPN()
		a.stateMu.Unlock()
	}

	a.debug("Starting YouTubeApp...")

	// Subscribe to engine state events.
	a.stateSub = a.engine.Bus().Subscribe(16)
	a.stopCh = make(chan struct{})
	go a.runStateLoop()

	// Wire session callbacks and begin both sessions in parallel.
	for key, sess := range a.sessions {
		k := key
		s := sess
		s.SetListener(lounge.SessionListener{
			OnMessages: func(session *lounge.Session, msgs []*lounge.Message) {
				if err := a.handleIncomingMessages(ctx, session, msgs); err != nil {
					a.errorLog("Caught error handling incoming message:", err)
				}
			},
			OnTerminate: func(err error) {
				go func() {
					if serr := a.Stop(context.Background(), err); serr != nil {
						a.errorLog("Caught error terminating YouTubeApp:", serr)
					}
				}()
			},
		})
		_ = k
	}

	// Begin sessions. Upstream calls Promise.all; we serialise here
	// because the lounge package's Begin is itself short and the
	// concurrency benefit is small — and serialising keeps error
	// propagation simple.
	var firstErr error
	for _, key := range []types.ClientKey{types.ClientKeyYT, types.ClientKeyYTMusic} {
		if err := a.sessions[key].Begin(ctx); err != nil {
			firstErr = err
			break
		}
	}
	if firstErr != nil {
		// Tear down anything that did start.
		for _, sess := range a.sessions {
			sess.SetListener(lounge.SessionListener{})
			_ = sess.End(context.Background(), nil)
		}
		close(a.stopCh)
		a.engine.Bus().Unsubscribe(a.stateSub)
		a.stateSub = nil
		a.stateMu.Lock()
		a.state = constants.StatusStopped
		a.stateMu.Unlock()
		return yterrors.NewAppError("Failed to start YouTubeApp", firstErr)
	}

	a.stateMu.Lock()
	a.state = constants.StatusRunning
	a.stateMu.Unlock()
	return nil
}

// Stop ports `async stop(error?)`. `cause` is the involuntary-termination
// trigger, nil for a clean Stop.
func (a *YouTubeApp) Stop(ctx context.Context, cause error) error {
	a.stateMu.Lock()
	if a.state != constants.StatusRunning && cause == nil {
		a.stateMu.Unlock()
		return nil
	}
	a.state = constants.StatusStopping
	a.stateMu.Unlock()

	a.debug("Stopping YouTubeApp...")

	a.connMu.Lock()
	senders := append([]*types.Sender(nil), a.connected...)
	a.connected = nil
	a.implicitlyDisconn = nil
	a.connMu.Unlock()

	if a.stopCh != nil {
		close(a.stopCh)
	}
	if a.stateSub != nil {
		a.engine.Bus().Unsubscribe(a.stateSub)
		a.stateSub = nil
	}
	a.engine.Reset(ctx, nil)

	for _, sess := range a.sessions {
		sess.SetListener(lounge.SessionListener{})
		if err := sess.End(ctx, nil); err != nil {
			a.warn("Ignoring error while stopping YouTubeApp:", err)
		}
	}

	a.stateMu.Lock()
	a.state = constants.StatusStopped
	a.stateMu.Unlock()

	for _, s := range senders {
		a.bus.Publish(SenderDisconnectedEvent{Sender: s, Implicit: false})
	}

	if cause != nil {
		a.bus.Publish(AppTerminateEvent{Err: cause})
	}
	return nil
}

// LaunchOptions is the parsed launch payload the DIAL server hands in.
// Theme / PairingCode are the only fields YouTubeApp.launch consults.
type LaunchOptions struct {
	Theme       string
	PairingCode string
}

// Launch ports `async launch(data)` — invoked from the DIAL server's
// OnLaunch handler with the parsed pairingCode + theme.
func (a *YouTubeApp) Launch(ctx context.Context, opts LaunchOptions) (string, error) {
	a.debug("YouTubeApp received DIAL launch request. Launch data:",
		map[string]string{"pairingCode": opts.PairingCode, "theme": opts.Theme})

	if opts.PairingCode == "" {
		return "", yterrors.NewAppError("Failed to launch YouTubeApp",
			yterrors.NewIncompleteAPIDataError("Invalid launch data", []string{"pairingCode"}))
	}
	client, ok := types.ClientByTheme(opts.Theme)
	if !ok {
		return "", yterrors.NewAppError("Failed to launch YouTubeApp",
			yterrors.NewIncompleteAPIDataError(
				fmt.Sprintf("Invalid launch data. Unknown theme: %s", opts.Theme),
				nil))
	}
	sess := a.sessionForClient(client)
	if sess == nil {
		return "", yterrors.NewAppError("Failed to launch YouTubeApp",
			yterrors.NewIncompleteAPIDataError(
				fmt.Sprintf("No session registered for client: %s", client.Name), nil))
	}

	if err := a.checkAndSwitchActiveSession(ctx, sess); err != nil {
		return "", err
	}
	a.info("Connecting sender through DIAL...")
	if err := sess.RegisterPairingCode(ctx, opts.PairingCode); err != nil {
		return "", err
	}
	return a.PID(), nil
}

// sessionForClient returns the session matching the given Client key.
func (a *YouTubeApp) sessionForClient(c types.Client) *lounge.Session {
	for _, sess := range a.sessions {
		if sess.Client().Key == c.Key {
			return sess
		}
	}
	return nil
}

// activeSession returns the currently-active session (or nil).
func (a *YouTubeApp) activeSession() *lounge.Session {
	a.activeMu.Lock()
	key := a.activeKey
	a.activeMu.Unlock()
	if key == "" {
		return nil
	}
	return a.sessions[key]
}

// setActiveSession atomically updates the active key. Pass empty to
// clear.
func (a *YouTubeApp) setActiveSession(s *lounge.Session) {
	a.activeMu.Lock()
	defer a.activeMu.Unlock()
	if s == nil {
		a.activeKey = ""
		return
	}
	a.activeKey = s.Client().Key
}

// checkAndSwitchActiveSession ports `#checkAndSwitchActiveSession(target)`.
func (a *YouTubeApp) checkAndSwitchActiveSession(ctx context.Context, target *lounge.Session) error {
	current := a.activeSession()
	if current != nil && current.Client().Key == target.Client().Key {
		return nil
	}
	if current == nil {
		a.setActiveSession(target)
		a.debug(fmt.Sprintf("Active session switched to '%s'.", target.Client().Name))
		return nil
	}
	a.debug(fmt.Sprintf("Target session is for %s clients, whereas active session is for %s clients.",
		target.Client().Name, current.Client().Name))
	a.debug("Switching over to target session, while disconnecting senders (if any) from old session.")
	a.engine.Reset(ctx, nil)

	a.connMu.Lock()
	oldSenders := append([]*types.Sender(nil), a.connected...)
	a.connected = nil
	a.implicitlyDisconn = nil
	a.connMu.Unlock()

	a.setActiveSession(target)
	// Disconnect events for the senders that were on the old session.
	for _, s := range oldSenders {
		a.bus.Publish(SenderDisconnectedEvent{Sender: s, Implicit: false})
	}
	// Best-effort loungeScreenDisconnected to the old session.
	go func(old *lounge.Session) {
		if _, err := old.SendMessage(context.Background(),
			[]*lounge.Message{lounge.NewLoungeScreenDisconnected()}, nil); err != nil {
			a.errorLog("Caught error sending message:", err)
		}
	}(current)
	a.debug(fmt.Sprintf("Active session switched to '%s'.", target.Client().Name))
	return nil
}

// handleIncomingMessages ports `#handleIncomingMessage`.
func (a *YouTubeApp) handleIncomingMessages(ctx context.Context, session *lounge.Session, msgs []*lounge.Message) error {
	for _, m := range msgs {
		if m == nil {
			continue
		}
		if err := a.handleIncomingMessage(ctx, session, m); err != nil {
			return err
		}
	}
	return nil
}

func (a *YouTubeApp) handleIncomingMessage(ctx context.Context, session *lounge.Session, m *lounge.Message) error {
	isActive := session == a.activeSession()
	client := session.Client()
	aidVal := m.AID
	a.debug("-----------------------------------")
	a.debug(fmt.Sprintf("(AID: %s) (%s) Incoming message: '%s'",
		formatAID(aidVal), client.Name, m.Name))

	var sendMessages []*lounge.Message

	switch m.Name {
	case "getNowPlaying":
		var ps *lounge.PlayerStateView
		if isActive {
			st, err := a.playerStateView(ctx)
			if err == nil {
				ps = &st
			}
		}
		sendMessages = append(sendMessages, lounge.NewNowPlaying(aidVal, ps))

	case "loungeStatus":
		payload := m.PayloadAsMap()
		devicesRaw, _ := payload["devices"].(string)
		var deviceList []json.RawMessage
		if devicesRaw != "" {
			var anyList []any
			if err := json.Unmarshal([]byte(devicesRaw), &anyList); err == nil {
				for _, d := range anyList {
					raw, _ := json.Marshal(d)
					deviceList = append(deviceList, raw)
				}
			} else {
				a.errorLog(fmt.Sprintf("(%s) Failed to parse 'devices' property of 'loungeStatus' message payload:", client.Name), err)
			}
		}
		var statusSenders []*types.Sender
		for _, raw := range deviceList {
			var typed struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(raw, &typed); err != nil || typed.Type != "REMOTE_CONTROL" {
				continue
			}
			s, err := types.Parse(raw)
			if err != nil {
				a.errorLog(fmt.Sprintf("(%s) Failed to parse sender data in 'loungeStatus' message:", client.Name), err)
				continue
			}
			statusSenders = append(statusSenders, s)
		}
		if a.State() == constants.StatusStarting {
			if len(statusSenders) > 0 {
				a.connMu.Lock()
				a.connected = statusSenders
				a.connMu.Unlock()
				a.debug(fmt.Sprintf("(%s) Updated connected senders info with 'loungeStatus' message:", client.Name), statusSenders)
				a.setActiveSession(session)
				a.debug(fmt.Sprintf("Active session switched to '%s'.", client.Name))
				if err := a.setAutoplayModeBySenderCapabilities(ctx, aidVal); err != nil {
					a.errorLog("autoplay reconcile failed:", err)
				}
				if err := a.enforceMutePolicy(ctx, aidVal); err != nil {
					a.errorLog("mute policy enforcement failed:", err)
				}
				nav := a.engine.NavInfo()
				sendMessages = append(sendMessages,
					lounge.NewOnHasPreviousNextChanged(aidVal, &lounge.PlayerNavInfo{HasPrevious: nav.HasPrevious, HasNext: nav.HasNext}),
					lounge.NewNowPlaying(aidVal, nil),
				)
			}
		} else {
			handled := false
			var detail map[string]any
			if raw, _ := payload["connectionEventDetails"].(string); raw != "" {
				if err := json.Unmarshal([]byte(raw), &detail); err != nil {
					wrapped := yterrors.NewSenderConnectionError(
						"Failed to handle sender connection / disconnection",
						err, yterrors.SenderConnectionActionUnknown)
					a.errorLog("Failed to parse connection event details in payload of loungeStatus message:", raw)
					a.bus.Publish(AppErrorEvent{Err: wrapped})
				}
			}
			if deviceID, _ := detail["deviceId"].(string); deviceID != "" {
				var connecting *types.Sender
				for _, s := range statusSenders {
					if s.ID == deviceID {
						connecting = s
						break
					}
				}
				var disconnecting *types.Sender
				if connecting == nil {
					a.connMu.Lock()
					for _, s := range a.connected {
						if s.ID == deviceID {
							disconnecting = s
							break
						}
					}
					a.connMu.Unlock()
				}
				if connecting != nil {
					msgs, err := a.handleSenderConnected(ctx, connecting, session, aidVal)
					if err != nil {
						return err
					}
					sendMessages = append(sendMessages, msgs...)
					handled = true
				}
				if disconnecting != nil {
					implicit := true
					if ui, _ := detail["ui"].(string); ui == "true" {
						implicit = false
					}
					if msgs, err := a.handleSenderDisconnected(ctx, disconnecting, session, implicit, aidVal); err != nil {
						return err
					} else {
						sendMessages = append(sendMessages, msgs...)
					}
					handled = true
				}
			}
			if !handled {
				var nav *lounge.PlayerNavInfo
				var autoplay *lounge.AutoplayInfo
				if isActive {
					n := a.engine.NavInfo()
					nav = &lounge.PlayerNavInfo{HasPrevious: n.HasPrevious, HasNext: n.HasNext}
					autoplay = &lounge.AutoplayInfo{Mode: n.AutoplayMode}
				}
				sendMessages = append(sendMessages,
					lounge.NewOnHasPreviousNextChanged(aidVal, nav),
					lounge.NewOnAutoplayModeChanged(aidVal, autoplay),
				)
			}
		}

	case "setPlaylist", "updatePlaylist":
		if !isActive {
			return nil
		}
		a.debug(fmt.Sprintf("'%s' message payload:", m.Name), m.PayloadAsMap())

		queueVideoIDs := a.engine.Queue().VideoIDs()
		payload := m.PayloadAsMap()
		msgPayloadVids, _ := payload["videoIds"].(string)
		var msgLast string
		if msgPayloadVids != "" {
			ids := splitCSV(msgPayloadVids)
			if len(ids) > 0 {
				msgLast = ids[len(ids)-1]
			}
		}
		autoplayDismissed := false
		if len(queueVideoIDs) == 0 || queueVideoIDs[len(queueVideoIDs)-1] != msgLast {
			if _, err := session.SendMessage(ctx, []*lounge.Message{lounge.NewAutoplayUpNext(aidVal, "")}, nil); err != nil {
				a.errorLog("Caught error sending message:", err)
			}
			autoplayDismissed = true
		}

		stateBefore := a.engine.Queue().GetState()
		if autoplayDismissed {
			stateBefore.Autoplay = nil
		}
		navBefore := a.engine.NavInfo()

		if _, err := a.engine.Queue().UpdateByMessage(ctx, m, client); err != nil {
			return err
		}
		stateAfter := a.engine.Queue().GetState()
		navAfter := a.engine.NavInfo()
		if videoID(stateBefore.Autoplay) != videoID(stateAfter.Autoplay) {
			sendMessages = append(sendMessages, lounge.NewAutoplayUpNext(aidVal, videoID(stateAfter.Autoplay)))
		}
		if m.Name == "setPlaylist" && (videoID(stateBefore.Current) != videoID(stateAfter.Current) ||
			videoContextIndex(stateBefore.Current) != videoContextIndex(stateAfter.Current)) {
			if a.engine.Status() != constants.PlayerStatusStopped {
				if _, err := a.engine.Stop(ctx, aidVal); err != nil {
					return err
				}
			}
			if stateAfter.Current != nil {
				curT := payload["currentTime"]
				position := 0.0
				switch v := curT.(type) {
				case string:
					if p, err := strconv.ParseFloat(v, 64); err == nil {
						position = p
					}
				case float64:
					position = v
				}
				if _, err := a.engine.Play(ctx, *stateAfter.Current, position, aidVal); err != nil {
					return err
				}
			}
		} else if m.Name == "updatePlaylist" && stateAfter.Current == nil {
			if _, err := a.engine.Stop(ctx, aidVal); err != nil {
				return err
			}
		} else {
			ps, err := a.playerStateView(ctx)
			if err == nil {
				sendMessages = append(sendMessages, lounge.NewNowPlaying(aidVal, &ps))
			}
			if navBefore.HasNext != navAfter.HasNext || navBefore.HasPrevious != navAfter.HasPrevious {
				sendMessages = append(sendMessages,
					lounge.NewOnHasPreviousNextChanged(aidVal, &lounge.PlayerNavInfo{HasPrevious: navAfter.HasPrevious, HasNext: navAfter.HasNext}))
			}
		}

	case "next":
		if !isActive {
			return nil
		}
		if _, err := a.engine.Next(ctx, aidVal); err != nil {
			return err
		}
	case "previous":
		if !isActive {
			return nil
		}
		if _, err := a.engine.Previous(ctx, aidVal); err != nil {
			return err
		}
	case "pause":
		if !isActive {
			return nil
		}
		if _, err := a.engine.Pause(ctx, aidVal); err != nil {
			return err
		}
	case "stopVideo":
		if !isActive {
			return nil
		}
		if _, err := a.engine.Stop(ctx, aidVal); err != nil {
			return err
		}
	case "seekTo":
		if !isActive {
			return nil
		}
		newTime, _ := m.PayloadAsMap()["newTime"].(string)
		pos, _ := strconv.ParseFloat(newTime, 64)
		if _, err := a.engine.Seek(ctx, pos, aidVal); err != nil {
			return err
		}
	case "getVolume":
		vol, err := a.engine.getVolume(ctx)
		if err != nil {
			return err
		}
		sendMessages = append(sendMessages, lounge.NewOnVolumeChanged(aidVal, vol))
	case "setVolume":
		if !isActive {
			return nil
		}
		payload := m.PayloadAsMap()
		mutedBool := false
		switch v := payload["muted"].(type) {
		case string:
			mutedBool = v == "true" || v == "True" || v == "TRUE"
		case bool:
			mutedBool = v
		}
		levelInt := 0
		switch v := payload["volume"].(type) {
		case string:
			if n, err := strconv.Atoi(v); err == nil {
				levelInt = n
			}
		case float64:
			levelInt = int(v)
		}
		newVol := pkgplayer.Volume{Level: levelInt, Muted: mutedBool}
		current, err := a.engine.getVolume(ctx)
		if err != nil {
			return err
		}
		if newVol.Level != current.Level || newVol.Muted != current.Muted {
			if _, err := a.engine.SetVolume(ctx, newVol, aidVal); err != nil {
				return err
			}
		}
	case "play":
		if !isActive {
			return nil
		}
		if _, err := a.engine.Resume(ctx, aidVal); err != nil {
			return err
		}
	case "setAutoplayMode":
		if !isActive {
			return nil
		}
		mode, _ := m.PayloadAsMap()["autoplayMode"].(string)
		if err := a.setAutoplayMode(ctx, aidVal, constants.AutoplayMode(mode)); err != nil {
			return err
		}
	default:
		a.debug(fmt.Sprintf("(AID: %s) (%s) Not handled: '%s'",
			formatAID(aidVal), client.Name, m.Name))
	}

	if len(sendMessages) > 0 {
		if _, err := session.SendMessage(ctx, sendMessages, nil); err != nil {
			a.errorLog("Caught error sending message:", err)
		}
	}
	return nil
}

// setAutoplayMode ports `#setAutoplayMode(AID, value)`.
func (a *YouTubeApp) setAutoplayMode(ctx context.Context, aid *int, value constants.AutoplayMode) error {
	active := a.activeSession()
	if active == nil {
		return nil
	}
	stateBefore := a.engine.Queue().GetState()
	if err := a.engine.Queue().SetAutoplayMode(ctx, value); err != nil {
		return err
	}
	stateAfter := a.engine.Queue().GetState()
	msgs := []*lounge.Message{
		lounge.NewOnAutoplayModeChanged(aid, &lounge.AutoplayInfo{Mode: value}),
	}
	if videoID(stateBefore.Autoplay) != videoID(stateAfter.Autoplay) {
		msgs = append(msgs, lounge.NewAutoplayUpNext(aid, videoID(stateAfter.Autoplay)))
	}
	if _, err := active.SendMessage(ctx, msgs, nil); err != nil {
		a.errorLog("Caught error sending message:", err)
	}
	return nil
}

// setAutoplayModeBySenderCapabilities ports
// `#setAutoplayModeBySenderCapabilities(AID)`.
func (a *YouTubeApp) setAutoplayModeBySenderCapabilities(ctx context.Context, aid *int) error {
	a.connMu.Lock()
	if len(a.connected) == 0 {
		a.connMu.Unlock()
		return nil
	}
	allSupport := true
	for _, s := range a.connected {
		if !s.SupportsAutoplay() {
			allSupport = false
			break
		}
	}
	a.connMu.Unlock()

	a.debug("Setting autoplay mode by sender(s) capabilities.")
	var mode constants.AutoplayMode
	if !allSupport {
		a.debug("(Some) sender(s) do not support autoplay. Autoplay support disabled.")
		if a.engine.AutoplayMode() != constants.AutoplayModeUnsupported {
			a.stateMu.Lock()
			a.autoplayBeforeUnsup = a.engine.AutoplayMode()
			a.autoplayBeforeUnsupV = true
			a.stateMu.Unlock()
		}
		mode = constants.AutoplayModeUnsupported
	} else {
		a.debug("(All) sender(s) support autoplay.")
		current := a.engine.AutoplayMode()
		if current != constants.AutoplayModeUnsupported {
			a.debug(fmt.Sprintf("Keeping current autoplay mode: %s", current))
			mode = current
		} else {
			a.stateMu.Lock()
			if a.autoplayBeforeUnsupV {
				mode = a.autoplayBeforeUnsup
			} else {
				mode = a.autoplayModeOnConn
			}
			a.stateMu.Unlock()
			a.debug(fmt.Sprintf("Setting autoplay mode: %s", mode))
		}
		a.stateMu.Lock()
		a.autoplayBeforeUnsupV = false
		a.stateMu.Unlock()
	}
	return a.setAutoplayMode(ctx, aid, mode)
}

// enforceMutePolicy ports `#enforceMutePolicy(AID)`.
func (a *YouTubeApp) enforceMutePolicy(ctx context.Context, aid *int) error {
	a.connMu.Lock()
	if len(a.connected) == 0 {
		a.connMu.Unlock()
		return nil
	}
	a.connMu.Unlock()

	a.debug("Configuring player based on mute policy...")
	const (
		logTrue  = "player will set volume level to 0 on mute"
		logFalse = "player will preserve volume level on mute"
	)
	a.stateMu.Lock()
	policy := a.mutePolicy
	a.stateMu.Unlock()
	var zero bool
	switch policy {
	case constants.MutePolicyZeroVolumeLevel:
		a.debug(fmt.Sprintf("Mute policy is 'ZERO_VOLUME_LEVEL': %s.", logTrue))
		zero = true
	case constants.MutePolicyPreserveVolumeLevel:
		a.debug(fmt.Sprintf("Mute policy is 'PRESERVE_VOLUME_LEVEL': %s.", logFalse))
		zero = false
	default: // MutePolicyAuto
		a.debug("Mute policy is 'AUTO': checking whether sender(s) support mute...")
		a.connMu.Lock()
		all := true
		for _, s := range a.connected {
			if !s.SupportsMute() {
				all = false
				break
			}
		}
		a.connMu.Unlock()
		if !all {
			a.debug(fmt.Sprintf("(Some) sender(s) do not support mute: %s.", logTrue))
			zero = true
		} else {
			a.debug(fmt.Sprintf("(All) sender(s) support mute: %s.", logFalse))
			zero = false
		}
	}
	return a.engine.SetZeroVolumeLevelOnMute(ctx, zero, aid)
}

// handleSenderConnected ports `#handleSenderConnected`.
func (a *YouTubeApp) handleSenderConnected(ctx context.Context, sender *types.Sender, session *lounge.Session, aid *int) ([]*lounge.Message, error) {
	if err := a.checkAndSwitchActiveSession(ctx, session); err != nil {
		return nil, err
	}
	client := session.Client()
	a.connMu.Lock()
	for _, c := range a.connected {
		if c.ID == sender.ID {
			a.connMu.Unlock()
			a.debug(fmt.Sprintf("(%s) Sender already connected.", client.Name))
			return nil, nil
		}
	}
	a.info(fmt.Sprintf("(%s) Sender connected: %s", client.Name, sender.Name))
	a.debug(fmt.Sprintf("(%s) Connected sender info:", client.Name), sender)
	// Drop from implicitly-disconnected list if present.
	out := a.implicitlyDisconn[:0]
	for _, s := range a.implicitlyDisconn {
		if s.ID != sender.ID {
			out = append(out, s)
		}
	}
	a.implicitlyDisconn = out
	a.connected = append(a.connected, sender)
	a.connMu.Unlock()

	if err := a.setAutoplayModeBySenderCapabilities(ctx, aid); err != nil {
		a.errorLog("autoplay reconcile failed:", err)
	}
	if err := a.enforceMutePolicy(ctx, aid); err != nil {
		a.errorLog("mute policy enforcement failed:", err)
	}
	nav := a.engine.NavInfo()
	ps, err := a.playerStateView(ctx)
	var psPtr *lounge.PlayerStateView
	if err == nil {
		psPtr = &ps
	}
	msgs := []*lounge.Message{
		lounge.NewOnHasPreviousNextChanged(aid, &lounge.PlayerNavInfo{HasPrevious: nav.HasPrevious, HasNext: nav.HasNext}),
		lounge.NewNowPlaying(aid, psPtr),
	}
	if psPtr != nil {
		msgs = append(msgs, lounge.NewOnStateChange(aid, *psPtr))
	}
	a.bus.Publish(SenderConnectedEvent{Sender: sender})
	return msgs, nil
}

// handleSenderDisconnected ports `#handleSenderDisconnected`.
func (a *YouTubeApp) handleSenderDisconnected(ctx context.Context, sender *types.Sender, session *lounge.Session, implicit bool, aid *int) ([]*lounge.Message, error) {
	if session != a.activeSession() {
		return nil, nil
	}
	client := session.Client()
	a.connMu.Lock()
	idx := -1
	for i, c := range a.connected {
		if c.ID == sender.ID {
			idx = i
			break
		}
	}
	if idx < 0 {
		a.connMu.Unlock()
		a.warn(fmt.Sprintf("(%s) Anomaly detected while unregistering disconnected sender: unable to find target among connected senders.", client.Name))
		return nil, nil
	}
	a.connected = append(a.connected[:idx], a.connected[idx+1:]...)
	implicitMsg := ""
	if implicit {
		implicitMsg = " (implicit)"
		known := false
		for _, s := range a.implicitlyDisconn {
			if s.ID == sender.ID {
				known = true
				break
			}
		}
		if !known {
			a.implicitlyDisconn = append(a.implicitlyDisconn, sender)
		}
	}
	remaining := len(a.connected)
	implicitDisconnCount := len(a.implicitlyDisconn)
	a.connMu.Unlock()

	a.info(fmt.Sprintf("(%s) Sender disconnected%s: %s", client.Name, implicitMsg, sender.Name))
	a.debug(fmt.Sprintf("(%s) Disconnected sender info:", client.Name), sender)

	if remaining == 0 {
		a.stateMu.Lock()
		policy := a.resetPolicy
		a.stateMu.Unlock()
		a.debug(fmt.Sprintf("No remaining senders connected. Reset player on disconnect policy is '%s'.", policy))
		reset := false
		if policy == constants.ResetPlayerOnDisconnectAllDisconnected {
			reset = true
		} else if policy == constants.ResetPlayerOnDisconnectAllExplicitlyDisconnected {
			if implicitDisconnCount > 0 {
				a.debug(fmt.Sprintf("Not resetting player because there is one or more implicitly disconnected sender (count: %d).", implicitDisconnCount))
			} else {
				a.debug("No implicitly disconnected senders.")
				reset = true
			}
		}
		if reset {
			a.debug("Resetting player...")
			a.engine.Reset(ctx, nil)
		}
	} else if a.engine.AutoplayMode() == constants.AutoplayModeUnsupported {
		if err := a.setAutoplayModeBySenderCapabilities(ctx, aid); err != nil {
			a.errorLog("autoplay reconcile failed:", err)
		}
	}

	if remaining > 0 {
		if err := a.enforceMutePolicy(ctx, aid); err != nil {
			a.errorLog("mute policy enforcement failed:", err)
		}
	}

	a.bus.Publish(SenderDisconnectedEvent{Sender: sender, Implicit: implicit})
	return nil, nil
}

// runStateLoop owns the engine state-event subscription. It maps each
// upstream `#handlePlayerStateEvent` into the right lounge messages.
func (a *YouTubeApp) runStateLoop() {
	sub := a.stateSub
	if sub == nil {
		return
	}
	for {
		select {
		case <-a.stopCh:
			return
		case ev, ok := <-sub:
			if !ok {
				return
			}
			a.handlePlayerStateEvent(ev)
		}
	}
}

// handlePlayerStateEvent ports `#handlePlayerStateEvent`.
func (a *YouTubeApp) handlePlayerStateEvent(ev pkgplayer.StateEvent) {
	active := a.activeSession()
	if active == nil {
		return
	}
	a.connMu.Lock()
	if len(a.connected) == 0 {
		a.connMu.Unlock()
		a.debug("Ignoring player state event because there is no connected sender.")
		return
	}
	a.connMu.Unlock()

	a.debug("Player state changed from:", ev.Previous)
	a.debug("To:", ev.Current)

	statusChanged := true
	positionChanged := true
	volumeChanged := true
	nowPlayingChanged := true
	autoplayChanged := true
	if ev.Previous != nil {
		statusChanged = ev.Previous.Status != ev.Current.Status
		positionChanged = ev.Previous.Position != ev.Current.Position
		volumeChanged = ev.Previous.Volume.Level != ev.Current.Volume.Level ||
			ev.Previous.Volume.Muted != ev.Current.Volume.Muted
		prevQ, prevOK := asPlaylistState(ev.Previous.Queue)
		curQ, curOK := asPlaylistState(ev.Current.Queue)
		if prevOK && curOK {
			nowPlayingChanged = videoID(prevQ.Current) != videoID(curQ.Current) ||
				prevQ.ID != curQ.ID ||
				videoContextIndex(prevQ.Current) != videoContextIndex(curQ.Current)
			autoplayChanged = videoID(prevQ.Autoplay) != videoID(curQ.Autoplay)
		}
	}

	curView := stateToView(ev.Current)
	var messages []*lounge.Message
	if statusChanged || positionChanged {
		messages = append(messages, lounge.NewOnStateChange(ev.AID, curView))
	}
	if nowPlayingChanged || (statusChanged && (ev.Previous == nil || ev.Previous.Status != constants.PlayerStatusPlaying)) {
		nav := a.engine.NavInfo()
		messages = append(messages,
			lounge.NewNowPlaying(ev.AID, &curView),
			lounge.NewOnHasPreviousNextChanged(ev.AID, &lounge.PlayerNavInfo{HasPrevious: nav.HasPrevious, HasNext: nav.HasNext}),
		)
	}
	if volumeChanged {
		messages = append(messages, lounge.NewOnVolumeChanged(ev.AID, ev.Current.Volume))
	}
	if autoplayChanged {
		if q, ok := asPlaylistState(ev.Current.Queue); ok {
			messages = append(messages, lounge.NewAutoplayUpNext(ev.AID, videoID(q.Autoplay)))
		}
	}
	if len(messages) == 0 {
		return
	}
	// If every message is OnVolumeChanged with non-nil AID, defer to
	// collapse setVolume bursts (upstream uses key=onVolumeChanged,
	// interval=200ms).
	allVolumeChange := true
	for _, m := range messages {
		if m.Name != "onVolumeChanged" || m.AID == nil {
			allVolumeChange = false
			break
		}
	}
	defer_ := (*lounge.SendDeferOptions)(nil)
	if allVolumeChange {
		defer_ = &lounge.SendDeferOptions{Key: "onVolumeChanged", Interval: 200_000_000} // 200ms
	}
	// Build a short summary of message names so the addon log shows
	// what state updates are being pushed to the sender — without this,
	// a perpetual loading spinner on the phone is opaque from the
	// receiver side.
	names := make([]string, 0, len(messages))
	for _, m := range messages {
		names = append(names, m.Name)
	}
	statusStr := fmt.Sprintf("%d", int(ev.Current.Status))
	a.info(fmt.Sprintf("Pushing player-state update to sender: names=%v status=%s", names, statusStr))
	go func(msgs []*lounge.Message, d *lounge.SendDeferOptions) {
		if _, err := active.SendMessage(context.Background(), msgs, d); err != nil {
			a.errorLog("Caught error sending message:", err)
		}
	}(messages, defer_)
}

// playerStateView builds the lounge.PlayerStateView projection from the
// engine's current state.
func (a *YouTubeApp) playerStateView(ctx context.Context) (lounge.PlayerStateView, error) {
	st, err := a.engine.GetState(ctx)
	if err != nil {
		return lounge.PlayerStateView{}, err
	}
	return stateToView(st), nil
}

// stateToView converts a pkgplayer.State into the projection the
// lounge message constructors consume.
func stateToView(st pkgplayer.State) lounge.PlayerStateView {
	view := lounge.PlayerStateView{
		Status:   st.Status,
		Position: st.Position,
		Duration: st.Duration,
		CPN:      st.CPN,
	}
	if q, ok := asPlaylistState(st.Queue); ok && q.Current != nil {
		view.Queue.Current = &lounge.Video{ID: q.Current.ID}
		if q.Current.Context != nil {
			view.Queue.Current.Context = &lounge.VideoContext{
				PlaylistID: q.Current.Context.PlaylistID,
				CTT:        q.Current.Context.CTT,
				Params:     q.Current.Context.Params,
			}
			if q.Current.Context.Index != nil {
				idx := *q.Current.Context.Index
				view.Queue.Current.Context.Index = &idx
			}
		}
	}
	return view
}

func asPlaylistState(v any) (lounge.PlaylistState, bool) {
	if v == nil {
		return lounge.PlaylistState{}, false
	}
	if ps, ok := v.(lounge.PlaylistState); ok {
		return ps, true
	}
	return lounge.PlaylistState{}, false
}

func videoID(v *types.Video) string {
	if v == nil {
		return ""
	}
	return v.ID
}

func videoContextIndex(v *types.Video) int {
	if v == nil || v.Context == nil || v.Context.Index == nil {
		return -1
	}
	return *v.Context.Index
}

func formatAID(aid *int) string {
	if aid == nil {
		return "nil"
	}
	return strconv.Itoa(*aid)
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	out := make([]string, 0, 4)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func (a *YouTubeApp) debug(args ...any) {
	if a.logger != nil {
		a.logger.Debug(prependTag(args)...)
	}
}

func (a *YouTubeApp) info(args ...any) {
	if a.logger != nil {
		a.logger.Info(prependTag(args)...)
	}
}

func (a *YouTubeApp) warn(args ...any) {
	if a.logger != nil {
		a.logger.Warn(prependTag(args)...)
	}
}

func (a *YouTubeApp) errorLog(args ...any) {
	if a.logger != nil {
		a.logger.Error(prependTag(args)...)
	}
}

func prependTag(args []any) []any {
	out := make([]any, 0, len(args)+1)
	out = append(out, "[yt-cast-receiver]")
	out = append(out, args...)
	return out
}

// Compile-time errors guard. Unused imports above are protected from
// `goimports` removing them by these references.
var _ = errors.Is
