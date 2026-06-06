# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

AegisHA is a Go 1.26 Home Assistant add-on that implements a dynamic, Alarmo-
inspired alarm system and fronts a UniFi Protect gateway. AegisHA is the single
authoritative source of alarm truth: it runs its own alarm state machine,
validates per-user PINs, and exposes a real `alarm_control_panel` entity (plus
companion entities) to Home Assistant via **MQTT discovery** — so the keypad
works on any dashboard with no custom integration. UniFi Protect is treated as a
downstream sensor source + actuator (sirens/outputs/cameras), and is armed only
where the Protect Alarm Manager permits (it refuses local arm/disarm in "Global"
mode — AegisHA detects this and runs app-managed).

## Phase status

- **Phase 0** — Scaffolding (S6 service, manifest, multi-stage Docker build,
  config loader, health server, docs). **Done.** `./build.sh` + the repo
  smoke-test pass; the daemon boots, serves `/health`, and exits 0 on SIGTERM.
- **Phase 1** — Native MQTT alarm entity (the primary goal). **Done.** Hand-
  rolled stdlib MQTT 3.1.1 client + discovery bridge publish a real
  `alarm_control_panel.aegis_ha` with a `REMOTE_CODE` keypad; PBKDF2 PIN store
  with HMAC-indexed O(1) lookup, lockout, duress and one-time codes; the
  alarm state machine (`internal/alarm`) with exit/entry/trigger timing
  (synctest-tested); bidirectional `number`/`button`/sensor companion
  entities. Validated end-to-end against a real Mosquitto broker.
- **Phase 2** — UniFi Protect wrap with non-destructive Global-mode detection.
  **Done.** `internal/unifi` client (X-API-KEY, isolated InsecureSkipVerify
  transport) + manager: non-destructive `DetectMode` (local/global/app-
  managed), local-mirror arm/disarm, sensor polling (+ a Protect device-
  event WebSocket that pokes the poll for low-latency breach detection)
  → engine open-sensor feed + breach-triggers-while-armed, siren
  actuation, and `protect_link_mode`/`protect_connected`/per-zone entities.
  httptest-mock tested (incl. the Global-mode 400).
- **Phase 3** — Alarmo-fidelity + HA bus events. **Done.** HA bus events
  (`aegis_ha_command_success`/`_failed_to_arm`/`_triggered`/`_duress`/
  `_command_not_allowed`) via the Core REST proxy; restart-safe persistence
  (committed armed mode survives a restart; transient countdowns fail safe
  to disarmed); and the **full sensor model** (`internal/alarm/sensors.go`):
  per-sensor `modes`/`always_on`/`immediate`/`use_exit_delay`/`auto_bypass`/
  `allow_open`(arm-on-close)/`trigger_unavailable`, manual bypass, and
  sensor-group event-count-within-timeout debounce — configurable via the
  `sensors`/`sensor_groups` add-on options and per-zone bypass switches.
- **Phase 4** — Ingress web UI (HTMX + WebSocket) with per-HA-user PINs.
  **Done.** `internal/web` serves a keypad (live state pushed over a
  `golang.org/x/net/websocket` socket via the htmx ws extension) + admin
  console;
  per-user identity from the non-spoofable `X-Remote-User-Id` ingress
  header; ingress-path-aware links; health stays loopback-open. httptest-
  tested (identity gating, arm-via-keypad, admin gating).
- **Phase 5** — Companion Lovelace card + delivery. **Done.** `internal/card`
  embeds a vanilla `aegis_ha-card` custom element (generic
  alarm_control_panel card) and, when `enable_companion_card` is set,
  writes it to `/config/www/aegis_ha` (served at `/local/...`) AND
  auto-registers it as a Lovelace resource over the Supervisor Core-WS
  (`ha.RegisterLovelaceResource`, storage mode) — logging a manual snippet
  on YAML-mode dashboards.
- **Phase 6** — Hardening + docs. **Done.** One native-Go dependency
  (`golang.org/x/net/websocket`, see below); `InsecureSkipVerify` isolated
  to the UniFi client; secrets masked/never logged; full docs. The UniFi
  client is verified against a real UCG Fiber (Global mode correctly
  detected via the non-destructive `GET /v1/arm-profiles` 400-'global'
  signal; `/v1/nvrs` is a single object with an `armMode` object; sensors
  use `mountType`/`isOpened`). Intentionally NOT done: native `aegis_ha.*`
  HA services (would require a Python companion integration that can't be
  zero-install; the bidirectional MQTT entities + bus events cover
  automations) and UniFi cert-pinning (the gateway is local/trusted/IP, so
  `InsecureSkipVerify` is acceptable).

## Essential Commands

```bash
# Build the add-on image locally (auto-detects architecture)
./build.sh

# Run the repo smoke test (mock Supervisor + container + health/shutdown)
IMAGE_NAME=local/amd64-addon-local_aegis_ha:0.1.0 \
  bash ../.github/scripts/smoke-test.sh aegis_ha local/amd64-addon-local_aegis_ha:0.1.0

# Fast iteration on the Go code
go vet ./...
go test ./...
go build ./cmd/aegis_ha
```

## Version Management

AegisHA has no upstream binary to track — the add-on **is** the software. Version
bumps are manual (`config.yaml` `version` + a `CHANGELOG.md` entry). There is no
update script or automated update workflow (the `sonuntius` model). The add-on is
registered in `.github/scripts/update-base-image.sh` so its base image stays
current.

## Architecture and Key Components

- **`cmd/aegis_ha/`** — daemon entry point. Loads options, wires components, owns
  SIGTERM shutdown via `signal.NotifyContext`.
- **`internal/config/`** — options loader (`/data/options.json` → Supervisor REST
  fallback). List/object option fields are decoded leniently (`StringList`,
  `UserList`) because the CI mock Supervisor serializes them as bare strings.
- **`internal/web/`** — HTTP server on the ingress port (8099). Health endpoints
  (`/health`, `/api/health`) are loopback-reachable and unauthenticated for the
  Docker HEALTHCHECK and smoke-test; the keypad/admin UI (Phase 4) is gated to
  the Supervisor ingress source.
- **`internal/alarm/`** (Phase 1+) — the alarm state machine: one owner goroutine
  + a buffered command channel emitting immutable snapshots; exit/entry/trigger
  timers send timer-fired commands back to the owner.
- **`internal/store/`** (Phase 1+) — hashed PIN/user store at
  `/data/aegis_ha/store.json`.
- **`internal/mqtt/`** (Phase 1+) — minimal stdlib MQTT 3.1.1 client + discovery
  publisher + command bridge.
- **`internal/unifi/`** (Phase 2+) — UniFi Protect client.
- **`internal/ha/`** (Phase 3+) — Home Assistant control plane (Core REST events,
  optional Core-WS service/card registration via the Supervisor proxy).

## Critical conventions (non-negotiable)

- **`ARG BUILD_FROM` has no inline default** — the base image version comes from
  `build.yaml` at build time.
- **`apk upgrade --no-cache` before `apk add`** in the Dockerfile.
- **Architecture**: `aarch64` and `amd64` only.
- **S6 scripts** use `#!/usr/bin/with-contenv bashio`; the `run` script `exec`s
  the Go binary so it becomes PID 1 and receives SIGTERM directly.
- **CHANGELOG version header is bare `## X.Y.Z`** (date on the next line) so
  Core's release-notes regex extracts only the new entry.
- **Signed commits, no Claude Code attribution** (per repo `CLAUDE.md`).

## Dependency policy (stdlib-first, native Go)

AegisHA has exactly **one** non-stdlib dependency: **`golang.org/x/net/websocket`**
— the Go team's own package in the `golang.org/x` namespace, the most
"native Go" choice given the standard library has no WebSocket implementation
(only `http.Hijacker` to build one by hand). This matches the sonuntius
precedent. It is used for both real-time paths: the ingress UI live-state push
(htmx WebSocket extension) and the UniFi Protect device-event subscription.
Per its own package docs, `x/net/websocket` "currently lacks some features"
of the more actively-maintained `github.com/gorilla/websocket` and
`github.com/coder/websocket`; we choose it anyway for native-Go affinity (the
Go-team `golang.org/x` namespace), and our usage — a one-way state push plus a
change-signal read loop — does not need those missing features.

Everything else stays stdlib:

- **PIN hashing**: stdlib `crypto/pbkdf2` (Go 1.24+) + `crypto/hmac` index +
  `crypto/subtle` constant-time compare — **no** `x/crypto/bcrypt`.
- **MQTT**: a minimal in-tree MQTT 3.1.1 client over `net.Conn`/`crypto/tls`
  (`internal/mqtt/client.go`) — **no** `paho`.
- **Lovelace card**: a vanilla custom element — **no** Node/Rollup build.

Any additional third-party import must be justified here.

Go 1.26 features used: `errors.AsType[T]`, `new(expr)`, `strings.SplitSeq`,
`testing/synctest` (deterministic alarm-timer tests), `ServeMux` pattern
routing, `omitzero` JSON tags, `log/slog`.

## Security model

- AegisHA owns PINs/lockout; PINs are PBKDF2-hashed at rest, looked up by a
  server-pepper HMAC index (O(1), no per-user hash sweep on the unauthenticated
  MQTT topic), wrong-PIN attempts increment a lockout counter *before* the hash
  verify.
- UniFi capability detection is **non-destructive** — never call arm/disarm as a
  probe.
- `InsecureSkipVerify` is isolated to the UniFi HTTP client only.
- The web server trusts `X-Remote-User-*` only behind ingress and binds to the
  Supervisor IP; admin actions are gated on a resolved admin user.

## Testing Checklist

- `go vet ./...` clean
- `go test ./...` passes
- `./build.sh` produces an image
- Smoke test passes (`bash ../.github/scripts/smoke-test.sh aegis_ha <image>`)
- Health endpoint responds on `:8099`; container exits 0 on SIGTERM
