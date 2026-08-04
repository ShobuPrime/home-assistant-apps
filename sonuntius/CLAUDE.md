# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Home Assistant App that bridges Cast / DIAL senders (YouTube, YouTube Music, Tidal) onto a Music Assistant–controlled Sendspin player. The app does **not** relay audio in proxy mode — it extracts the playback intent (track id, transport command, volume) from the sender's protocol and asks Music Assistant to play the corresponding content via its native YouTube Music / Tidal provider integrations. An opt-in Tidal Connect binary fallback path is documented for the cases where the Cast proxy cannot extract Tidal track IDs.

The implementation is staged across six phases. The phase plan and rationale are summarized in the **Phase status** section below; for a deeper architectural overview see the `Architecture and Key Components` section further down and the `# Architecture` block in `DOCS.md`.

### Phase status

- **Phase 0** — Scaffolding (S6 services, app manifest, smoke test). Done.
- **Phase 1** — Music Assistant bridge skeleton: HA REST client, IPC broker, dispatcher, HA WS state subscription. Done.
- **Phase 2** — YouTube / YouTube Music via DIAL + Lounge: full Go 1.26 port of `yt-cast-receiver` v2.1.1 (commit `83d61fa169e33c5e0046c2440b99a17cd9493e73`) under `internal/ytcast/`, plus a sonuntius binary at `cmd/yt-cast/` that emits `PlayIntent` events into the existing IPC broker. Done.
- **Phase 3** — Tidal proxy via CASTV2 + AirReceiver auth replay. Done.
  - 3a (in tree): CASTV2 protocol stack — framing, namespace handlers,
    AirReceiver responder, mDNS responder, and the orchestrating Server.
  - 3b (in tree): `parsers.NewTidal` extracts the track ID from LOAD
    customData (probing the four historically-observed JSON paths plus
    a metadata-subtitle heuristic), and the `cast-receiver` Go binary
    wires the parser chain, mDNS responder, CASTV2 server, and the IPC
    emitter together. The service stays alive when the user-supplied
    AirReceiver cert is absent — TLS listener is suppressed, mDNS still
    announces, no S6 restart loop.
- **Phase 4** — Default Media Receiver fallback for generic Cast audio URLs. Done.
  - `parsers.NewGeneric` claims LOADs whose `contentType` starts with
    `audio/` (or whose contentType is empty but contentId is an http(s)
    URL) and emits a `url`-provider intent. Registered after the Tidal
    parser in the cast-receiver chain.
- **Phase 5** — Opt-in iFi Tidal Connect binary fallback. Done.
  - `cont-init.d/20-tidal-fallback.sh` extracts the user-supplied iFi
    tarball from `/share/sonuntius/`, locates the binary and cert, and
    drops a marker file the per-service run scripts gate on. Always
    exits 0 — bad inputs log clearly and leave the services idle.
  - `tidal-connect` S6 service execs the iFi binary against
    `hw:Loopback,0,0`.
  - `alsa-to-sendspin` Go binary execs `arecord` against the loopback
    capture side and forwards PCM frames to the Sendspin server
    WebSocket. The Sendspin frame envelope is the only place wire
    format lives (`encodeFrame`) — currently raw-PCM-through, marked as
    a Phase 2.1 TODO pending the public Sendspin spec.
  - Disabled by default. Enable via `tidal_fallback.enabled = true` in
    app options, plus drop the iFi tarball at the configured path.
- **Phase 6** — Polish. Done.
  - 6a (Direct MA WS): `internal/ma/` opens `ws://<host>:8095/ws`,
    handles the schema-aware auth handshake, subscribes to MA player
    events, and broadcasts `PlayerState` frames. ma-bridge probes
    MA-direct first and falls back to the HA core WS path on failure.
  - 6b (Health + persistence): ma-bridge hosts a JSON health endpoint at
    `http://127.0.0.1:8099/health` summarizing every component
    (config, dispatcher, ipc, state). cast-receiver records the loaded
    AirReceiver cert's SHA-256 fingerprint and logs cert rotations.
  - 6 prerequisite (URL+token overrides): four new config fields
    (`ha_base_url`, `ha_token`, `ma_ws_url`, `ma_token`) — all optional,
    defaults preserve Supervisor-proxy behavior. Plumbed end-to-end.

## Essential Commands

### Building and Testing
```bash
# Build the app image locally (auto-detects architecture)
./build.sh

# Run the full smoke test (mock Supervisor + container + IPC round-trip)
bash ../.github/scripts/smoke-test.sh sonuntius local/amd64-addon-local_sonuntius:0.1.0

# Build just the Go binaries (no container, useful for fast iteration)
go vet ./...
go build ./cmd/ma-bridge
go build ./cmd/sonuntius-ctl

# Push a one-off event into a running bridge (from inside the container)
sonuntius-ctl play --provider ytmusic --track-id <id>
sonuntius-ctl transport --command pause
sonuntius-ctl volume --level 0.4
```

### Version Management

This app has no upstream binary to track — the app IS the software. Version bumps are manual. There is no update script or automated update workflow.

## Architecture and Key Components

### How It Works

1. **Init script** (`cont-init.d/10-prepare.sh`): Creates `/data/sonuntius/` and `/run/sonuntius/`, validates the `ma-bridge` binary is present.
2. **ma-bridge daemon** (`services.d/ma-bridge/run` → `/usr/local/bin/ma-bridge`):
   - Loads app options from `/data/options.json` (real HA) or the Supervisor REST API (smoke test).
   - Opens the JSON-line UDS broker at `/run/sonuntius/events.sock`.
   - Subscribes to HA WebSocket `state_changed` events for the configured `media_player.*` entity and broadcasts derived `PlayerState` frames to IPC clients.
   - Translates incoming `PlayIntent` / `TransportCommand` / `VolumeCommand` events into HA `media_player.*` REST calls.
3. **cast-receiver / yt-cast** (`services.d/cast-receiver/run`, `services.d/yt-cast/run`): Phase 0 sleep-infinity stubs that become functional in Phases 2 and 3.

### Language and Dependencies

- **Go 1.26** for all application services. Single static binary per service (`CGO_ENABLED=0`), no glibc / musl dependency.
- Stdlib only, with the single exception of `golang.org/x/net/websocket` (Go team's blessed WebSocket package) for the HA core WS subscription. No third-party libraries — the Phase 2 yt-cast port replaced the upstream's Node-only deps (`peer-dial`, `express`, `node-persist`, `youtubei.js`, etc.) with stdlib equivalents or, where unavoidable, a documented stub.
- Dockerfile is a multi-stage build: `golang:1.26-alpine` builds the binaries (`ma-bridge`, `sonuntius-ctl`, `yt-cast`), then the hassio-addons base copies them in.

### Port-tracking convention (yt-cast)

The `internal/ytcast/` tree is a maintained port of an external project. Two invariants:

1. **Every `.go` file under `internal/ytcast/` opens with a `// Maps to:` header** naming the upstream source file (e.g. `// Maps to: src/lib/app/Session.ts`). Multi-file ports use ` + ` between paths. Go-only support files use `// Maps to: N/A — Go-only [description]`.
2. **The pinned upstream commit lives in two places that must stay in sync**: `internal/ytcast/constants/upstream.go` (Go const) and `internal/ytcast/UPSTREAM.md` (port table + workflow). When updating the upstream pin, bump both and walk every `// Maps to:` header for drift.

### Directory Structure

- **`/cmd/ma-bridge/`**: Bridge daemon entry point (one binary).
- **`/cmd/sonuntius-ctl/`**: CLI for sending one-shot events to the bridge (used by the smoke test and development).
- **`/cmd/yt-cast/`**: YouTube / YT Music DIAL + Lounge receiver service. Wraps the `internal/ytcast` library with a sonuntius-specific Player adapter that emits IPC events.
- **`/cmd/cast-receiver/`**: Tidal proxy + Default Media Receiver fallback service. Wraps the `internal/castv2` package — mDNS responder + TLS Server + Tidal/Generic parsers — and translates parser-claimed LOAD intents into `PlayIntent` events on the IPC bus.
- **`/cmd/alsa-to-sendspin/`**: Phase 5 opt-in fallback forwarder. Execs `arecord` against the ALSA loopback capture side (where the iFi `tidal_connect_application` writes its decoded Tidal audio) and pushes PCM frames over WebSocket to the configured Sendspin server. The `encodeFrame` function is the wire-format hook — currently raw-PCM-through pending the public Sendspin spec.
- **`/internal/config/`**: App options loader (file → Supervisor REST fallback).
- **`/internal/events/`**: Wire types for the IPC protocol (`PlayIntent`, `TransportCommand`, `VolumeCommand`, `PlayerState`) with self-describing JSON via a `type` discriminator.
- **`/internal/ipc/`**: JSON-line UDS broker — server (in ma-bridge) and client (in receivers and `sonuntius-ctl`).
- **`/internal/ha/`**: HA REST client routed through the Supervisor proxy (`http://supervisor/core/api`).
- **`/internal/state/`**: HA core WebSocket state watcher; subscribes via `subscribe_trigger` and broadcasts `PlayerState`. Fallback for the direct MA WS path.
- **`/internal/ma/`**: Music Assistant direct WebSocket client. Preferred state-subscription path when the MA app is reachable. Handles the schema-aware auth handshake and translates MA `player_updated` events to `PlayerState` frames.
- **`/internal/health/`**: Loopback HTTP health endpoint (127.0.0.1:8099) hosted by ma-bridge. Aggregates per-component status as JSON for HA's app watchdog.
- **`/internal/dispatcher/`**: Routes IPC events to HA service calls; owns the `(provider, track_id) → media_content_id` URI translation.
- **`/internal/ytcast/`**: Go 1.26 port of [`yt-cast-receiver`](https://github.com/patrickkfkan/yt-cast-receiver) v2.1.1. Subpackages: `logger`, `datastore`, `asyncq`, `yterrors`, `constants`, `types`, `player`, `dial` (stdlib SSDP + UPnP + DIAL HTTP), `lounge` (RPCConnection, Message, BindParams, Session, Playlist, PairingCode). Top-level files `youtubeapp.go`, `receiver.go`, `engine.go` are the orchestrator. Upstream commit pinned in `constants/upstream.go` and `UPSTREAM.md`. Every file opens with a `// Maps to:` header naming its upstream source.
- **`/internal/castv2/`**: Stdlib CASTV2 protocol stack. Top-level files implement the wire framing (`castmessage.go`, `framing.go`), the orchestrating `Server`, the `Intent` / `Message` shared types, and a package doc. Subpackages: `auth` (AirReceiver responder), `namespaces` (connection, heartbeat, receiver, media handlers + the `Parser` interface + `LogOnlyParser`), `mdns` (RFC 6762 + RFC 6763 responder), `parsers` (Phase 3b/4 — `NewTidal`, `NewGeneric`). No upstream pin file; the package implements a protocol spec rather than tracking a specific upstream codebase.
- **`/rootfs/etc/cont-init.d/`**: S6 init scripts.
- **`/rootfs/etc/services.d/{cast-receiver,yt-cast,ma-bridge}/`**: S6 service definitions.

### Critical Files

- **`config.yaml`**: App configuration. `host_network: true` is mandatory — Cast (mDNS) and DIAL (SSDP) are L2 broadcast and don't traverse Docker bridge networking.
- **`translations/en.yaml`**: Plain-English config-option names + descriptions shown in the HA Configuration tab (kept in sync with `config.yaml`'s options).
- **`build.yaml`**: hassio-addons base image per architecture (auto-bumped by the repo workflow).
- **`Dockerfile`**: Multi-stage Go builder + hassio-addons base. `ARG BUILD_FROM` has no inline default (per repo policy).
- **`go.mod`**: Module path `github.com/shobuprime/sonuntius`, Go 1.26.
- **`apparmor.txt`**: Blanket `file,` rule. No specific paths because the receivers and bridge open arbitrary sockets and read user-supplied certs under `/share/sonuntius/`.

### Architecture Support

- `aarch64` (HA Yellow, RPi 4/5)
- `amd64`

armhf / armv7 / i386 are not supported — hassio-addons base v19+ dropped those architectures.

### Port and Network Layout

- **`host_network: true`** is required for mDNS (`_googlecast._tcp.local`, port 5353/udp) and SSDP (port 1900/udp) advertisements. Without it, Android Cast/DIAL senders cannot discover the app.
- **8009/tcp** — CASTV2 receiver (Phase 3+; bound by `cast-receiver`).
- **8008/tcp** — DIAL HTTP endpoint (Phase 2+; bound by `yt-cast`).
- **`/run/sonuntius/events.sock`** — UDS IPC broker (internal to the app).

## Development Guidelines

### S6-Overlay Integration

- All S6 scripts use the `#!/usr/bin/with-contenv bashio` shebang.
- The `run` script uses `exec /usr/local/bin/ma-bridge` so the Go binary becomes PID 1 inside its S6 service and receives SIGTERM cleanly.
- The bridge listens for SIGINT / SIGTERM via `signal.NotifyContext` and shuts down the IPC server, dispatcher, and WS watcher in order.

### Configuration Handling

- App options are read once at startup. The Go bridge does **not** call bashio — it reads `/data/options.json` directly (or the Supervisor REST API at `/addons/self/options/config` for smoke-test environments).
- `ma_player_id` is the only required option; the dispatcher logs and drops events when it's empty rather than crashing.

### IPC Protocol

- Bidirectional JSON-line over UDS. One event per line.
- Each event marshals to a JSON object with a `type` discriminator field equal to the Go type name (`PlayIntent`, `TransportCommand`, `VolumeCommand`, `PlayerState`).
- The ma-bridge process owns the server; cast-receiver and yt-cast attach as clients (Phases 2–3).
- Misbehaving clients (slow writes, broken pipes) are dropped immediately so they cannot wedge the server.

### Music Assistant Integration

- `play_media` is always issued via HA REST (`POST /core/api/services/media_player/play_media`) because that path works whether or not the MA WebSocket is reachable.
- Provider URI templates live in `internal/dispatcher/dispatcher.go`. As of Phase 1: `ytmusic://track/<id>` and `tidal://track/<id>`. Confirm against current MA stable when wiring Phases 2 and 3.
- State subscription uses HA's core WebSocket (`ws://supervisor/core/websocket`) with `subscribe_trigger` for the configured entity. MA's direct WS (`ws://<ma-addon>:8095/ws`) is reserved for a future enhancement.

### Cert and Binary Provisioning (Do NOT bake into image)

- The AirReceiver certificate (Phase 3 Tidal proxy) is **never** shipped in the image. Users drop it at `/share/sonuntius/airreceiver_cert.pem` and `/share/sonuntius/airreceiver_key.pem`.
- The iFi Tidal Connect binary (Phase 5 fallback) is **never** shipped in the image. Users provide it as a tarball at `/share/sonuntius/ifi-tidal-release.tar.gz`. See `DOCS.md` for provenance disclosure.

### Version Updates

When updating version:
1. Update `version` in `config.yaml`.
2. Add an entry to `CHANGELOG.md`.
3. Update the version reference in `README.md` (if any).

### Testing Checklist

- `go vet ./...` is clean.
- `go build ./cmd/ma-bridge` and `go build ./cmd/sonuntius-ctl` succeed.
- `./build.sh` produces an image successfully.
- Smoke test passes: `bash ../.github/scripts/smoke-test.sh sonuntius <image>`.
- `sonuntius-ctl play --provider ytmusic --track-id <id>` exits 0 inside a running container.
- The IPC socket appears at `/run/sonuntius/events.sock` after startup.

## Important Notes

- **`host_network: true`** is mandatory and cannot be replaced with port mappings; mDNS/SSDP are L2 broadcast.
- **Single language**: all application code is Go 1.26+. Resist the temptation to introduce other languages without a documented reason — the YouTube `yt-cast-receiver` library in Node.js is the most likely future exception (Phase 2 decision).
- **Stdlib-only policy**: third-party imports require justification. `golang.org/x/net/websocket` is the only current exception and lives in the Go project's own namespace.
- **No Claude Code attribution lines** in commit messages (per repo CLAUDE.md).
- **Signed commits required** (per repo CLAUDE.md).
- **`ARG BUILD_FROM` has no inline default** — version comes from `build.yaml` at build time (per repo CLAUDE.md).
- **`apk upgrade --no-cache` before `apk add`** in the Dockerfile to resolve libcrypto3/libssl3 conflicts (per repo CLAUDE.md).
- **Architecture support**: `aarch64` and `amd64` only.

## Common Issues and Troubleshooting

### Issue: ma-bridge logs `SUPERVISOR_TOKEN unset` and exits

**Cause:** The app is being run outside HA without the Supervisor token injected.

**Solution:** When running locally, set `SUPERVISOR_TOKEN` manually (the smoke-test harness does this). In real HA, the token is auto-injected because `hassio_api: true` and `homeassistant_api: true` are set in `config.yaml`.

### Issue: Dispatcher logs `dispatcher idle (ma_player_id unset)` and drops events

**Cause:** The `ma_player_id` option is empty in the app configuration.

**Solution:** Set `ma_player_id` in the app UI to the Music Assistant player entity ID (e.g. `media_player.sendspin_living_room`) and restart the app.

### Issue: HA WebSocket state subscription keeps disconnecting

**Cause:** Network instability, HA restart, or the configured entity does not exist.

**Solution:** The watcher reconnects with exponential backoff (2 s → 60 s). Check the logs for the rejection reason; if it's `subscribe_trigger rejected`, verify the entity exists via the HA developer tools.

### Issue: Phone does not see the app as a Cast / DIAL target

**Cause:** Phase 2 / 3 receivers are not yet implemented (only the bridge is online as of Phase 1), or `host_network: true` is disabled.

**Solution:** Confirm the phase is reached and verify `host_network: true` in `config.yaml`. Use `avahi-browse -art` on the host to confirm `_googlecast._tcp.local.` advertisement.

### Issue: Smoke test reports `play_media call not logged — dispatcher may be idle`

**Cause:** Expected in the mock-supervisor environment; the smoke test does not set `ma_player_id`, so the dispatcher drops the test event with a warning. Not a failure.
