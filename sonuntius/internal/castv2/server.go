// Maps to: N/A — Go-only TLS server orchestrating framed CASTV2 I/O and
//          dispatching messages to per-namespace handlers.
//
// Lifecycle mirrors internal/ytcast/dial/server.go: NewServer assembles
// the config and namespace handlers, Start binds the TLS listener and
// spawns the accept goroutine, Stop closes the listener and waits for
// per-connection goroutines to drain.
//
// One goroutine per accepted sender. Each goroutine owns its tls.Conn and
// reads framed CastMessages in a loop. The connection-namespace handler
// gates the per-sender state; the auth-namespace handler responds to the
// initial AuthChallenge before any other namespace processing happens.
package castv2

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/shobuprime/sonuntius/internal/castv2/auth"
	"github.com/shobuprime/sonuntius/internal/castv2/namespaces"
)

// DefaultAddr is the standard CASTV2 listen port.
const DefaultAddr = ":8009"

// Options configure a Server. Zero-valued fields fall back to documented
// defaults.
type Options struct {
	// Addr is the TCP listen address. Defaults to ":8009".
	Addr string

	// TLSConfig is the server-side TLS configuration. The CASTV2 protocol
	// runs over TLS even though our receiver is a "fake". If nil, the
	// server refuses to Start — the cmd binary in Phase 3b validates the
	// cert+key paths before constructing the Options.
	TLSConfig *tls.Config

	// AuthResponder builds the AuthResponse for the device-auth namespace.
	// Required — if nil, the server's auth handler will close any sender
	// that sends an AuthChallenge.
	AuthResponder auth.Responder

	// FriendlyName is surfaced as the Cast receiver's display name.
	// Defaults to "Sonuntius".
	FriendlyName string

	// Logger receives lifecycle events and per-connection diagnostics.
	// Defaults to slog.Default().
	Logger *slog.Logger

	// ReceiverSourceID is the source-id the server uses when originating
	// frames toward senders. Defaults to "receiver-0".
	ReceiverSourceID string

	// ReadTimeout caps how long a single ReadFrame may block before the
	// per-conn goroutine reasserts ctx and the heartbeat-staleness check.
	// Defaults to 35s (just over the heartbeat PongTimeout).
	ReadTimeout time.Duration

	// HandshakeTimeout caps how long tls.Handshake may take. Defaults to
	// 10s; senders on flaky networks sometimes need a bit of slack.
	HandshakeTimeout time.Duration
}

// Server is a CASTV2 TLS receiver. Construct with NewServer; lifecycle
// via Start (blocks until ctx cancels) and Stop.
type Server struct {
	opts Options
	log  *slog.Logger

	// Namespace handlers.
	conns     *namespaces.Connection
	heartbeat *namespaces.Heartbeat
	receiver  *namespaces.Receiver
	media     *namespaces.Media

	listener net.Listener

	mu        sync.Mutex
	startedAt time.Time
	wg        sync.WaitGroup
	cancel    context.CancelFunc
	stopOnce  sync.Once
}

// NewServer constructs a Server with sensible defaults filled in for any
// zero-valued option.
func NewServer(opts Options) *Server {
	if opts.Addr == "" {
		opts.Addr = DefaultAddr
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.ReceiverSourceID == "" {
		opts.ReceiverSourceID = PlatformReceiverID
	}
	if opts.FriendlyName == "" {
		opts.FriendlyName = "Sonuntius"
	}
	if opts.ReadTimeout == 0 {
		opts.ReadTimeout = 35 * time.Second
	}
	if opts.HandshakeTimeout == 0 {
		opts.HandshakeTimeout = 10 * time.Second
	}
	s := &Server{
		opts:      opts,
		log:       opts.Logger,
		conns:     namespaces.NewConnection(opts.Logger),
		heartbeat: namespaces.NewHeartbeat(opts.ReceiverSourceID, opts.Logger),
		receiver:  namespaces.NewReceiver(opts.FriendlyName, opts.Logger),
		media:     namespaces.NewMedia(nil, nil, opts.Logger),
	}
	// Always register the LogOnly parser so Phase 3a deploys produce
	// useful diagnostics in real environments. Phase 3b adds concrete
	// parsers that run before LogOnly (parsers run in registration order
	// but LogOnly never claims, so order doesn't matter for correctness).
	s.media.RegisterParser(namespaces.NewLogOnlyParser(opts.Logger))
	return s
}

// RegisterParser exposes the media-namespace parser chain to callers so
// Phase 3b can plug in its Tidal and generic-URL parsers.
func (s *Server) RegisterParser(p namespaces.Parser) {
	s.media.RegisterParser(p)
}

// SetIntentHandler wires the callback the media handler invokes whenever a
// parser claims a LOAD payload. Phase 3b uses this to push the parsed
// Intent onto the IPC bus.
func (s *Server) SetIntentHandler(fn func(ctx context.Context, source string, intent Intent)) {
	if fn == nil {
		s.media.SetIntentHandler(nil)
		return
	}
	s.media.SetIntentHandler(func(ctx context.Context, source string, pi namespaces.ParsedIntent) {
		fn(ctx, source, Intent{
			Provider: pi.Provider,
			TrackID:  pi.TrackID,
			URL:      pi.URL,
			Metadata: pi.Metadata,
		})
	})
}

// SetState lets the host nudge the receiver-namespace into a particular
// state. Currently only "idle" is meaningful (clears the launched app);
// any other value is interpreted as "launched with that string as the
// appId" for the synthetic RECEIVER_STATUS payload.
func (s *Server) SetState(state string) {
	switch state {
	case "", "idle", "IDLE":
		// Force back to idle. We do this by calling Handle with a STOP
		// payload — keeps the state-mutation logic in one place.
		_, _ = s.receiver.Handle(context.Background(), s.opts.ReceiverSourceID,
			json.RawMessage(`{"type":"STOP"}`))
	default:
		_, _ = s.receiver.Handle(context.Background(), s.opts.ReceiverSourceID,
			json.RawMessage(fmt.Sprintf(`{"type":"LAUNCH","appId":%q}`, state)))
	}
}

// Start binds the TLS listener and runs the accept loop until ctx is
// cancelled. Returns the first non-nil error from the listener; callers
// should treat any error other than net.ErrClosed as fatal.
func (s *Server) Start(ctx context.Context) error {
	if s.opts.TLSConfig == nil {
		return errors.New("castv2: Start called with nil TLSConfig")
	}
	if s.opts.AuthResponder == nil {
		return errors.New("castv2: Start called with nil AuthResponder")
	}

	ln, err := tls.Listen("tcp", s.opts.Addr, s.opts.TLSConfig)
	if err != nil {
		return fmt.Errorf("castv2: listen %s: %w", s.opts.Addr, err)
	}
	s.mu.Lock()
	s.listener = ln
	s.startedAt = time.Now()
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.mu.Unlock()

	s.log.Info("castv2: TLS receiver listening", "addr", s.opts.Addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || runCtx.Err() != nil {
				s.log.Info("castv2: accept loop exiting")
				return nil
			}
			s.log.Warn("castv2: accept error", "err", err)
			// Brief back-off to avoid a busy loop if the listener is in a
			// degraded state.
			select {
			case <-runCtx.Done():
				return nil
			case <-time.After(100 * time.Millisecond):
			}
			continue
		}
		s.wg.Add(1)
		go s.handleConn(runCtx, conn)
	}
}

// Stop closes the listener and waits for any in-flight per-connection
// goroutines to finish. Safe to call multiple times.
func (s *Server) Stop() error {
	var err error
	s.stopOnce.Do(func() {
		s.mu.Lock()
		if s.cancel != nil {
			s.cancel()
		}
		ln := s.listener
		s.mu.Unlock()
		if ln != nil {
			err = ln.Close()
		}
		s.wg.Wait()
	})
	return err
}

// handleConn is the per-connection goroutine. It performs the TLS
// handshake, then enters the read loop until ctx cancels or the peer
// disconnects.
func (s *Server) handleConn(ctx context.Context, raw net.Conn) {
	defer s.wg.Done()
	defer raw.Close()

	tlsConn, ok := raw.(*tls.Conn)
	if !ok {
		s.log.Warn("castv2: non-TLS connection on TLS listener", "remote", raw.RemoteAddr())
		return
	}
	_ = tlsConn.SetDeadline(time.Now().Add(s.opts.HandshakeTimeout))
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		s.log.Debug("castv2: TLS handshake failed", "remote", raw.RemoteAddr(), "err", err)
		return
	}
	// Clear the handshake deadline; the read loop sets its own.
	_ = tlsConn.SetDeadline(time.Time{})

	remote := tlsConn.RemoteAddr().String()
	s.log.Info("castv2: sender connected", "remote", remote)
	defer s.log.Info("castv2: sender disconnected", "remote", remote)

	// Per-connection lock around writes (multiple goroutines may originate
	// frames: the read loop's reply, the heartbeat ticker's PING). One
	// mutex is simpler than a write channel and safer than parallel writes.
	var writeMu sync.Mutex
	write := func(msg *CastMessage) {
		writeMu.Lock()
		defer writeMu.Unlock()
		if err := WriteFrame(tlsConn, msg); err != nil {
			s.log.Debug("castv2: write failed", "remote", remote, "err", err)
		}
	}

	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Heartbeat ticker — emits PING to every observed sender source-id.
	// We do not start it until the sender has emitted at least one
	// connection-namespace CONNECT, because the source-id is otherwise
	// unknown.
	tickerStarted := false
	startHeartbeat := func(source string) {
		if tickerStarted {
			return
		}
		tickerStarted = true
		go s.heartbeatLoop(connCtx, source, write)
	}

	for {
		if err := tlsConn.SetReadDeadline(time.Now().Add(s.opts.ReadTimeout)); err != nil {
			s.log.Debug("castv2: SetReadDeadline failed", "err", err)
			return
		}
		cm, err := ReadFrame(tlsConn)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return
			}
			var nerr net.Error
			if errors.As(err, &nerr) && nerr.Timeout() {
				if connCtx.Err() != nil {
					return
				}
				continue
			}
			s.log.Debug("castv2: read failed", "remote", remote, "err", err)
			return
		}

		msg := FromCastMessage(cm)
		s.log.Debug("castv2: rx",
			"remote", remote,
			"namespace", msg.Namespace,
			"source", msg.SourceID,
			"dest", msg.DestinationID,
			"binary", msg.IsBinary())

		// Auth namespace runs before connection-namespace gating: senders
		// send AuthChallenge as their first frame, before any CONNECT.
		if msg.Namespace == NamespaceDeviceAuth {
			s.handleDeviceAuth(msg, write)
			continue
		}

		// Connection-namespace traffic is handled by the connection handler
		// directly; it also implicitly opens the source if it sees something
		// other than CONNECT.
		if msg.Namespace == NamespaceConnection {
			_, _ = s.conns.Handle(connCtx, msg.SourceID, msg.PayloadJSON)
			// Once the sender has handshaked, start the heartbeat ticker.
			if s.conns.IsOpen(msg.SourceID) {
				startHeartbeat(msg.SourceID)
			}
			continue
		}

		// Other namespaces require an open virtual connection. Be lenient:
		// if the sender skipped CONNECT (some debug tools do), open it
		// implicitly and proceed.
		if !s.conns.IsOpen(msg.SourceID) {
			s.log.Debug("castv2: implicit connection open", "source", msg.SourceID)
			s.conns.Open(msg.SourceID)
			startHeartbeat(msg.SourceID)
		}

		handler := s.handlerFor(msg.Namespace)
		if handler == nil {
			s.log.Debug("castv2: no handler for namespace", "namespace", msg.Namespace)
			continue
		}
		reply, err := handler.Handle(connCtx, msg.SourceID, msg.PayloadJSON)
		if err != nil {
			s.log.Warn("castv2: handler error",
				"namespace", msg.Namespace, "err", err)
			continue
		}
		if reply.IsEmpty() {
			continue
		}
		out := s.buildReply(msg, reply)
		write(out)
	}
}

// handlerFor selects the namespace handler for ns. Returns nil for unknown
// namespaces; the caller logs and skips.
func (s *Server) handlerFor(ns string) namespaces.Handler {
	switch ns {
	case NamespaceConnection:
		return s.conns
	case NamespaceHeartbeat:
		return s.heartbeat
	case NamespaceReceiver:
		return s.receiver
	case NamespaceMedia:
		return s.media
	default:
		return nil
	}
}

// buildReply renders a wire-level CastMessage from an inbound message +
// the handler's Reply. Defaults flow as documented on Reply.
func (s *Server) buildReply(in *Message, reply namespaces.Reply) *CastMessage {
	ns := reply.Namespace
	if ns == "" {
		ns = in.Namespace
	}
	dest := reply.DestinationID
	if dest == "" {
		dest = in.SourceID
	}
	source := in.DestinationID
	if source == "" || source == "*" {
		source = s.opts.ReceiverSourceID
	}
	return &CastMessage{
		ProtocolVersion: ProtocolVersionCastV21,
		SourceID:        source,
		DestinationID:   dest,
		Namespace:       ns,
		PayloadType:     PayloadTypeString,
		PayloadUTF8:     string(reply.Payload),
	}
}

// handleDeviceAuth builds and writes an AuthResponse using the configured
// Responder. The challenge bytes are forwarded (the AirReceiver responder
// currently ignores them per shanocast — but a future Responder
// implementation could inspect them).
func (s *Server) handleDeviceAuth(in *Message, write func(*CastMessage)) {
	resp, err := s.opts.AuthResponder.BuildResponse(in.PayloadBinary)
	if err != nil {
		s.log.Warn("castv2: auth responder failed", "err", err)
		return
	}
	source := in.DestinationID
	if source == "" || source == "*" {
		source = s.opts.ReceiverSourceID
	}
	out := &CastMessage{
		ProtocolVersion: ProtocolVersionCastV21,
		SourceID:        source,
		DestinationID:   in.SourceID,
		Namespace:       NamespaceDeviceAuth,
		PayloadType:     PayloadTypeBinary,
		PayloadBinary:   resp,
	}
	write(out)
	s.log.Debug("castv2: auth response sent", "bytes", len(resp))
}

// heartbeatLoop emits a PING to source every PingInterval until ctx is
// cancelled. The per-connection goroutine starts this once it has seen a
// CONNECT for the given source.
func (s *Server) heartbeatLoop(ctx context.Context, source string, write func(*CastMessage)) {
	t := time.NewTicker(namespaces.PingInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			ping := s.heartbeat.EmitPing(source)
			out := &CastMessage{
				ProtocolVersion: ProtocolVersionCastV21,
				SourceID:        s.opts.ReceiverSourceID,
				DestinationID:   ping.DestinationID,
				Namespace:       NamespaceHeartbeat,
				PayloadType:     PayloadTypeString,
				PayloadUTF8:     string(ping.Payload),
			}
			write(out)
		}
	}
}
