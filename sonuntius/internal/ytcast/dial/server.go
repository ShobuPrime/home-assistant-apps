// Maps to: src/lib/dial/DialServer.ts
//
// Package dial implements the DIAL 1.7 discovery + launch protocol that
// upstream `yt-cast-receiver` delegates to `@patrickkfkan/peer-dial` (SSDP
// over UDP multicast + a small Express HTTP server). Ports both the
// peer-dial wire behaviour and the upstream `DialServer` orchestration into
// a single Go package backed by stdlib only.
//
// The package layout deliberately mirrors the conceptual split inside
// peer-dial:
//
//   - server.go    — public API (Options, Server, Start/Stop, callbacks)
//                    that maps to the upstream DialServer class.
//   - ssdp.go      — SSDP M-SEARCH responder + NOTIFY broadcaster.
//                    Replaces peer-ssdp / peer-dial discovery internals.
//   - upnp.go      — UPnP device description XML + DIAL app descriptor XML
//                    + the HTTP route handlers. Replaces peer-dial/express.
//   - launch_payload.go — parses the form-encoded DIAL launch POST body.
//
// Every file in the package opens with the same `// Maps to:` discipline as
// the wider sonuntius/internal/ytcast tree so that future audits can re-walk
// from the upstream source to its Go counterpart.
package dial

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shobuprime/sonuntius/internal/ytcast/constants"
	"github.com/shobuprime/sonuntius/internal/ytcast/logger"
	"github.com/shobuprime/sonuntius/internal/ytcast/yterrors"
)

// Defaults that match upstream peer-dial / DialServer constants. Any caller
// can override these via Options.
const (
	// DefaultPort is the HTTP listen port. Upstream picks 3000.
	DefaultPort = 3000
	// DefaultPrefix is the URL prefix prepended to every DIAL endpoint.
	// Upstream's `peer-dial` defaults to "/ytcr"; senders that have been
	// tested against yt-cast-receiver expect that path.
	DefaultPrefix = "/ytcr"
	// DefaultAppName is the registered DIAL application name. Only "YouTube"
	// is wired up upstream — anything else throws DialServerError.
	DefaultAppName = "YouTube"
	// DefaultAdvertisePeriod is how often we resend NOTIFY ssdp:alive.
	// Upstream peer-ssdp defaults to ~30s.
	DefaultAdvertisePeriod = 30 * time.Second
	// DefaultMaxBodyBytes is the cap upstream peer-dial enforces on launch
	// POST bodies (`maxContentLength`, minimum 4096).
	DefaultMaxBodyBytes int64 = 4096
)

// AppState is the value reported under `<state>` in the DIAL app descriptor.
// Mirrors the three states upstream surfaces ("stopped" / "starting" /
// "running"); callers update it via Server.SetState as the orchestrator
// transitions YouTubeApp through its lifecycle.
type AppState string

// Recognised AppState values.
const (
	AppStateStopped  AppState = "stopped"
	AppStateStarting AppState = "starting"
	AppStateRunning  AppState = "running"
)

// LaunchRequest is the parsed payload of a DIAL launch POST. The Phase 3
// orchestrator's OnLaunch handler reads `PairingCode` to pair with the
// sender, and may inspect `Sender` / `RawBody` for any additional
// sender-supplied parameters.
type LaunchRequest struct {
	// PairingCode is the value of the `pairingCode` form field. YouTube
	// senders place the lounge pairing code here when launching via DIAL.
	// Empty if the sender omitted it.
	PairingCode string
	// Sender holds every other key/value pair from the form body. Multi-value
	// fields are joined with "," to match how `application/x-www-form-urlencoded`
	// would render them through ParseForm.
	Sender map[string]string
	// RawBody is the raw POST body (capped at Options.MaxBodyBytes). Kept
	// for debug logs and for senders that switch to a non-form encoding.
	RawBody []byte
	// RemoteAddr is the sender IP:port that originated the launch.
	RemoteAddr string
}

// LaunchHandler is the callback the host registers via Server.OnLaunch.
// Returning a non-nil error causes the HTTP response to be 503; returning
// nil yields 201 (when the app was previously stopped) or 200 (otherwise),
// matching upstream peer-dial behaviour.
type LaunchHandler func(ctx context.Context, req LaunchRequest) error

// StopHandler is the callback registered via Server.OnStop. It is only
// invoked when AllowStop is true — upstream YouTubeApp keeps it false and
// stops the app via YouTubeCastReceiver.stop() instead.
type StopHandler func(ctx context.Context) error

// Options configure a Server. Zero-valued fields fall back to the Default*
// constants above (or to constants.ConfDefault* for branding fields).
type Options struct {
	// Port is the TCP port the HTTP server listens on. Defaults to DefaultPort.
	Port int
	// FriendlyName is the UPnP `<friendlyName>` advertised in device-desc.xml.
	FriendlyName string
	// Manufacturer / ModelName populate the UPnP device description; defaults
	// to constants.ConfDefaultBrand / constants.ConfDefaultModel respectively.
	Manufacturer string
	// ModelName see Manufacturer.
	ModelName string
	// UUID is the stable UUID embedded in `<UDN>` and the SSDP `USN` headers.
	// The host (Phase 3 orchestrator) is expected to derive a stable UUID
	// per receiver instance.
	UUID string
	// BindInterface optionally pins which network interface SSDP advertises
	// on. Empty means auto-detect by dialling the multicast group.
	BindInterface string
	// AdvertisePeriod is how often the NOTIFY ssdp:alive packets are
	// re-broadcast. Defaults to DefaultAdvertisePeriod.
	AdvertisePeriod time.Duration
	// Prefix is prepended to every HTTP path so endpoints become
	// `<prefix>/apps/YouTube`. Defaults to DefaultPrefix.
	Prefix string
	// AppName is the registered DIAL application name (the segment between
	// `/apps/` and `/<pid>`). Defaults to DefaultAppName.
	AppName string
	// AllowStop mirrors upstream `app.allowStop`. Upstream YouTubeApp sets it
	// to false; we keep the default at false so DELETE returns 405.
	AllowStop bool
	// MaxBodyBytes caps launch POST body size. Defaults to DefaultMaxBodyBytes.
	MaxBodyBytes int64
	// Logger receives lifecycle and error events. Defaults to a stderr-bound
	// DefaultLogger when nil.
	Logger logger.Logger
}

// Server is the DIAL server. Construct with NewServer; lifecycle via
// Start/Stop. Status reporting and callback registration are safe from any
// goroutine.
//
// Maps to upstream `class DialServer` — the public surface (Start, Stop,
// status, callback registration) lines up one-for-one with the TS class,
// while peer-dial's `Delegate.launchApp / stopApp / getApp` callbacks are
// folded into Server.OnLaunch / OnStop / SetState because we serve only one
// app ("YouTube") in this port and embedding the delegate dance buys nothing.
type Server struct {
	opts Options
	log  logger.Logger

	httpSrv *http.Server
	ssdp    *ssdpResponder

	// state, pid, status are atomic so HTTP handlers (running on
	// arbitrary net/http goroutines) can read them without synchronising
	// against Start/Stop.
	state  atomic.Value // AppState
	pid    atomic.Value // string
	status atomic.Value // constants.Status

	mu       sync.Mutex
	onLaunch LaunchHandler
	onStop   StopHandler

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewServer constructs a Server with sensible defaults filled in for any
// zero-valued option. The Server is not started until Start is called.
//
// Maps to the constructor body of upstream DialServer — option defaulting
// (`options.port || 3000`, `options.prefix || '/ytcr'`, etc.) and initial
// status assignment (`STATUSES.STOPPED`).
func NewServer(opts Options) *Server {
	if opts.Port == 0 {
		opts.Port = DefaultPort
	}
	if opts.Prefix == "" {
		opts.Prefix = DefaultPrefix
	} else if !strings.HasPrefix(opts.Prefix, "/") {
		opts.Prefix = "/" + opts.Prefix
	}
	opts.Prefix = strings.TrimRight(opts.Prefix, "/")
	if opts.AppName == "" {
		opts.AppName = DefaultAppName
	}
	if opts.AdvertisePeriod <= 0 {
		opts.AdvertisePeriod = DefaultAdvertisePeriod
	}
	if opts.Manufacturer == "" {
		opts.Manufacturer = constants.ConfDefaultBrand
	}
	if opts.ModelName == "" {
		opts.ModelName = constants.ConfDefaultModel
	}
	if opts.MaxBodyBytes <= 0 {
		opts.MaxBodyBytes = DefaultMaxBodyBytes
	}
	if opts.Logger == nil {
		opts.Logger = logger.NewDefaultLogger(false)
	}

	s := &Server{opts: opts, log: opts.Logger}
	s.state.Store(AppStateStopped)
	s.pid.Store("")
	s.status.Store(constants.StatusStopped)
	return s
}

// OnLaunch registers (or replaces) the launch callback. Calling with nil
// clears the handler — POST will respond as if the launch succeeded with no
// pid (matching upstream's "callback('')" branch).
func (s *Server) OnLaunch(fn LaunchHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onLaunch = fn
}

// OnStop registers (or replaces) the stop callback. Only invoked when
// Options.AllowStop is true.
func (s *Server) OnStop(fn StopHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onStop = fn
}

// SetState updates the AppState reported by GET /apps/<name>. Hosts call
// this when the YouTube app starts/stops in the orchestrator.
func (s *Server) SetState(state AppState) {
	switch state {
	case AppStateStopped, AppStateStarting, AppStateRunning:
		s.state.Store(state)
	default:
		s.log.Warn("[dial] SetState ignored — unknown state:", string(state))
	}
}

// SetPID updates the pid embedded in the LOCATION header of POST responses
// and in the DELETE URL surfaced via `<options/>` in the app descriptor.
// Phase 3 orchestrator calls this once it has assigned a session id.
func (s *Server) SetPID(pid string) {
	s.pid.Store(pid)
}

// State returns the current AppState (helper for tests / introspection).
func (s *Server) State() AppState {
	v, _ := s.state.Load().(AppState)
	return v
}

// PID returns the current pid string.
func (s *Server) PID() string {
	v, _ := s.pid.Load().(string)
	return v
}

// Status mirrors upstream `DialServer.status` lifecycle. Values are
// constants.Status* enumerations.
func (s *Server) Status() constants.Status {
	v, _ := s.status.Load().(constants.Status)
	return v
}

// Start launches both the HTTP server and the SSDP responder. It returns
// once the listeners are bound; long-running serve loops run on background
// goroutines that exit when Stop is called or `ctx` is cancelled.
//
// Maps to upstream `DialServer.start()` — option-validation, listen, status
// transition (STOPPED → STARTING → RUNNING), error wrapping in
// DialServerError on failure.
func (s *Server) Start(ctx context.Context) error {
	if s.Status() != constants.StatusStopped {
		s.log.Warn("[dial] start called but server not in STOPPED state")
		return nil
	}
	s.status.Store(constants.StatusStarting)

	httpLn, err := net.Listen("tcp", fmt.Sprintf(":%d", s.opts.Port))
	if err != nil {
		s.status.Store(constants.StatusStopped)
		return yterrors.NewDialServerError("failed to bind HTTP listener", err)
	}

	advertiseIP, err := resolveAdvertiseIP(s.opts.BindInterface)
	if err != nil {
		_ = httpLn.Close()
		s.status.Store(constants.StatusStopped)
		return yterrors.NewDialServerError("could not resolve advertise IP", err)
	}

	location := fmt.Sprintf("http://%s:%d%s/ssdp/device-desc.xml",
		advertiseIP.String(), s.opts.Port, s.opts.Prefix)

	mux := s.newMux()
	s.httpSrv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	s.wg.Go(func() {
		if err := s.httpSrv.Serve(httpLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log.Error("[dial] HTTP serve error:", err)
		}
	})

	ssdp, err := newSSDPResponder(ssdpConfig{
		UUID:            s.opts.UUID,
		Location:        location,
		AdvertiseIP:     advertiseIP,
		AdvertisePeriod: s.opts.AdvertisePeriod,
		Logger:          s.log,
	})
	if err != nil {
		_ = s.httpSrv.Close()
		cancel()
		s.wg.Wait()
		s.status.Store(constants.StatusStopped)
		return yterrors.NewDialServerError("failed to start SSDP responder", err)
	}
	if err := ssdp.Start(runCtx); err != nil {
		_ = s.httpSrv.Close()
		cancel()
		s.wg.Wait()
		s.status.Store(constants.StatusStopped)
		return yterrors.NewDialServerError("failed to start SSDP responder", err)
	}
	s.ssdp = ssdp

	s.status.Store(constants.StatusRunning)
	s.log.Info(fmt.Sprintf("[dial] DIAL server listening on port %d (location=%s)",
		s.opts.Port, location))
	return nil
}

// Stop gracefully shuts down the HTTP server and SSDP responder. Sends
// `ssdp:byebye` advertisements before exiting. Safe to call multiple times.
//
// Maps to upstream `DialServer.stop()` — status transition (RUNNING →
// STOPPING → STOPPED), DialServerError wrapping on failure.
func (s *Server) Stop() error {
	if s.Status() != constants.StatusRunning {
		s.log.Warn("[dial] stop called but server not in RUNNING state")
		return nil
	}
	s.status.Store(constants.StatusStopping)

	var firstErr error
	if s.ssdp != nil {
		if err := s.ssdp.Stop(); err != nil {
			firstErr = err
		}
	}
	if s.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := s.httpSrv.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
		cancel()
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	s.status.Store(constants.StatusStopped)

	if firstErr != nil {
		return yterrors.NewDialServerError("failed to stop DIAL server cleanly", firstErr)
	}
	return nil
}

// launchHandler / stopHandler return the currently-registered callbacks
// under the mutex, so HTTP handlers can call them without holding `s.mu`
// for the duration of the user code.
func (s *Server) launchHandler() LaunchHandler {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.onLaunch
}

func (s *Server) stopHandler() StopHandler {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.onStop
}
