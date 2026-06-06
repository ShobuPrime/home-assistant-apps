// Command aegis_ha is the AegisHA add-on daemon.
//
// It loads add-on options, opens the hashed PIN store, runs the alarm
// state machine, and — when a Supervisor-managed MQTT broker is available
// — publishes a native Home Assistant alarm_control_panel entity (plus
// companion entities) via MQTT discovery and bridges keypad commands to
// the state machine. The ingress HTTP server (health now; keypad/admin UI
// in a later phase) runs alongside. Everything shuts down on SIGTERM.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/shobuprime/aegis_ha/internal/alarm"
	"github.com/shobuprime/aegis_ha/internal/card"
	"github.com/shobuprime/aegis_ha/internal/config"
	"github.com/shobuprime/aegis_ha/internal/ha"
	"github.com/shobuprime/aegis_ha/internal/mqtt"
	"github.com/shobuprime/aegis_ha/internal/store"
	"github.com/shobuprime/aegis_ha/internal/unifi"
	"github.com/shobuprime/aegis_ha/internal/web"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "0.1.0"

const (
	ingressAddr = ":8099"
	dataDir     = "/data/aegis_ha"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := newLogger("info")

	opts, err := config.Load(ctx)
	if err != nil {
		logger.Error("aegis_ha: failed to load options", "err", err)
		os.Exit(1)
	}
	logger = newLogger(opts.LogLevel)
	logger.Info("aegis_ha: starting",
		"version", version,
		"protect_mode", opts.ProtectMode,
		"arm_modes", strings.Join(opts.ArmModes, ","),
		"mqtt_topic_prefix", opts.MQTTTopicPrefix,
		"exit_delay", opts.ExitDelay,
		"entry_delay", opts.EntryDelay,
		"trigger_time", opts.TriggerTime,
		"web_ui", opts.EnableWebUI,
		"bootstrap_users", len(opts.Users),
		"unifi_configured", opts.UniFiHost != "" && opts.UniFiAPIKey != "",
	)

	token := os.Getenv("SUPERVISOR_TOKEN")

	// PIN / user store.
	st, err := store.Open(dataDir, store.Policy{
		LockoutThreshold: opts.LockoutThreshold,
		LockoutDuration:  time.Duration(opts.LockoutDuration) * time.Second,
		PINMin:           opts.PINMinLength,
		PINMax:           opts.PINMaxLength,
	})
	if err != nil {
		logger.Error("aegis_ha: failed to open store", "err", err)
		os.Exit(1)
	}
	if n, err := st.ImportBootstrap(toBootstrap(opts.Users), opts.DefaultRole); err != nil {
		logger.Warn("store: bootstrap import failed", "err", err)
	} else if n > 0 {
		logger.Warn("store: imported bootstrap users — now clear the plaintext PINs from the add-on options", "count", n)
	}

	// Optional companion Lovelace card: write it to /config/www and
	// auto-register it as a Lovelace resource over the Supervisor Core-WS
	// (storage mode); on YAML-mode dashboards, log the manual snippet.
	if opts.EnableCompanionCard {
		if url := card.Deploy(version, logger); url != "" && token != "" {
			if err := ha.RegisterLovelaceResource(token, url, logger); err != nil {
				logger.Warn("card: auto-registration failed — add it manually as a JavaScript Module resource (storage-mode Lovelace auto-registers)",
					"url", url, "err", err)
			}
		}
	}

	// Alarm state machine.
	alarmCfg := alarm.Config{
		ExitDelay:           seconds(opts.ExitDelay),
		EntryDelay:          seconds(opts.EntryDelay),
		TriggerTime:         seconds(opts.TriggerTime),
		ArmModes:            opts.ArmModes,
		DisarmAfterTrigger:  opts.DisarmAfterTrigger,
		RestoreAfterTrigger: opts.IgnoreBlockingAfterTrigger,
	}
	alarmCfg.RestoreArmMode = readAlarmState(dataDir, logger)
	alarmCfg.OnCommit = func(armMode string) { writeAlarmState(dataDir, armMode) }
	engine := alarm.New(alarmCfg, logger)
	go engine.Run(ctx)

	// Native MQTT alarm entity (optional — disabled cleanly when no broker).
	client, bridge, prefix := setupMQTT(ctx, logger, opts, engine, st, token, alarmCfg)

	// Fire HA bus events (aegis_ha_command_success, aegis_ha_failed_to_arm, …) so
	// automations get Alarmo-style hooks alongside the MQTT entities.
	if bridge != nil && token != "" {
		haClient := ha.New(token, logger)
		bridge.SetEventSink(func(eventType string, data map[string]any) {
			go func() {
				cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				if err := haClient.FireEvent(cctx, eventType, data); err != nil {
					logger.Debug("ha: fire event failed", "event", eventType, "err", err)
				}
			}()
		})
	}

	// UniFi Protect manager (optional — only when a host + API key are set).
	if opts.UniFiHost != "" && opts.UniFiAPIKey != "" {
		uclient := unifi.New(opts.UniFiHost, opts.UniFiAPIKey, opts.UniFiVerifySSL, logger)
		var pub unifi.Publisher = noopPublisher{}
		if bridge != nil {
			pub = bridge
		}
		mgr := unifi.NewManager(uclient, engine, pub, unifi.Config{
			PreferMode:      opts.ProtectMode,
			PollInterval:    10 * time.Second,
			SensorOverrides: sensorOverrides(opts.Sensors),
			Groups:          sensorGroups(opts.SensorGroups),
			ExposeZones:     opts.ExposeZoneEntities,
		}, logger)
		go mgr.Run(ctx)
		logger.Info("unifi: protect manager started", "host", opts.UniFiHost, "mode_pref", opts.ProtectMode)
	}

	// Ingress HTTP server: health endpoints + the keypad/admin UI. Blocks
	// until SIGTERM.
	srv := web.New(logger, ingressAddr, web.Options{
		Engine:             engine,
		Store:              st,
		AdminUsernames:     opts.AdminUsernames,
		ArmModes:           opts.ArmModes,
		ArmingRequiresCode: opts.ArmingRequiresCode,
		DisarmRequiresCode: opts.DisarmRequiresCode,
		EnableUI:           opts.EnableWebUI,
		Version:            version,
	})
	runErr := srv.Run(ctx)

	// Best-effort graceful MQTT offline (the LWT also covers ungraceful exits).
	if client != nil {
		_ = client.Publish(prefix+"/status", []byte("offline"), true)
		client.Disconnect()
	}

	if runErr != nil {
		logger.Error("aegis_ha: web server failed", "err", runErr)
		os.Exit(1)
	}
	logger.Info("aegis_ha: stopped cleanly")
}

// setupMQTT discovers the broker and, if present, starts the client +
// bridge. Returns the client (nil if MQTT is disabled) and the topic
// prefix for the graceful-offline publish.
func setupMQTT(ctx context.Context, logger *slog.Logger, opts *config.Options, engine *alarm.Engine, st *store.Store, token string, alarmCfg alarm.Config) (*mqtt.Client, *mqtt.Bridge, string) {
	broker, ok, err := mqtt.DiscoverBroker(ctx, token)
	if err != nil {
		logger.Warn("mqtt: broker discovery failed — native alarm entity disabled", "err", err)
		return nil, nil, opts.MQTTTopicPrefix
	}
	if !ok {
		logger.Warn("mqtt: no broker available — native alarm entity disabled (add the MQTT integration + a broker)")
		return nil, nil, opts.MQTTTopicPrefix
	}
	var tlsCfg *tls.Config
	if broker.SSL {
		tlsCfg = &tls.Config{InsecureSkipVerify: true} // internal Supervisor broker, often self-signed
	}
	client := mqtt.New(mqtt.Options{
		Broker:    broker.Addr(),
		ClientID:  "aegis_ha",
		Username:  broker.Username,
		Password:  broker.Password,
		KeepAlive: 30 * time.Second,
		TLS:       tlsCfg,
		Logger:    logger,
	})
	bridge := mqtt.NewBridge(client, engine, st, mqtt.Config{
		Prefix:              opts.MQTTTopicPrefix,
		CodeFormat:          opts.MQTTCodeFormat,
		ArmModes:            opts.ArmModes,
		ArmingRequiresCode:  opts.ArmingRequiresCode,
		DisarmRequiresCode:  opts.DisarmRequiresCode,
		TriggerRequiresCode: opts.TriggerRequiresCode,
		Version:             version,
	}, alarmCfg, logger)
	go client.Run(ctx)
	go bridge.Run(ctx)
	logger.Info("mqtt: native alarm entity enabled", "broker", broker.Addr(), "prefix", opts.MQTTTopicPrefix)
	return client, bridge, opts.MQTTTopicPrefix
}

// noopPublisher satisfies unifi.Publisher when MQTT is unavailable, so the
// UniFi manager can still drive local-mirror arm/disarm and breach
// detection without publishing entities.
type noopPublisher struct{}

func (noopPublisher) EnableProtect()                          {}
func (noopPublisher) AnnounceZone(string, string)             {}
func (noopPublisher) PublishProtectStatus(string, bool)       {}
func (noopPublisher) PublishZone(string, bool)                {}

func toBootstrap(users []config.User) []store.Bootstrap {
	out := make([]store.Bootstrap, 0, len(users))
	for _, u := range users {
		out = append(out, store.Bootstrap{
			Name:            u.Name,
			HAUserID:        u.HAUserID,
			PIN:             u.PIN,
			Role:            u.Role,
			AllowedArmModes: u.AllowedArmModes,
		})
	}
	return out
}

func seconds(n int) time.Duration { return time.Duration(n) * time.Second }

// sensorOverrides converts the options sensor list into the alarm engine's
// per-sensor config, keyed by lowercased name for matching UniFi sensors.
func sensorOverrides(list []config.SensorOverride) map[string]alarm.SensorConfig {
	if len(list) == 0 {
		return nil
	}
	out := make(map[string]alarm.SensorConfig, len(list))
	for _, o := range list {
		if o.Name == "" {
			continue
		}
		out[strings.ToLower(o.Name)] = alarm.SensorConfig{
			Name:               o.Name,
			Modes:              o.Modes,
			AlwaysOn:           o.AlwaysOn,
			Immediate:          o.Immediate,
			UseExitDelay:       o.UseExitDelay,
			AutoBypass:         o.AutoBypass,
			AllowOpen:          o.AllowOpen,
			TriggerUnavailable: o.TriggerUnavailable,
			Group:              o.Group,
		}
	}
	return out
}

func sensorGroups(list []config.SensorGroupCfg) []alarm.SensorGroup {
	out := make([]alarm.SensorGroup, 0, len(list))
	for _, g := range list {
		if g.Name == "" {
			continue
		}
		out = append(out, alarm.SensorGroup{
			Name:       g.Name,
			EventCount: g.EventCount,
			Timeout:    time.Duration(g.Timeout) * time.Second,
		})
	}
	return out
}

type alarmStateFile struct {
	ArmMode string `json:"arm_mode"`
}

// readAlarmState returns the committed arm mode persisted before the last
// shutdown (empty if none), so an armed system survives an add-on restart.
func readAlarmState(dir string, logger *slog.Logger) string {
	b, err := os.ReadFile(filepath.Join(dir, "alarm_state.json"))
	if err != nil {
		return ""
	}
	var s alarmStateFile
	if json.Unmarshal(b, &s) != nil {
		return ""
	}
	if s.ArmMode != "" {
		logger.Info("alarm: restoring committed armed state across restart", "mode", s.ArmMode)
	}
	return s.ArmMode
}

func writeAlarmState(dir, armMode string) {
	b, err := json.Marshal(alarmStateFile{ArmMode: armMode})
	if err != nil {
		return
	}
	tmp := filepath.Join(dir, "alarm_state.json.tmp")
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, filepath.Join(dir, "alarm_state.json"))
	}
}

// newLogger maps the add-on log_level option onto an slog text handler.
func newLogger(level string) *slog.Logger {
	var lv slog.Level
	switch strings.ToLower(level) {
	case "trace", "debug":
		lv = slog.LevelDebug
	case "warning", "warn":
		lv = slog.LevelWarn
	case "error", "fatal":
		lv = slog.LevelError
	default: // info, notice, ""
		lv = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lv}))
}
