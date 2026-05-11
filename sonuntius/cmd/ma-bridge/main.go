// Command ma-bridge is the Music Assistant bridge daemon for the
// Sonuntius addon. It opens the IPC broker on a Unix domain socket and
// translates events from the Cast/DIAL receivers into HA REST calls
// against the configured Music Assistant player entity.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/shobuprime/sonuntius/internal/config"
	"github.com/shobuprime/sonuntius/internal/dispatcher"
	"github.com/shobuprime/sonuntius/internal/ha"
	"github.com/shobuprime/sonuntius/internal/health"
	"github.com/shobuprime/sonuntius/internal/ipc"
	"github.com/shobuprime/sonuntius/internal/ma"
	"github.com/shobuprime/sonuntius/internal/state"
)

func main() {
	// Initial logger at info; reconfigured once options are read.
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	opts, err := config.Load(ctx, logger)
	if err != nil {
		logger.Error("config load failed", "err", err)
		os.Exit(2)
	}

	level := config.ResolveLogLevel(opts.LogLevel)
	logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	haToken := opts.HARESTToken()
	if haToken == "" {
		logger.Error("HA token unavailable — SUPERVISOR_TOKEN unset and no ha_token override configured")
		os.Exit(2)
	}
	if opts.HAToken != "" {
		logger.Info("ha: using user-supplied long-lived token override")
	}
	if opts.HABaseURL != "" {
		logger.Info("ha: base URL overridden", "url", opts.HABaseURL)
	}

	if err := opts.Validate(); err != nil {
		logger.Warn("config: required field missing — dispatcher will idle until corrected", "err", err)
	} else {
		logger.Info("config: ma_player_id configured", "entity_id", opts.MAPlayerID)
	}

	haCli := ha.NewClientWithBaseURL(opts.HARESTBaseURL(), haToken, logger.With("component", "ha"))

	// Music Assistant addon discovery feeds the Phase 6a direct MA WS
	// path. The discovered hostname is only used when the explicit
	// ma_ws_url override is empty. Log every outcome so the cause of a
	// silent fallback to HA core WS is visible in the addon log.
	maHost, discoveryErr := haCli.FindMAAddonHostname(ctx)
	switch {
	case discoveryErr != nil:
		logger.Warn("ha: MA addon discovery errored — falling back to HA core WS",
			"err", discoveryErr)
	case maHost != "":
		logger.Info("music_assistant addon detected", "hostname", maHost)
	default:
		logger.Info("ha: MA addon not discovered — falling back to HA core WS (set ma_ws_url to override)")
	}

	// Health endpoint — exposes /health JSON for the HA addon watchdog
	// (plan §6 Phase 6). Hosted by ma-bridge since it's always present
	// regardless of which receivers are enabled.
	healthSrv := health.NewServer(health.DefaultAddr, logger.With("component", "health"))
	if err := healthSrv.Start(ctx); err != nil {
		// Non-fatal — port collision should not crash the addon.
		logger.Warn("health: server failed to start", "err", err)
	}
	healthSrv.Set("config", true, summarizeConfig(opts))
	if opts.MAPlayerID == "" {
		healthSrv.Set("dispatcher", false, "ma_player_id is unset — events will be dropped")
	} else {
		healthSrv.Set("dispatcher", true, "entity="+opts.MAPlayerID)
	}

	disp := dispatcher.New(haCli, opts.MAPlayerID, logger.With("component", "dispatcher"))
	srv := ipc.NewServer(ipc.SocketPath(), logger.With("component", "ipc"))
	srv.Handler = disp.Dispatch

	if err := srv.Start(ctx); err != nil {
		logger.Error("ipc start failed", "err", err)
		healthSrv.Set("ipc", false, "start failed: "+err.Error())
		os.Exit(3)
	}
	healthSrv.Set("ipc", true, "listening on "+srv.Path)

	// Player state subscription: try MA direct WS first (plan §10 Path B,
	// richer state) and fall back to the HA core WS (Path A) when MA is
	// unreachable. Either watcher self-reconnects, so a flaky endpoint
	// does not disrupt the REST dispatch path.
	maURL := opts.MAWsURL
	if maURL == "" && maHost != "" {
		maURL = ma.URLFromHost(maHost)
	}
	if maURL != "" {
		probe := ma.NewWatcher(maURL, opts.MAToken, opts.MAPlayerID, srv, logger.With("component", "ma"))
		probeCtx, cancel := context.WithTimeout(ctx, 7*time.Second)
		err := probe.TryConnect(probeCtx)
		cancel()
		if err == nil {
			logger.Info("state: using direct MA WebSocket", "url", maURL)
			healthSrv.Set("state", true, "direct MA WebSocket: "+maURL)
			go probe.Run(ctx)
			goto running
		}
		logger.Warn("state: MA direct WS unreachable — falling back to HA core WS",
			"url", maURL, "err", err)
	}
	{
		watcher := state.NewWithURL(opts.HAWebSocketURL(), haToken, opts.MAPlayerID, srv, logger.With("component", "state"))
		healthSrv.Set("state", true, "HA core WebSocket: "+opts.HAWebSocketURL())
		go watcher.Run(ctx)
	}
running:

	logger.Info("ma-bridge online", "socket", srv.Path, "log_level", level.String())

	<-ctx.Done()
	logger.Info("ma-bridge shutting down")
}

// summarizeConfig returns a concise one-line summary of the loaded
// options for the /health endpoint detail field. Tokens are never
// included.
func summarizeConfig(o config.Options) string {
	parts := []string{"log_level=" + o.LogLevel}
	if o.HABaseURL != "" {
		parts = append(parts, "ha_base_url=set")
	}
	if o.HAToken != "" {
		parts = append(parts, "ha_token=user")
	} else {
		parts = append(parts, "ha_token=supervisor")
	}
	if o.MAWsURL != "" {
		parts = append(parts, "ma_ws_url=set")
	}
	if o.MAToken != "" {
		parts = append(parts, "ma_token=set")
	}
	if o.TidalFallback.Enabled {
		parts = append(parts, "tidal_fallback=enabled")
	}
	return strings.Join(parts, ", ")
}
