# Changelog

## Version 0.1.11 (2026-05-11)

### MA queue_id auto-discovery, `volume_step` default → 10

Follow-up from the v0.1.10 live test. v0.1.10 fixed the auth path
(`ma_token` accepted) but exposed a deeper issue: the queue_id we
were deriving from the HA entity_id (`media_player.3rspk_a8e29151e187_2`
→ `3rspk_a8e29151e187`) doesn't match MA's actual internal player_id,
so the play_media WS call still failed — this time with
`error_code: 10 Queue 3rspk_a8e29151e187 is not available`. MA's UI
showed audio playing from the HA REST fallback path but, again, no
metadata.

**1. Auto-discover the queue_id via `players/all`.** On startup, after
the MA WS reachability probe succeeds, ma-bridge now sends
`players/all` over the MA WebSocket, logs every visible player at
info (player_id, display_name, name, provider, available, type), and
runs a four-rule matcher against the configured HA entity_id:

  1. Exact `player_id == entity_slug`
  2. Exact match after stripping a trailing `_N` (the HA collision
     disambiguator)
  3. Slug-equivalent `display_name` / `name`
  4. Substring containment in either direction (covers
     `<provider>_<id>` MA naming)

The matched player_id becomes the dispatcher's `MAPlayerQueue`. If
nothing matches, ma-bridge logs the available IDs at warn and tells
the user to set `ma_queue_id` explicitly.

`internal/ma/client.go`: new `PlayerInfo`, `ListPlayers`, `MatchPlayer`,
`slugify`, `containsFold`.
`cmd/ma-bridge/main.go`: new `resolveMAQueueID` orchestrator running
config-override → discovery → derive fallback.

**2. New `ma_queue_id` option.** An explicit override for the auto-
discovered value. When set, ma-bridge skips `players/all` entirely
and uses the value as-is. This is the right thing to set when (a)
discovery can't find the right player or (b) you want the lookup to
short-circuit at startup.

`internal/config/config.go`: new `MAQueueID` field.
`config.yaml`: `ma_queue_id: ""` option, `ma_queue_id: str?` schema.

**3. `volume_step` default 5 → 10.** Most Sendspin/AirPlay speakers
(including the user's 3RSPK) step in 10s on their physical buttons,
which makes 10 the least-surprising default. Existing configs that
explicitly set `volume_step: 5` are preserved.

`config.yaml`: `volume_step: 10`.
`internal/config/config.go::EffectiveVolumeStep`, `cmd/yt-cast/player.go`:
default constant flipped to 10.

### Tests

`internal/ma/match_player_test.go` (new): covers each MatchPlayer
rule plus slugify edge cases.

## Version 0.1.10 (2026-05-11)

### Volume quantisation, actionable MA-auth guidance, debug telemetry

Three follow-ups from the v0.1.9 live test, all narrow and additive:

**1. Volume quantisation (`volume_step`).** The YouTube cast UI emits a
fresh volume value on every slider tick, which (a) floods the MA log
and (b) doesn't match the host speaker's physical button increments
— the user's speaker, for example, steps in 10s. The adapter now
rounds the incoming 0–100 level to the nearest multiple of
`volume_step` (default `5`) and drops back-to-back repeats so
identical rounded values aren't pushed twice. Set `volume_step: 10`
in addon options to match a speaker that steps in 10s, or `1` to
disable rounding entirely.

`cmd/yt-cast/player.go::DoSetVolume`: the new `roundToStep` helper
plus per-adapter `lastQuantizedVolume` / `lastVolumeMuted` /
`lastVolumeHasSent` fields handle rounding and dedup. The original
raw value is preserved in the log for diagnosis.

**2. Actionable warning when MA WS auth is missing.** v0.1.9's MA-WS
play_media path needs `ma_token` set to attach rich metadata; when
the user hasn't set one, MA returns `error_code: 20` ("Authentication
required") and we silently fall back to HA REST (no metadata). The
client now detects the empty-token case up-front and the error-20
response post-command, returns a typed `ma.ErrAuthRequired`, and the
dispatcher logs a one-time warning explaining how to mint a token
in MA (Settings → Security → API Tokens) and where to paste it.
Subsequent attempts log at debug to avoid spam.

`internal/ma/client.go`: new `ErrAuthRequired` sentinel,
`isAuthRequiredCode` helper.
`internal/dispatcher/dispatcher.go`: `authWarned` flag, one-time
warn + thereafter debug.

**3. Verbose debug telemetry for long-video drift.** The user
reported that on very long videos the phone UI doesn't update its
timestamp until a pause/play cycle nudges it. To narrow this down,
`updateCachedState` now emits a single debug record per MA push
showing the incoming state, the local wall-clock estimator's value,
and the delta between them. `DoPause` and `DoResume` log the
local-estimator transition (frozen at / resumed after pause-duration).
Enable in addon options with `log_level: debug`.

`cmd/yt-cast/player.go`: extracted `localEstimateLocked()` so the
estimator value is reusable, added drift logging in
`updateCachedState`, debug logs in `DoPause` / `DoResume`.

### Schema / config

- `config.yaml` adds `volume_step: 5` (default) with schema
  `int(1,50)?`.
- `internal/config/config.go`: new `VolumeStep` field with
  `EffectiveVolumeStep()` accessor (default 5, clamped to ≤50).

## Version 0.1.9 (2026-05-11)

### Local position estimation + seek/volume visibility

Two issues from the v0.1.8 live test:

**1. Position snaps to 0:00 on the first pause after play.** Even with
the v0.1.8 `subscribe_events` fix, HA does not emit `state_changed` for
a `media_player` entity until MA has actually started streaming
(typically 2–10 seconds after our `play_media` call). If the user pauses
during that gap, the engine calls `DoGetPosition`, our cachedState is
still empty, and we return 0 — the phone's progress bar snaps to 0:00
in both YouTube *and* the MA UI on the first pause. Once playback runs
long enough for HA to emit position attributes, sync becomes correct.

`cmd/yt-cast/player.go`: the adapter now tracks a local wall-clock
position estimator alongside the cached state. `DoGetPosition` prefers
the cached value when available (MA's truth), and falls back to the
estimator otherwise. Estimator is seeded by `DoPlay` (with the
sender-supplied start position), frozen by `DoPause`, advanced by
`DoResume` (absorbing the pause duration), rebased by `DoSeek`, and
cleared by `DoStop`.

The result: the phone always sees a sensible non-zero position from
the moment the cast lands, even during the initial gap before MA
reports back.

**2. Seek and volume traces went to debug.** Logging on `DoSeek` and
`DoSetVolume` is now info-level so we can diagnose the inconsistent
"no sound after scrub" / "volume slider doesn't move the speaker"
reports from the live device. Position rebasing on `DoSeek` is
described above.

### MA-WS native `play_media` for rich metadata (the RAW URL fix)

v0.1.8 confirmed that MA's URL provider strips metadata extras passed
through HA's `media_player.play_media` service regardless of the
shape (flat or nested) — the title in MA's UI still showed the raw
`videoplayback?…` URL.

The route that bypasses the stripping is **MA's native WebSocket
`player_queues/play_media` command** with a fully-formed `MediaItem`
object. New `internal/ma/PlayMediaItem(ctx, url, token, queueID,
MediaItem, logger)`: opens a short-lived WS connection per call,
handles the schema-aware auth handshake, sends the command, waits
for the matching `message_id` response. `internal/ma` also gains a
`MediaItem` + `MediaItemImage` struct mirroring the subset of MA's
schema we populate (`item_id`, `provider`, `name`, `media_type`,
`image`, `artists`, `uri`).

The dispatcher now tries the MA-WS path first for url-provider
intents when `MAWsURL` + `MAPlayerQueue` are configured, falling
back to the HA-routed `media_player.play_media` on any error so
configuration regressions degrade gracefully.

To bridge HA entity_id → MA player_id (the WS command needs MA's
internal id), `ma.DerivePlayerID` strips `media_player.` and any
trailing `_N` disambiguator HA adds when multiple integrations
register the same player — e.g.
`media_player.3rspk_a8e29151e187_2` → `3rspk_a8e29151e187`. Covered
by a test matrix in `internal/ma/derive_player_id_test.go`.

`cmd/ma-bridge/main.go` wires this up at startup when both an MA
hostname is discovered (or `ma_ws_url` is set) and `ma_player_id` is
configured. A new log line on startup —
`dispatcher: MA WS play_media path enabled` — confirms the bypass
is active.

## Version 0.1.8 (2026-05-11)

### subscribe_events instead of subscribe_trigger; flat-and-nested play_media metadata

First-light testing of v0.1.7 surfaced two bugs that the new logging
immediately diagnosed:

**1. HA state subscription wasn't actually receiving the state we
care about.** The watcher logged `state: HA WS authenticated` but
never logged `state: first HA state update received` — meaning the
`subscribe_trigger` API I used in v0.1.7 only fires on transitions
of the primary `state` field (idle ↔ playing ↔ paused). Attribute-
only updates like `media_position` ticking forward, `volume_level`
changing from the speaker, etc. went through `state_changed` events
which `subscribe_trigger` does not surface.

Switched `internal/state/watcher.go` to use `subscribe_events` with
`event_type: state_changed` and filter for the configured entity_id
client-side. This is the broad event stream — every attribute change
fires it. The throughput is one ~200-byte JSON parse per state-change
in the whole HA install, which is fine.

**2. Music Assistant's UI still shows the raw `videoplayback?…` URL
as the title even though `has_extra=true` confirmed the metadata
extras were being sent.** MA's URL provider apparently doesn't read
`extra.metadata.{title, artist, image}` — different MA versions read
metadata from different locations and the URL-provider code path in
particular looks at the flat `extra.<field>` form.

`internal/dispatcher/dispatcher.go` now emits metadata fields under
BOTH `extra.<field>` (flat) AND `extra.metadata.<field>` (nested),
defensive against whichever shape MA's resolver consumes. Thumbnail
is also mirrored under `extra.metadata.artwork` for MA versions that
look for that key specifically.

If MA's UI still shows the raw URL after this, the next escalation
is to drop `media_player.play_media` (the HA-routed service call)
and use MA's native WS `player_queues/play_media` command with a
full MediaItem object — that bypass guarantees richer metadata
handling, at the cost of a deeper change to the dispatcher.

## Version 0.1.7 (2026-05-11)

### Bidirectional player-state sync + rich MA metadata

A first end-to-end success of v0.1.6 (audio actually playing on a real
MA player) surfaced three UX gaps that all trace back to two missing
hooks. Both are fixed in this release.

**Problem 1 — phone's position counter resets to 0 on every transition.**
Phase 6a switched the state subscription to MA's direct WS instead of
HA's core WS. MA's WS events identify players by MA's *internal* player
id, not the HA `media_player.*` entity_id we configured in
`ma_player_id`, so the watcher silently filtered out every event and
the adapter's cached PlayerState never updated. With nothing in the
cache, `DoGetPosition` / `DoGetDuration` / `DoGetVolume` returned 0
and the phone's UI extrapolated from there.

Fix: `cmd/ma-bridge/main.go` now **always runs the HA core WS state
watcher** (`internal/state`). HA subscribes by entity_id and aggregates
MA's reports — `media_position`, `media_duration`, `media_title`,
`volume_level` all flow through reliably. The MA-direct WS connection
probe is kept as an advisory log line ("ma: direct WS reachable") for
visibility into the MA server version / schema, but no longer carries
the state subscription.

`internal/state/watcher.go` now logs an info-level line on the first
broadcast for a given entity (subsequent updates stay at debug to
avoid log spam) so confirmation that the chain is live is visible at
the default log level.

**Problem 2 — external state changes (pause/resume from MA's UI, volume
changes from the speaker, seek-by-double-tap on the phone) never made
it back to the phone's Lounge UI even when the cache was updated.**
The yt-cast engine only emits state events on engine-side transitions
(play / pause / stop), not when the host's cached state changes
externally.

Fix: new `receiver.EmitPlayerState(ctx)` exposes the engine's state
emission. The adapter now fires an `onStateChange` callback every time
`updateCachedState` is called from the IPC connector. `cmd/yt-cast/main.go`
wires that callback to `receiver.EmitPlayerState`, so every MA-driven
state update propagates back through:

```
HA state_changed → state.Watcher → IPC → adapter.updateCachedState
                                          → receiver.EmitPlayerState
                                            → engine.EmitCurrentState
                                              → orchestrator builds
                                                onStateChange / onVolumeChanged
                                                / nowPlaying messages
                                              → Lounge POST to phone
```

This is what makes volume sync, seek accuracy, and continuous
position display work bidirectionally.

**Problem 3 — Music Assistant's UI showed the raw `videoplayback?…`
URL as the track title.** MA's URL provider just plays whatever it's
handed and has no out-of-band metadata source for YouTube URLs.

Fix: synchronously resolve title + channel via the existing oEmbed
helper before emitting the `PlayIntent`, populate
`PlayIntent.Metadata` with `title` / `channel` / `thumbnail` /
`video_id` / `source`. `internal/dispatcher` reads those and passes
them through to `media_player.play_media` as
`extra.metadata.{title, artist, image, thumb, external_id, source}`,
matching MA's expected schema. `internal/ha/client.PlayMedia` grew an
`extra` parameter.

Resolution adds ~200ms on cold cache (oEmbed call) but the existing
yt-dlp stream resolve already takes 1–5s in the same path, so the
incremental cost is negligible.

Thumbnail handling: `cmd/yt-cast/metadata.go` now also captures
`thumbnail_url` / `thumbnail_width` / `thumbnail_height` from the
oEmbed payload — YouTube's officially-recommended preview image for
the video. The player adapter prefers that over the hard-coded
`https://i.ytimg.com/vi/<id>/hqdefault.jpg` fallback, and the
dispatcher forwards it as both `extra.metadata.image` and
`extra.metadata.thumb` so MA's UI can render proper cover art.

## Version 0.1.6 (2026-05-11)

### Pre-resolve YouTube watch URLs to direct stream URLs via yt-dlp

A first live v0.1.5 test with `ma_player_id` correctly set surfaced
the next blocker:

```
[music_assistant.player_queues] Skipping https://www.youtube.com/watch?v=...:
  Unable to retrieve info (Invalid data found when processing input)
[music_assistant.webserver] player_queues/play_media: No playable items found
```

Music Assistant's "URL" provider expects a direct audio stream URL
(mp3 / m4a / `googlevideo.com` / etc.) and ffmpeg-probes it. Handing
MA a raw `https://www.youtube.com/watch?v=...` URL fails because MA
gets HTML instead of audio bytes. The v0.1.3 commit-message claim
that MA's stream extractor handles raw YouTube watch URLs was wrong.

Fix: install `yt-dlp` in the addon image (apk package) and pre-resolve
the watch URL to a direct audio stream URL in the yt-cast Player
adapter's `DoPlay` before emitting the `PlayIntent`. yt-dlp's signed
googlevideo.com URLs are then handed to MA, which can ffmpeg-probe
them successfully and play.

The resolution is synchronous — it adds ~1–2s before the engine's
LOADING → PLAYING transition, which is the correct UX (the phone's
status genuinely is LOADING while we resolve). If resolution fails
(network error, video unavailable, etc.) we emit the bare watch URL
and log the failure so MA's "No playable items found" log line is
preceded by a clear root cause from our side.

Existing tests:
- `streamresolve_test.go` covers empty-id, missing-binary, and
  timeout-propagation paths.
- The resolver is only invoked for `Theme == "cl"` (regular YouTube
  app), so the YouTube Music path (`ytmusic://track/<id>`) is
  unchanged.

Image size impact: ~5 MB for `yt-dlp` + its Python deps via apk.

## Version 0.1.5 (2026-05-11)

### Trim whitespace from every string option

A first live test of v0.1.4 with `ma_player_id` set caught an
invisible-failure mode: HA's addon options UI happily preserves a
leading or trailing space typed (or pasted) into a string field. The
loaded config carried `ma_player_id=" media_player.3rspk_a8e29151e187_2"`
and `media_player.play_media` would have rejected that entity_id as
unknown without ever logging a useful diagnosis.

`config.Options.normalize()` now `strings.TrimSpace`'s every string
field after each successful load (file path + Supervisor REST path) —
`log_level`, `ma_player_id`, friendly names, cert/key paths, the four
HA/MA URL+token overrides, and every `tidal_fallback.*` string. New
`TestNormalize_TrimsStringFields` covers the matrix.

This complements the `cmd/cast-receiver/options.go` cert-path trim that
already existed and centralises the defensive behaviour in the loader.

## Version 0.1.4 (2026-05-11)

### MA addon hostname derivation

A first live v0.1.3 cast revealed that the Supervisor `/addons` bulk
listing returns each addon entry with the `hostname` field empty —
the field is only populated when you call `/addons/<slug>/info`
individually. Result: `FindMAAddonHostname` would correctly *match*
the `music_assistant` addon in the list, but return an empty hostname
and fall back to the HA core WS path even when MA was installed and
reachable.

Fix: when the bulk-listing hostname is empty, derive it from the slug
by replacing underscores with hyphens (the canonical HA Supervisor
addon-to-Docker hostname convention). For example,
`d5369777_music_assistant` → `d5369777-music-assistant`. Probed and
confirmed against the live HA host.

The Phase 6a direct-MA-WS path now engages automatically on installs
that have Music Assistant alongside Sonuntius, without the user
needing to set `ma_ws_url` manually.

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
