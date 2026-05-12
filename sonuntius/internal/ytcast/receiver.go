// Maps to: src/lib/YouTubeCastReceiver.ts
//
// Receiver is the public-API root that ties YouTubeApp to the DIAL
// server. Hosts construct it with a Player implementation and options,
// then call Start to begin serving / Stop to shut down.
//
// The Go port mirrors upstream's lifecycle (STOPPED → STARTING →
// RUNNING → STOPPING), the option-defaulting (friendly name, screen
// name, data store), and the event re-emission from YouTubeApp to the
// receiver's own event bus.
package ytcast

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/shobuprime/sonuntius/internal/ytcast/constants"
	"github.com/shobuprime/sonuntius/internal/ytcast/datastore"
	"github.com/shobuprime/sonuntius/internal/ytcast/dial"
	"github.com/shobuprime/sonuntius/internal/ytcast/logger"
	"github.com/shobuprime/sonuntius/internal/ytcast/lounge"
	pkgplayer "github.com/shobuprime/sonuntius/internal/ytcast/player"
	"github.com/shobuprime/sonuntius/internal/ytcast/types"
)

// DeviceOptions ports the upstream `options.device` sub-object.
type DeviceOptions struct {
	// Name is the friendly name advertised over DIAL. Defaults to the
	// host's hostname when empty.
	Name string
	// ScreenName is the label shown in a sender app when paired
	// manually. Defaults to "YouTube on <Name>".
	ScreenName string
	// Brand / Model populate the UPnP device description.
	Brand string
	Model string
}

// Options ports `interface YouTubeCastReceiverOptions`. Zero-valued
// fields fall back to defaults that match upstream.
type Options struct {
	// Player is the host-provided playback implementation. Required.
	Player pkgplayer.Player
	// Device branding.
	Device DeviceOptions
	// Dial overrides DIAL server defaults (port, advertise interval, ...).
	Dial DialOptions
	// App overrides YouTubeApp defaults (autoplay-on-connect, mute
	// policy, reset policy, playlist request handler).
	App AppDefaults
	// DataStore persists session state (lounge tokens, screen IDs,
	// pid). Nil disables persistence — pass DataStore: NoStore to opt
	// out explicitly.
	DataStore datastore.DataStore
	// LogLevel sets the level on the bundled DefaultLogger. Ignored
	// when Logger is set.
	LogLevel logger.LogLevel
	// Logger is an externally-constructed logger. When nil, a stderr-
	// backed DefaultLogger is used.
	Logger logger.Logger
}

// DialOptions is the subset of dial.Options that hosts may override.
// Friendly name / manufacturer / model are sourced from DeviceOptions,
// so we exclude them here.
type DialOptions struct {
	Port            int
	BindInterface   string
	AdvertisePeriod time.Duration
	Prefix          string
	AppName         string
	AllowStop       bool
	MaxBodyBytes    int64
	UUID            string
}

// AppDefaults is the subset of AppOptions hosts may override.
type AppDefaults struct {
	ScreenApp                     string
	EnableAutoplayOnConnect       *bool
	MutePolicy                    constants.MutePolicy
	ResetPlayerOnDisconnectPolicy constants.ResetPlayerOnDisconnectPolicy
	PlaylistRequestHandler        lounge.PlaylistRequestHandler
}

// Receiver is the public root. Construct with NewReceiver.
type Receiver struct {
	app    *YouTubeApp
	dial   *dial.Server
	log    logger.Logger

	mu     sync.Mutex
	status constants.Status
	uuid   string

	// Subscriber wires to forward AppEvent values to listeners.
	bus *ReceiverBus
}

// ReceiverBus is the typed event bus surfaced to hosts. We re-export it
// here so callers don't need to import the AppEventBus type.
type ReceiverBus = AppEventBus

// NewReceiver constructs a Receiver. Player must be non-nil; DataStore
// defaults to a temp-dir-backed FileStore when nil (matching upstream's
// `dataStore !== false` branch which constructs a DefaultDataStore).
//
// Errors are limited to obvious configuration failures (nil player,
// missing UUID and inability to derive one).
func NewReceiver(opts Options) (*Receiver, error) {
	if opts.Player == nil {
		return nil, fmt.Errorf("ytcast: Player is required")
	}

	log := opts.Logger
	if log == nil {
		log = logger.NewDefaultLogger(false)
	}
	if opts.LogLevel != "" {
		log.SetLevel(opts.LogLevel)
	} else {
		log.SetLevel(logger.LogLevelInfo)
	}

	friendly := opts.Device.Name
	if friendly == "" {
		if h, err := os.Hostname(); err == nil && h != "" {
			friendly = h
		} else {
			friendly = "YouTube Cast Receiver"
		}
	}
	screen := opts.Device.ScreenName
	if screen == "" {
		screen = "YouTube on " + friendly
	}

	store := opts.DataStore
	if store != nil {
		store.SetLogger(log)
	}

	appOpts := AppOptions{
		ScreenName:                    screen,
		ScreenApp:                     opts.App.ScreenApp,
		Brand:                         opts.Device.Brand,
		Model:                         opts.Device.Model,
		EnableAutoplayOnConnect:       opts.App.EnableAutoplayOnConnect,
		MutePolicy:                    opts.App.MutePolicy,
		ResetPlayerOnDisconnectPolicy: opts.App.ResetPlayerOnDisconnectPolicy,
		PlaylistRequestHandler:        opts.App.PlaylistRequestHandler,
		DataStore:                     store,
		Logger:                        log,
	}
	app := NewYouTubeApp(opts.Player, appOpts)

	dialOpts := dial.Options{
		Port:            opts.Dial.Port,
		FriendlyName:    friendly,
		Manufacturer:    opts.Device.Brand,
		ModelName:       opts.Device.Model,
		UUID:            opts.Dial.UUID,
		BindInterface:   opts.Dial.BindInterface,
		AdvertisePeriod: opts.Dial.AdvertisePeriod,
		Prefix:          opts.Dial.Prefix,
		AppName:         opts.Dial.AppName,
		AllowStop:       opts.Dial.AllowStop,
		MaxBodyBytes:    opts.Dial.MaxBodyBytes,
		Logger:          log,
	}
	server := dial.NewServer(dialOpts)

	r := &Receiver{
		app:    app,
		dial:   server,
		log:    log,
		status: constants.StatusStopped,
		uuid:   opts.Dial.UUID,
		bus:    app.Bus(),
	}

	// Wire DIAL launch → YouTubeApp.Launch.
	server.OnLaunch(func(ctx context.Context, req dial.LaunchRequest) error {
		theme := req.Sender["theme"]
		_, err := app.Launch(ctx, LaunchOptions{
			Theme:       theme,
			PairingCode: req.PairingCode,
		})
		return err
	})

	// Wire YouTubeApp termination → Receiver.Stop.
	go r.forwardTerminate()

	return r, nil
}

// Bus exposes the receiver's event bus to hosts.
func (r *Receiver) Bus() *ReceiverBus { return r.bus }

// EmitPlayerState forces the engine to re-emit its current state to
// the orchestrator's state-event bus. Hosts call this when the
// underlying player's position / volume / duration changed
// out-of-band (e.g. Music Assistant updated the entity over WS) so
// the orchestrator can forward the new values to connected senders
// as onStateChange / onVolumeChanged / nowPlaying Lounge messages.
func (r *Receiver) EmitPlayerState(ctx context.Context) error {
	return r.app.engine.EmitCurrentState(ctx)
}

// NotifyExternalStatus updates the engine's tracked player status and
// emits a fresh state event to all connected senders. Use this when
// playback transitions happen outside the receiver (e.g. the user
// paused the speaker via Music Assistant's own UI) so the cast sender
// reflects the new state instead of the engine's last-known status.
//
// EmitPlayerState by itself only re-emits the cached status — calling
// it after an MA-side pause leaves the phone showing the old status.
// NotifyExternalStatus is the right call when MA's queue event signals
// a real status transition (idle/paused/playing/buffering).
func (r *Receiver) NotifyExternalStatus(ctx context.Context, status constants.PlayerStatus) error {
	return r.app.engine.NotifyExternalStateChange(ctx, &status)
}

// Status returns the lifecycle state.
func (r *Receiver) Status() constants.Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

// Logger returns the configured logger.
func (r *Receiver) Logger() logger.Logger { return r.log }

// PairingCodeService exposes the YT pairing service so hosts can
// surface manual-pair codes to users.
func (r *Receiver) PairingCodeService() *lounge.PairingCodeRequestService {
	return r.app.PairingCodeService()
}

// ConnectedSenders returns the currently-connected senders.
func (r *Receiver) ConnectedSenders() []*types.Sender { return r.app.ConnectedSenders() }

// UpcomingVideo returns the engine's "what plays next" — see
// YouTubeApp.UpcomingVideo for semantics.
func (r *Receiver) UpcomingVideo() *types.Video { return r.app.UpcomingVideo() }

// EnableAutoplayOnConnect ports `enableAutoplayOnConnect(value)`.
func (r *Receiver) EnableAutoplayOnConnect(value bool) { r.app.EnableAutoplayOnConnect(value) }

// SetResetPlayerOnDisconnectPolicy ports the same method on the
// upstream class.
func (r *Receiver) SetResetPlayerOnDisconnectPolicy(p constants.ResetPlayerOnDisconnectPolicy) {
	r.app.SetResetPlayerOnDisconnectPolicy(p)
}

// SetLogLevel adjusts the logger's level.
func (r *Receiver) SetLogLevel(level logger.LogLevel) { r.log.SetLevel(level) }

// Start ports `async start()`. Idempotent if already running.
func (r *Receiver) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.status != constants.StatusStopped {
		r.mu.Unlock()
		return nil
	}
	r.status = constants.StatusStarting
	r.mu.Unlock()

	r.dial.SetState(dial.AppStateStarting)
	if err := r.app.Start(ctx); err != nil {
		_ = r.dial.Stop()
		r.mu.Lock()
		r.status = constants.StatusStopped
		r.mu.Unlock()
		return err
	}
	if err := r.dial.Start(ctx); err != nil {
		_ = r.app.Stop(ctx, nil)
		r.mu.Lock()
		r.status = constants.StatusStopped
		r.mu.Unlock()
		return err
	}
	r.dial.SetPID(r.app.PID())
	r.dial.SetState(dial.AppStateRunning)
	r.mu.Lock()
	r.status = constants.StatusRunning
	r.mu.Unlock()
	if l := r.log; l != nil {
		l.Info(fmt.Sprintf("[yt-cast-receiver] Receiver started (pid=%s, upstream=%s).",
			r.app.PID(), constants.UpstreamCommit))
	}
	return nil
}

// Stop ports `async stop()`. Idempotent.
func (r *Receiver) Stop(ctx context.Context) error {
	r.mu.Lock()
	if r.status != constants.StatusRunning {
		r.mu.Unlock()
		return nil
	}
	r.status = constants.StatusStopping
	r.mu.Unlock()

	r.dial.SetState(dial.AppStateStopped)
	var firstErr error
	if err := r.app.Stop(ctx, nil); err != nil {
		firstErr = err
	}
	if err := r.dial.Stop(); err != nil && firstErr == nil {
		firstErr = err
	}
	r.mu.Lock()
	r.status = constants.StatusStopped
	r.mu.Unlock()
	return firstErr
}

// forwardTerminate subscribes to AppTerminateEvent and tears down the
// receiver on irrecoverable failure.
func (r *Receiver) forwardTerminate() {
	sub := r.bus.Subscribe(8)
	for evt := range sub {
		if t, ok := evt.(AppTerminateEvent); ok {
			r.log.Error("[yt-cast-receiver] Receiver terminated due to error:", t.Err)
			_ = r.Stop(context.Background())
			return
		}
	}
}

// receiverDebug emits a debug record without going through the upstream
// logger surface (used by the wrapper binary so it can use slog directly
// in its own log line without forking).
func receiverDebug(l logger.Logger, args ...any) {
	if l == nil {
		_ = slog.Default()
		return
	}
	l.Debug(args...)
}

var _ = receiverDebug
