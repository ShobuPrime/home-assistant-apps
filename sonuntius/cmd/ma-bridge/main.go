// Command ma-bridge is the Music Assistant bridge daemon for the
// Sonuntius addon. It opens the IPC broker on a Unix domain socket and
// translates events from the Cast/DIAL receivers into HA REST calls
// against the configured Music Assistant player entity.
package main

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/shobuprime/sonuntius/internal/events"
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
	disp.Start(ctx)

	// Construct the IPC server up-front so the MA WS event handler
	// (declared below) can reference it. Start() is deferred until
	// after the MA WS path is wired so the dispatcher's queue_id is
	// known by the time a sender's first PlayIntent is processed.
	srv := ipc.NewServer(ipc.SocketPath(), logger.With("component", "ipc"))
	srv.Handler = disp.Dispatch

	// Configure MA WS direct play_media path for url-provider intents.
	// MA's HA integration strips metadata when routing
	// media_player.play_media through HA's service registry, so for
	// rich-metadata casts (e.g. the YouTube watch URL we hand MA) we
	// prefer MA's native WS API with a fully-formed MediaItem. The
	// queue_id is MA's internal player_id, derived from the HA
	// entity_id by stripping `media_player.` and any trailing `_N`.
	// MA WS direct path. A single long-lived WebSocket replaces the
	// per-call short connections used before v0.2.0 and replaces HA
	// REST for transport / volume / seek (~30-40 ms saved per command,
	// and seek-after-play actually lands on the new queue item
	// instead of racing HA's 3-4 s media_seek round-trip). HA REST
	// remains the fallback when MA is unavailable.
	var maClient *ma.WSClient
	if opts.MAPlayerID != "" {
		maPlayURL := opts.MAWsURL
		if maPlayURL == "" && maHost != "" {
			maPlayURL = ma.URLFromHost(maHost)
		}
		if maPlayURL != "" {
			queueID := resolveMAQueueID(ctx, maPlayURL, opts, logger.With("component", "ma-discovery"))
			if queueID != "" {
				// MA WS event handler: when MA fires a player_updated
				// for OUR queue, decode it and broadcast a PlayerState
				// over the IPC bus so the yt-cast adapter sees pause /
				// resume / volume / position changes that originate
				// inside MA (e.g. user paused in MA's UI). Closes the
				// bidirectional sync loop that v0.2.0 left to HA core
				// WS — which doesn't fire reliably for MA-internal
				// state changes.
				logCtx := logger.With("component", "ma-ws-events")
				maEventHandler := func(eventName, objectID string, data json.RawMessage) {
					// queue_updated and queue_time_updated carry the
					// authoritative state (playing / paused / idle /
					// buffering) and elapsed_time. The Player object's
					// state is the speaker on/off mirror and can lag
					// queue transitions by one frame.
					//
					// player_updated still gives us volume/muted echo
					// and the speaker-level state — useful as a
					// fallback when the queue path is silent.
					var ps *events.PlayerState
					switch eventName {
					case "queue_updated", "queue_time_updated":
						if objectID != "" && objectID != queueID {
							return
						}
						ps = ma.PlayerStateFromQueueEvent(data)
					case "player_updated", "player_added":
						if objectID != "" && objectID != queueID {
							return
						}
						ps = ma.PlayerStateFromPlayerEvent(data)
					default:
						logCtx.Debug("ma ws event: ignoring", "event", eventName, "object_id", objectID)
						return
					}
					if ps == nil {
						return
					}
					ps.Source = "ma-ws"
					logCtx.Debug("ma ws event: broadcasting PlayerState",
						"event", eventName,
						"state", ps.State,
						"title", ps.Title,
						"track_id", ps.TrackID)
					srv.Broadcast(ps)
				}
				maClient = ma.NewWSClient(maPlayURL, opts.MAToken,
					logger.With("component", "ma-ws"), maEventHandler)
				maClient.Start(ctx)
				disp.SetMAWS(maClient, queueID)
				logger.Info("dispatcher: MA WS direct path enabled",
					"queue_id", queueID, "url", maPlayURL)
			} else {
				logger.Warn("dispatcher: MA WS direct path disabled — could not determine queue_id (set ma_queue_id explicitly)",
					"url", maPlayURL)
			}
		}
	}
	if err := srv.Start(ctx); err != nil {
		logger.Error("ipc start failed", "err", err)
		healthSrv.Set("ipc", false, "start failed: "+err.Error())
		os.Exit(3)
	}
	healthSrv.Set("ipc", true, "listening on "+srv.Path)

	// Player state subscription:
	//
	// HA core WS state subscription. ALWAYS started — it watches the
	// HA `media_player.*` entity for state_changed events and feeds
	// position / duration / title back to receivers via IPC. The
	// MA-direct WS (above, via the WSClient) handles outbound commands;
	// state still flows back via HA core because HA's MA integration
	// aggregates MA's player events into the entity state.
	watcher := state.NewWithURL(opts.HAWebSocketURL(), haToken, opts.MAPlayerID, srv, logger.With("component", "state"))
	healthSrv.Set("state", true, "HA core WebSocket: "+opts.HAWebSocketURL()+" (entity="+opts.MAPlayerID+")")
	go watcher.Run(ctx)

	logger.Info("ma-bridge online", "socket", srv.Path, "log_level", level.String())

	<-ctx.Done()
	logger.Info("ma-bridge shutting down")
	if maClient != nil {
		maClient.Stop()
	}
}

// resolveMAQueueID determines the MA-side queue_id (internal player_id)
// to use for player_queues/play_media. Precedence:
//
//  1. Explicit `ma_queue_id` from config — trust the user; do not probe.
//  2. Auto-discover via `players/all` and MatchPlayer on the configured
//     HA entity_id. Logs every visible player at info so the user can
//     copy the correct one into `ma_queue_id` if matching fails.
//  3. Conservative fallback to ma.DerivePlayerID(entity_id) — preserves
//     v0.1.10 behaviour if MA is unreachable for discovery. Returns ""
//     when discovery yields nothing and we don't want to send doomed
//     play_media commands to a non-existent queue.
func resolveMAQueueID(ctx context.Context, maURL string, opts config.Options, log *slog.Logger) string {
	if opts.MAQueueID != "" {
		log.Info("ma: queue_id from config override", "queue_id", opts.MAQueueID)
		return opts.MAQueueID
	}
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	players, err := ma.ListPlayers(probeCtx, maURL, opts.MAToken, log)
	if err != nil {
		log.Warn("ma: players/all failed — falling back to derived queue_id",
			"err", err, "fallback", ma.DerivePlayerID(opts.MAPlayerID))
		return ma.DerivePlayerID(opts.MAPlayerID)
	}
	for _, p := range players {
		log.Info("ma: known player",
			"player_id", p.PlayerID,
			"display_name", p.DisplayName,
			"name", p.Name,
			"provider", p.Provider,
			"available", p.Available,
			"type", p.Type)
	}
	match, rule := ma.MatchPlayer(players, opts.MAPlayerID)
	if rule != "" {
		log.Info("ma: queue_id resolved via discovery",
			"queue_id", match.PlayerID,
			"display_name", match.DisplayName,
			"match_rule", rule,
			"entity_id", opts.MAPlayerID)
		return match.PlayerID
	}
	log.Warn("ma: no MA player matches entity_id — set ma_queue_id explicitly from the list above",
		"entity_id", opts.MAPlayerID,
		"count", len(players))
	return ""
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
