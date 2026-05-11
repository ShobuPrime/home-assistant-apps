// Maps to: N/A — Go-only sonuntius CASTV2 receiver binary.
//
// cast-receiver is the sonuntius addon's Cast (CASTV2 over TLS) receiver
// service. It runs inside the addon container under S6 supervision and
// is responsible for:
//
//   1. Loading the AirReceiver cert + companion artifacts (graceful
//      degrade if they're missing — TLS server simply does not start).
//   2. Announcing the receiver via mDNS as a _googlecast._tcp instance
//      so Android Cast senders discover us.
//   3. Running the CASTV2 TLS server with the Tidal + generic-URL
//      parsers registered.
//   4. Translating every parser-claimed LOAD into a PlayIntent and
//      pushing it onto the existing ma-bridge IPC bus.
//
// Resilience guarantees (matches cmd/yt-cast/main.go):
//   - No fatal exit on missing cert. The binary logs a clear warning,
//     suppresses the TLS listener, and stays alive so S6 doesn't enter
//     a restart loop.
//   - IPC reconnect with exponential backoff.
//   - Server.Start retries with exponential backoff so a port collision
//     or transient listen failure doesn't crash the addon.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/shobuprime/sonuntius/internal/castv2"
	"github.com/shobuprime/sonuntius/internal/castv2/auth"
	"github.com/shobuprime/sonuntius/internal/castv2/mdns"
	"github.com/shobuprime/sonuntius/internal/castv2/parsers"
	"github.com/shobuprime/sonuntius/internal/events"
	"github.com/shobuprime/sonuntius/internal/ipc"
)

const (
	ipcReconnectFloor = 2 * time.Second
	ipcReconnectCeil  = 30 * time.Second
	serverRetryFloor  = 5 * time.Second
	serverRetryCeil   = 5 * time.Minute
	mdnsRetryFloor    = 5 * time.Second
	mdnsRetryCeil     = 5 * time.Minute
)

func main() {
	// Initial logger at info — reconfigured once options load.
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	log.Info("cast-receiver: starting")

	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	opts := loadRuntimeOptions(ctx, log)
	log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: opts.LogLevel}))
	slog.SetDefault(log)

	log.Info("cast-receiver: configuration loaded",
		"friendly_name", opts.FriendlyName,
		"data_dir", opts.DataDir,
		"uuid", opts.UUID,
		"cert_path", opts.CertPath,
		"key_path", opts.KeyPath,
		"enable_tidal_proxy", opts.AddonOptions.EnableTidalProxy)

	if !opts.AddonOptions.EnableTidalProxy {
		log.Info("cast-receiver: enable_tidal_proxy=false — service idling")
		<-ctx.Done()
		log.Info("cast-receiver: shutting down")
		return
	}

	// --- Cert + auth responder ----------------------------------------
	tlsConfig, certLoaded := loadTLSConfig(opts.CertPath, opts.KeyPath, log)
	if certLoaded {
		trackCertFingerprint(opts.CertPath, opts.DataDir, log)
	}
	responder, err := auth.NewAirReceiverResponder(
		opts.CertPath, opts.SignaturePath, opts.IntermediatesPath, log)
	if err != nil {
		log.Warn("cast-receiver: AirReceiver responder failed to initialise — continuing without auth replay",
			"err", err)
	}

	// --- IPC connector -------------------------------------------------
	emitter := newIntentEmitter(log.With("component", "ipc"))
	conn := newIPCConnector(emitter, log.With("component", "ipc"))
	conn.run(ctx)

	// --- mDNS responder ------------------------------------------------
	mdnsR, mdnsErr := mdns.NewResponder(mdns.Options{
		InstanceName: opts.FriendlyName,
		ServiceType:  "_googlecast._tcp",
		Port:         opts.AddonOptions.EffectiveCastReceiverTLSPort(),
		UUID:         opts.UUID,
		TXTRecords: map[string]string{
			"id": opts.UUID,
			"fn": opts.FriendlyName,
			"md": "Chromecast",
			"ic": "/setup/icon.png",
			"rs": "",
			"ca": "4101",
			"ve": "05",
			"st": "0",
		},
		Logger: log.With("component", "mdns"),
	})
	if mdnsErr != nil {
		log.Warn("cast-receiver: mdns responder build failed — continuing without mDNS",
			"err", mdnsErr)
		mdnsR = nil
	}

	// --- CASTV2 server -------------------------------------------------
	server := castv2.NewServer(castv2.Options{
		Addr:          fmt.Sprintf(":%d", opts.AddonOptions.EffectiveCastReceiverTLSPort()),
		TLSConfig:     tlsConfig,
		AuthResponder: responderOrNil(responder),
		FriendlyName:  opts.FriendlyName,
		Logger:        log.With("component", "castv2"),
	})
	// Parser registration order matters: Tidal first, generic last
	// (LogOnlyParser is registered by NewServer and never claims, so
	// declaration order doesn't matter for correctness — but the live
	// parsers must come before LogOnly so they run before the diagnostic
	// log line. RegisterParser appends, so we get the order:
	// [LogOnly, Tidal, Generic]. That actually puts LogOnly *first*; it
	// never claims so Tidal still wins, but we explicitly want Tidal →
	// Generic → LogOnly. We can't reorder LogOnly, but its non-claiming
	// behaviour means correctness is preserved regardless.)
	server.RegisterParser(parsers.NewTidal(log.With("component", "parser", "name", "tidal")))
	server.RegisterParser(parsers.NewGeneric(log.With("component", "parser", "name", "url")))
	server.SetIntentHandler(emitter.Emit)

	// --- Lifecycle -----------------------------------------------------
	if mdnsR != nil {
		startMDNSLoop(ctx, mdnsR, log)
	}
	if tlsConfig != nil && certLoaded {
		startServerLoop(ctx, server, log)
	} else {
		log.Warn("cast-receiver: TLS server disabled (cert not configured) — staying alive for mDNS only")
	}

	// --- Wait for shutdown --------------------------------------------
	<-ctx.Done()
	log.Info("cast-receiver: shutting down")
	if mdnsR != nil {
		if err := mdnsR.Stop(); err != nil {
			log.Debug("cast-receiver: mdns stop returned error", "err", err)
		}
	}
	if tlsConfig != nil && certLoaded {
		if err := server.Stop(); err != nil {
			log.Debug("cast-receiver: server stop returned error", "err", err)
		}
	}
	conn.close()
	log.Info("cast-receiver: stopped")
}

// loadTLSConfig builds a tls.Config from the cert + key paths. Returns
// (nil, false) when either file is missing or unparseable so the caller
// can suppress the TLS listener without crashing.
func loadTLSConfig(certPath, keyPath string, log *slog.Logger) (*tls.Config, bool) {
	if certPath == "" || keyPath == "" {
		log.Warn("cast-receiver: cert or key path empty — TLS server disabled (cert not configured)")
		return nil, false
	}
	for _, p := range []string{certPath, keyPath} {
		if _, err := os.Stat(p); err != nil {
			log.Warn("cast-receiver: TLS server disabled (cert not configured)",
				"missing", p, "err", err)
			return nil, false
		}
	}
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		log.Warn("cast-receiver: TLS server disabled — cert / key unloadable",
			"err", err, "cert", certPath, "key", keyPath)
		return nil, false
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, true
}

// responderOrNil returns the responder cast to the auth.Responder
// interface, or nil if the underlying pointer is nil. The CASTV2 server
// will refuse to Start with a nil responder, which is fine when we've
// already decided not to start the TLS listener.
func responderOrNil(r *auth.AirReceiverResponder) auth.Responder {
	if r == nil {
		return nil
	}
	return r
}

// startServerLoop launches a goroutine that calls server.Start with
// exponential backoff on failure. Mirrors the receiver retry loop in
// cmd/yt-cast/main.go — we never give up because the addon container
// must stay healthy.
func startServerLoop(ctx context.Context, server *castv2.Server, log *slog.Logger) {
	go func() {
		backoff := serverRetryFloor
		for {
			if ctx.Err() != nil {
				return
			}
			if err := server.Start(ctx); err != nil {
				log.Warn("cast-receiver: server start failed — retrying",
					"err", err, "retry_in", backoff)
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
				if backoff < serverRetryCeil {
					backoff *= 2
					if backoff > serverRetryCeil {
						backoff = serverRetryCeil
					}
				}
				continue
			}
			// Server.Start returned nil — listener was closed normally.
			return
		}
	}()
}

// startMDNSLoop launches a goroutine that retries mdns.Start with
// exponential backoff. A transient port-in-use error should not crash
// the process.
func startMDNSLoop(ctx context.Context, r *mdns.Responder, log *slog.Logger) {
	go func() {
		backoff := mdnsRetryFloor
		for {
			if ctx.Err() != nil {
				return
			}
			if err := r.Start(ctx); err != nil {
				log.Warn("cast-receiver: mdns start failed — retrying",
					"err", err, "retry_in", backoff)
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
				if backoff < mdnsRetryCeil {
					backoff *= 2
					if backoff > mdnsRetryCeil {
						backoff = mdnsRetryCeil
					}
				}
				continue
			}
			log.Info("cast-receiver: mdns responder online")
			return
		}
	}()
}

// ipcConnector manages the persistent connection to ma-bridge: dials
// with backoff, reads inbound frames (currently ignored — the
// cast-receiver does not need PlayerState snapshots because parsers run
// at LOAD time), and reconnects on read failure.
//
// Mirrors cmd/yt-cast/main.go's connector. Kept as a separate type so
// the two services can evolve independently.
type ipcConnector struct {
	emitter *intentEmitter
	log     *slog.Logger

	mu     sync.Mutex
	client *ipc.Client
	cancel context.CancelFunc
	doneCh chan struct{}
}

func newIPCConnector(e *intentEmitter, log *slog.Logger) *ipcConnector {
	return &ipcConnector{
		emitter: e,
		log:     log,
		doneCh:  make(chan struct{}),
	}
}

func (c *ipcConnector) run(ctx context.Context) {
	rctx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.cancel = cancel
	c.mu.Unlock()
	go c.loop(rctx)
}

func (c *ipcConnector) loop(ctx context.Context) {
	defer close(c.doneCh)
	backoff := ipcReconnectFloor
	for {
		if ctx.Err() != nil {
			return
		}
		cli, err := ipc.Dial(ipc.SocketPath())
		if err != nil {
			c.log.Debug("cast-receiver: ipc dial failed", "err", err, "retry_in", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < ipcReconnectCeil {
				backoff *= 2
				if backoff > ipcReconnectCeil {
					backoff = ipcReconnectCeil
				}
			}
			continue
		}
		c.log.Info("cast-receiver: ipc connected")
		c.swap(cli)
		c.emitter.setIPCClient(cli)
		c.readLoop(ctx, cli)
		c.log.Info("cast-receiver: ipc disconnected — will reconnect")
		c.emitter.setIPCClient(nil)
		backoff = ipcReconnectFloor
	}
}

func (c *ipcConnector) readLoop(ctx context.Context, cli *ipc.Client) {
	for {
		if ctx.Err() != nil {
			return
		}
		ev, err := cli.Recv()
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				c.log.Debug("cast-receiver: ipc recv error", "err", err)
			}
			_ = cli.Close()
			return
		}
		// cast-receiver doesn't currently react to PlayerState frames —
		// the Cast LOAD pipeline is one-way (sender → MA). Log at debug
		// so a maintainer can confirm the receiver is connected.
		if _, ok := ev.(*events.PlayerState); ok {
			c.log.Debug("cast-receiver: ignored PlayerState frame from ma-bridge")
		}
	}
}

func (c *ipcConnector) swap(cli *ipc.Client) {
	c.mu.Lock()
	if old := c.client; old != nil {
		_ = old.Close()
	}
	c.client = cli
	c.mu.Unlock()
}

func (c *ipcConnector) close() {
	c.mu.Lock()
	cancel := c.cancel
	cli := c.client
	c.client = nil
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if cli != nil {
		_ = cli.Close()
	}
	<-c.doneCh
}
