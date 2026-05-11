// Maps to: N/A — Go-only sonuntius binary.
//
// yt-cast is the sonuntius addon's Cast/DIAL receiver service. It runs
// inside the addon container under S6 supervision; main() reads
// /data/options.json, dials the ma-bridge IPC socket, builds a
// PlayerEngine adapter that forwards PlayIntent/Transport/Volume
// events over IPC, and starts the Go port of yt-cast-receiver
// (internal/ytcast).
//
// Resilience: the service keeps the addon healthy even when its
// dependencies are not — a missing ma-bridge socket, a failed receiver
// start, or a network outage should not crash the process. Each
// reconnect runs through an exponential-backoff loop so a flaky
// network does not turn into a CPU-burning retry storm.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/shobuprime/sonuntius/internal/events"
	"github.com/shobuprime/sonuntius/internal/ipc"
	"github.com/shobuprime/sonuntius/internal/ytcast"
	"github.com/shobuprime/sonuntius/internal/ytcast/constants"
	"github.com/shobuprime/sonuntius/internal/ytcast/datastore"
	"github.com/shobuprime/sonuntius/internal/ytcast/logger"
)

const (
	ipcReconnectFloor = 2 * time.Second
	ipcReconnectCeil  = 30 * time.Second
	recvRetryFloor    = 5 * time.Second
	recvRetryCeil     = 5 * time.Minute
)

func main() {
	// Initial logger at info — reconfigured once options load.
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	log.Info(fmt.Sprintf("yt-cast: starting (yt-cast-receiver port @ %s)", upstreamShort()))

	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	opts := loadRuntimeOptions(ctx, log)
	log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: opts.LogLevel}))
	slog.SetDefault(log)

	log.Info("yt-cast: configuration loaded",
		"friendly_name", opts.FriendlyName,
		"data_dir", opts.DataDir,
		"uuid", opts.UUID,
		"ma_player_id", opts.AddonOptions.MAPlayerID,
		"enable_youtube", opts.AddonOptions.EnableYouTube,
		"upstream_commit", upstreamShort())

	if !opts.AddonOptions.EnableYouTube {
		log.Info("yt-cast: enable_youtube=false — service idling")
		<-ctx.Done()
		log.Info("yt-cast: shutting down")
		return
	}

	// --- Build the adapter and connect to the IPC broker. ----------
	adapt := newAdapter(nil)
	adapt.setLogger(log.With("component", "player"))
	adapt.setVolumeStep(opts.AddonOptions.EffectiveVolumeStep())
	log.Info("yt-cast: volume quantisation configured",
		"volume_step", opts.AddonOptions.EffectiveVolumeStep())
	conn := newIPCConnector(adapt, log.With("component", "ipc"))
	conn.run(ctx)

	// --- Build the receiver. The constructor doesn't touch the wire,
	//     so it's safe to do once even though Start may be retried. -
	rlogger := &slogAdapter{l: log.With("component", "ytcast"), level: opts.LogLevel}
	store := datastore.NewFileStore(opts.DataDir)
	receiver, err := ytcast.NewReceiver(ytcast.Options{
		Player: adapt,
		Device: ytcast.DeviceOptions{
			Name:  opts.FriendlyName,
			Brand: "ShobuPrime",
			Model: "Sonuntius",
		},
		Dial: ytcast.DialOptions{
			Port: opts.AddonOptions.EffectiveYTCastDialPort(),
			UUID: opts.UUID,
		},
		DataStore: store,
		Logger:    rlogger,
	})
	if err != nil {
		log.Error("yt-cast: failed to build receiver — running in idle mode", "err", err)
		<-ctx.Done()
		log.Info("yt-cast: shutting down")
		return
	}

	// Wire external state updates back into the engine's state event
	// bus. Whenever the IPC connector caches a fresh PlayerState (i.e.
	// Music Assistant reported a position / volume / duration change
	// over HA's WebSocket), the adapter calls this hook and the
	// engine re-emits its current state to the orchestrator, which
	// forwards onStateChange / onVolumeChanged / nowPlaying to the
	// connected sender's Lounge protocol. Without this hook, the
	// phone only sees state updates on engine-side transitions
	// (play / pause / stop) and external changes are invisible.
	adapt.setOnStateChange(func(ctx context.Context) {
		if err := receiver.EmitPlayerState(ctx); err != nil {
			log.Debug("yt-cast: EmitPlayerState failed (likely no active sender)", "err", err)
		}
	})

	// --- Start the receiver with retry-with-backoff. ---------------
	startReceiverLoop(ctx, receiver, log)

	// --- Wait for shutdown signal. ---------------------------------
	<-ctx.Done()
	log.Info("yt-cast: shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := receiver.Stop(shutdownCtx); err != nil {
		log.Warn("yt-cast: receiver stop returned error", "err", err)
	}
	conn.close()
	log.Info("yt-cast: stopped")
}

// startReceiverLoop launches a goroutine that calls receiver.Start
// with exponential backoff on failure. We don't exit on persistent
// failure because the addon container must stay healthy (S6 would
// restart us in a loop otherwise).
func startReceiverLoop(ctx context.Context, receiver *ytcast.Receiver, log *slog.Logger) {
	go func() {
		backoff := recvRetryFloor
		for {
			if ctx.Err() != nil {
				return
			}
			if err := receiver.Start(ctx); err != nil {
				log.Warn("yt-cast: receiver start failed — retrying", "err", err, "retry_in", backoff)
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
				if backoff < recvRetryCeil {
					backoff *= 2
					if backoff > recvRetryCeil {
						backoff = recvRetryCeil
					}
				}
				continue
			}
			log.Info("yt-cast: receiver online",
				"status", string(receiver.Status()),
				"upstream", constants.UpstreamCommit)
			return
		}
	}()
}

// ipcConnector manages the persistent connection to ma-bridge: dials
// with backoff, reads inbound PlayerState frames to refresh the
// adapter's cache, and reconnects on read failure.
type ipcConnector struct {
	adapter *adapter
	log     *slog.Logger

	mu     sync.Mutex
	client *ipc.Client
	cancel context.CancelFunc
	doneCh chan struct{}
}

func newIPCConnector(a *adapter, log *slog.Logger) *ipcConnector {
	return &ipcConnector{
		adapter: a,
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
			c.log.Debug("yt-cast: ipc dial failed", "err", err, "retry_in", backoff)
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
		c.log.Info("yt-cast: ipc connected")
		c.swap(cli)
		c.adapter.setIPCClient(cli)
		c.readLoop(ctx, cli)
		c.log.Info("yt-cast: ipc disconnected — will reconnect")
		c.adapter.setIPCClient(nil)
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
				c.log.Debug("yt-cast: ipc recv error", "err", err)
			}
			_ = cli.Close()
			return
		}
		if ps, ok := ev.(*events.PlayerState); ok {
			c.adapter.updateCachedState(*ps)
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

// slogAdapter implements logger.Logger over an *slog.Logger so the
// receiver uses the wrapper's structured logger.
type slogAdapter struct {
	mu    sync.Mutex
	l     *slog.Logger
	level slog.Level
}

func (a *slogAdapter) Error(msg ...any) { a.log(slog.LevelError, msg) }
func (a *slogAdapter) Warn(msg ...any)  { a.log(slog.LevelWarn, msg) }
func (a *slogAdapter) Info(msg ...any)  { a.log(slog.LevelInfo, msg) }
func (a *slogAdapter) Debug(msg ...any) { a.log(slog.LevelDebug, msg) }

func (a *slogAdapter) SetLevel(value logger.LogLevel) {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch value {
	case logger.LogLevelError:
		a.level = slog.LevelError
	case logger.LogLevelWarn:
		a.level = slog.LevelWarn
	case logger.LogLevelInfo:
		a.level = slog.LevelInfo
	case logger.LogLevelDebug:
		a.level = slog.LevelDebug
	case logger.LogLevelNone:
		a.level = slog.LevelError + 1
	}
}

func (a *slogAdapter) log(level slog.Level, msg []any) {
	a.mu.Lock()
	if level < a.level {
		a.mu.Unlock()
		return
	}
	l := a.l
	a.mu.Unlock()
	l.Log(context.Background(), level, fmt.Sprint(msg...))
}

// Compile-time assertion that slogAdapter satisfies logger.Logger.
var _ logger.Logger = (*slogAdapter)(nil)
