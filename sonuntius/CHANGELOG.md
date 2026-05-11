# Changelog

## Version 0.1.3 (2026-05-11)

### YouTube-classic playback path + auto-discovery observability

- `cmd/yt-cast/player.go`: when a Cast sender is the regular YouTube
  app (`Video.Client.Theme == "cl"`) the adapter now emits
  `PlayIntent{Provider:"url", URL:"https://www.youtube.com/watch?v=<id>"}`
  instead of the previous `provider="youtube"` (which the dispatcher
  has no URI template for and dropped as "unresolvable"). Music
  Assistant's stream extractor (yt-dlp) handles arbitrary YouTube
  watch URLs, so the dispatcher's existing `provider="url"` path is
  reused. The YouTube Music path (`Theme == "m"`) is unchanged and
  continues to use `ytmusic://track/<id>`.
- `internal/ha/client.go` + `cmd/ma-bridge/main.go`: every outcome of
  the MA-addon auto-discovery path now logs at info or warn level —
  addon-list count, matched slug + hostname on success, an explicit
  "not discovered" line on the empty-result path. The previous silent
  fallback to HA core WS was easy to miss when debugging real installs.
- 5 new test cases in `cmd/yt-cast/player_test.go` covering the
  resolveIntent provider mapping (YT Music, YouTube classic, unknown
  surface).

### YouTube video-title resolution + Lounge-state visibility

- New `cmd/yt-cast/metadata.go` (with tests) — `metadataResolver`
  fetches video title + channel via YouTube's public oEmbed endpoint
  (`https://www.youtube.com/oembed?url=...&format=json`), stdlib-only,
  no third-party deps, in-process cache to avoid re-fetching the same
  video. `DoPlay` fires the resolution in a goroutine so the play path
  stays optimistic; the addon log now shows
  `yt-cast: now playing  video_id=bp4_7T9J6Fg  title="birds for some reason"  channel="Avocado Animations"  provider=url`
  shortly after every cast.
- `internal/ytcast/youtubeapp.go`: when the orchestrator pushes a
  player-state update to the connected sender (the messages that drive
  the phone's play/pause/skip/seek controls), it now logs the message
  names + status code at info level. The previous behavior was silent
  on success, so a perpetual loading spinner on the phone was opaque
  from the addon side. Lines like
  `Pushing player-state update to sender: names=[onStateChange nowPlaying onHasPreviousNextChanged] status=1`
  now appear after every state transition.

## Version 0.1.2 (2026-05-11)

### MA addon auto-discovery + /share/sonuntius bootstrap

- `config.yaml`: add `hassio_role: manager`. Without this, the addon's
  Supervisor token is rejected by `GET /addons` (HTTP 403) and the
  Phase 6a direct-MA-WS auto-discovery silently falls back to the HA
  core WS path. Manager is the lowest role that grants the addon-list
  endpoint; we don't need anything broader.
- `cont-init.d/10-prepare.sh`: auto-create `/share/sonuntius/` so the
  user has a known, pre-existing directory to drop the AirReceiver
  cert (Phase 3 Tidal proxy) and the iFi tarball (Phase 5 fallback)
  into. The dir is empty by default and carries no secrets.
- `internal/ha/client.go`: elevate the `FindMAAddonHostname` HTTP-error
  log from debug to warn with an explicit hint about `hassio_role` —
  so future permission issues surface in the addon log instead of
  being swallowed at the default log level.

## Version 0.1.1 (2026-05-11)

### Configurable listen ports — Music Assistant port-3000 conflict fix

- New options `yt_cast_dial_port` (default `8008`) and
  `cast_receiver_tls_port` (default `8009`). Both plumbed through
  `internal/config` and consumed by the matching cmd binaries.
- The DIAL HTTP default changed from upstream's `3000` to `8008`
  because Music Assistant — which runs with `host_network: true` and
  binds host port 3000 for its frontend — was causing yt-cast's
  `Server.Start` to fail with `bind: address already in use` and
  enter the retry-with-backoff loop on every fresh install.
- DIAL discovery does not require a specific port; the SSDP
  advertisement carries the actual port via the `LOCATION` header,
  so cast senders find the receiver regardless of the new default.
- 4 new unit tests in `internal/config/config_test.go` covering the
  effective-port helpers (default, user override, partial override).

## Version 0.1.0 (2026-05-11)

### Phase 6 — Polish (health endpoint + persistent state + direct MA WS)

- Plan §10 Path B implemented: a new `internal/ma/` package opens the
  Music Assistant addon's WebSocket directly (`ws://<host>:8095/ws`),
  performs the schema-aware auth handshake (auth required when the MA
  server's `schema_version >= 28`), and broadcasts `PlayerState` frames
  derived from `player_updated` / `player_added` /
  `player_queue_time_updated` events. The bridge probes MA-direct first
  (auto-discovered hostname via Supervisor `/addons`, or explicit
  `ma_ws_url` override) and transparently falls back to the HA core WS
  (`internal/state`) when the direct path is unreachable. Closes the
  Phase 1 deviation.
- Plan §6 Phase 6 health endpoint shipped at
  `http://127.0.0.1:8099/health` (hosted by ma-bridge). Returns
  aggregated component statuses (config, dispatcher, ipc, state) as
  JSON. Reports `degraded` when any component is unhealthy so HA's
  watchdog and external tooling can distinguish boot order issues from
  configuration gaps.
- Plan §6 Phase 6 persistent state — first concrete piece: cast-receiver
  records the SHA-256 fingerprint of the loaded AirReceiver cert under
  `/data/sonuntius/airreceiver_cert.fingerprint` and logs a warning when
  the cert changes across restarts. Receiver UUIDs continue to persist
  via the existing JSON-file-per-key store under `/data/sonuntius/`.
- Smoke test extended with health-endpoint check and degraded-state
  aggregation check.

### Phase 5 — Tidal Connect binary fallback (opt-in)

- New service: `tidal-connect`. When `tidal_fallback.enabled = true` (off
  by default), `cont-init.d/20-tidal-fallback.sh` extracts the
  user-supplied iFi binary tarball from `/share/sonuntius/`, locates
  `tidal_connect_application` anywhere in the extracted tree, verifies
  the bundled cert, and links both to stable paths. The S6 service
  execs the binary with `--playback-device hw:Loopback,0,0` and the
  user's friendly name. When fallback is disabled the service logs
  "idle" and sleeps so S6 doesn't restart-loop.
- New service: `alsa-to-sendspin`. New Go binary at
  `/usr/local/bin/alsa-to-sendspin` that execs `arecord` against the
  loopback capture side and forwards PCM frames to the Sendspin server
  WebSocket. Sendspin frame format is the only place wire-level
  encoding lives (`encodeFrame`); currently passes raw PCM through as
  a clearly-marked Phase 2.1 TODO since the Sendspin spec is still
  being finalized. Reconnect-with-backoff on the WS side, signal-
  propagating exec on the arecord side.
- Dockerfile now installs `alsa-utils` (provides `arecord`) plus
  `libc6-compat` on aarch64 only so the ARMv7 iFi binary can run via
  the kernel's compat layer. `amd64` skips libc6-compat by design —
  the fallback is recommended only on aarch64.
- `cont-init.d/20-tidal-fallback.sh` never fails init; it warns clearly
  on every failure path (tarball missing, arch mismatch, binary not
  found in tree, cert not found) and leaves the marker file absent so
  the two services stay idle.
- Smoke test extended to verify the disabled-by-default state — both
  Phase 5 services must log "idle", never attempt to exec the binary,
  and never crash-loop.

### Phase 3b + Phase 4 — Tidal customData parser + Default Media Receiver fallback

- New parsers package at `internal/castv2/parsers/`:
  - `parsers.NewTidal(...)` extracts the Tidal track ID from a CASTV2
    LOAD message by probing `customData.tidal.trackId`,
    `customData.trackId`, `customData.media.trackId`, and
    `customData.data.trackId` across both the outer LOAD customData and
    the inner `media.customData`. A metadata-subtitle heuristic is the
    last-ditch fallback for senders whose customData shape has not yet
    been profiled. All inspected blobs log at debug so the parser can be
    iterated against real Tidal traffic.
  - `parsers.NewGeneric(...)` is the Phase 4 backstop: claims any LOAD
    whose `contentType` starts with `audio/` (or whose `contentType` is
    empty but whose `contentId` parses as an http(s) URL) and emits a
    `url`-provider intent so MA's URL provider can play arbitrary public
    audio streams.
- New service: `cast-receiver` (Go binary at `/usr/local/bin/cast-receiver`).
  Loads the AirReceiver cert + companion artifacts from
  `/share/sonuntius/`, advertises a `_googlecast._tcp` mDNS instance
  under `friendly_name_tidal`, runs the CASTV2 TLS server, registers the
  Tidal + Generic parsers (in that priority order), and translates every
  parser-claimed LOAD into a `PlayIntent` over the existing IPC bus.
  Mirrors the yt-cast resilience model: IPC reconnect-with-backoff,
  Server.Start retry-with-backoff, and graceful degrade when the cert
  is missing (the binary stays alive, logs `TLS server disabled (cert
  not configured)`, and lets mDNS run on its own so the addon never
  enters an S6 restart loop).
- `cont-init.d/10-prepare.sh` now warns when the configured
  `cast_cert_path` is missing instead of failing init.
- Dockerfile builds four Go binaries (`ma-bridge`, `sonuntius-ctl`,
  `yt-cast`, `cast-receiver`).
- Smoke test verifies the cast-receiver service boots, logs the no-cert
  path, and stays up without an S6 restart loop.

### Phase 2 — YouTube / YouTube Music DIAL + Lounge receiver

- Full Go 1.26 port of [`yt-cast-receiver`](https://github.com/patrickkfkan/yt-cast-receiver)
  v2.1.1 (upstream commit pinned at
  `83d61fa169e33c5e0046c2440b99a17cd9493e73`) lands under
  `internal/ytcast/`. ~9 000 LOC across foundation (logger, datastore,
  asyncq, errors, constants, types, player), DIAL layer (stdlib SSDP
  responder + UPnP description + DIAL HTTP endpoints, replacing the
  upstream `peer-dial` + Express deps), Lounge protocol (RPCConnection
  long-poll, line-by-line Message parser, BindParams AID/GSN/SID
  arithmetic, per-sender Session lifecycle, in-memory Playlist, pairing
  code service), and the YouTubeApp / YouTubeCastReceiver orchestrator.
- Every ported `.go` file opens with a `// Maps to:` header naming the
  upstream source so the port chain is auditable. Go-only support files
  use `// Maps to: N/A — Go-only ...`.
- Upstream pin recorded in `internal/ytcast/constants/upstream.go` and
  in `internal/ytcast/UPSTREAM.md` (full file-by-file port table).
- Stdlib-only — same dependency posture as Phase 1. No third-party
  YouTube client; `DefaultPlaylistRequestHandler` is a deliberate stub
  that returns empty metadata (sonuntius only needs the video ID;
  marked as a Phase 2.1 TODO).
- New service: `yt-cast` (Go binary at `/usr/local/bin/yt-cast`).
  Persists a stable receiver UUID under `/data/sonuntius/`, dials the
  ma-bridge IPC broker with reconnect/backoff, retries `Start` on
  failure so a misconfigured network does not crash the S6 service.
  Logs the pinned upstream commit short SHA on startup.
- Player adapter at `cmd/yt-cast/player.go` translates `DoPlay` /
  `DoPause` / `DoResume` / `DoStop` / `DoSeek` / `DoSetVolume` into
  `PlayIntent` / `TransportCommand` / `VolumeCommand` events over IPC.
  `Client.Theme` "m" maps to `ytmusic`, "cl" to `youtube`.
- Dockerfile builds three Go binaries now (`ma-bridge`,
  `sonuntius-ctl`, `yt-cast`). Smoke test verifies the yt-cast banner
  reports the pinned upstream commit and the service stays up without
  an S6 restart loop.

### Phase 1 — Music Assistant bridge skeleton

- Implementation language switched to **Go 1.26** with stdlib-only
  packages plus `golang.org/x/net/websocket` (Go team's blessed
  WebSocket package) for HA state subscriptions.
- New service: `ma-bridge` (Go binary at `/usr/local/bin/ma-bridge`).
  Reads addon options from `/data/options.json` with a Supervisor REST
  fallback, opens a JSON-line UDS broker at `/run/sonuntius/events.sock`,
  and dispatches `PlayIntent` / `TransportCommand` / `VolumeCommand`
  events into Home Assistant via the `media_player.*` services.
- New CLI: `sonuntius-ctl` for sending one-shot events into the bridge
  during development and from the smoke test
  (`sonuntius-ctl play --provider ytmusic --track-id <id>`).
- HA core WebSocket subscription for the configured `media_player.*`
  entity. State changes translate to `PlayerState` events and broadcast
  to every connected IPC client (cast-receiver / yt-cast in later
  phases). The watcher reconnects on failure with exponential backoff.
- `cast-receiver` and `yt-cast` remain Phase 0 sleep-infinity stubs;
  they come online in Phase 3 and Phase 2 respectively.
- Dockerfile is now a multi-stage build (Go builder + hassio-addons base).

### Phase 0 — scaffolding (preceding work)

- Directory layout, S6 services, and addon manifest in place so the
  container installs cleanly and S6 supervises three placeholder services
  (`cast-receiver`, `yt-cast`, `ma-bridge`).
