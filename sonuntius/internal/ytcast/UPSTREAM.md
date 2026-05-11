# Upstream: yt-cast-receiver

This directory is a Go 1.26 port of the Node.js library
[`yt-cast-receiver`](https://github.com/patrickkfkan/yt-cast-receiver).

## Pinned commit

| Field   | Value |
| ---     | --- |
| Commit  | `83d61fa169e33c5e0046c2440b99a17cd9493e73` |
| Tag     | `v2.1.1` |
| Date    | `2026-03-19` |
| Subject | `v2.1.1 (release)` |

Verify with: `cd /home/adardano3/Developer/yt-cast-receiver-upstream && git rev-parse HEAD`

## Update workflow

When updating to a newer upstream commit:

1. Fetch + checkout the new commit in `/home/adardano3/Developer/yt-cast-receiver-upstream`.
2. Bump the pin in this file and in `internal/ytcast/constants/upstream.go`.
3. Walk each ported file (each carries a `// Maps to:` header) and re-port any drift.
4. Update the port table below with new files / deletions.
5. Re-run `go vet ./internal/ytcast/...` and `go build ./internal/ytcast/...`.

## Port table

| Upstream file | Go destination | Notes |
| --- | --- | --- |
| `src/lib/utils/Logger.ts` | `internal/ytcast/logger/logger.go` | Interface only |
| `src/lib/utils/DefaultLogger.ts` | `internal/ytcast/logger/default.go` | slog-backed default; `color` flag preserved for parity but is currently a no-op |
| `src/lib/utils/DataStore.ts` | `internal/ytcast/datastore/datastore.go` | Interface; `Set/Get` standardize on `json.RawMessage` because Go has no method-level generics |
| `src/lib/utils/DefaultDataStore.ts` | `internal/ytcast/datastore/default.go` | JSON-file-per-key store, replaces `node-persist`; keys are URL-path-escaped |
| `src/lib/utils/AsyncTaskQueue.ts` | `internal/ytcast/asyncq/queue.go` | Channel-backed serial executor; `Task.Run` accepts a `context.Context` so cancellation flows through |
| `src/lib/utils/Errors.ts` | `internal/ytcast/yterrors/errors.go` | Error types; sentinel `errors.Is` plus `errors.As`-friendly concrete structs |
| `src/lib/Constants.ts` | `internal/ytcast/constants/constants.go` | Protocol URLs, enums (`PlayerStatus`, `LogLevel`, `Status`, `MutePolicy`, `ResetPlayerOnDisconnectPolicy`, `AutoplayMode`). The `CLIENTS` map upstream stores here is co-located with the `Client` type in `internal/ytcast/types/client.go` to avoid a constants ↔ types import cycle |
| n/a | `internal/ytcast/constants/upstream.go` | Go-only commit pin (`UpstreamCommit`, `UpstreamTag`, `UpstreamVersion`) |
| `src/lib/app/Sender.ts` | `internal/ytcast/types/sender.go` | `Parse(json.RawMessage) (*Sender, error)` ports `Sender.parse(data)` |
| `src/lib/app/Client.ts` | `internal/ytcast/types/client.go` | YT vs YTMUSIC; `Clients` map and `ClientByTheme` co-located here |
| `src/lib/app/Video.ts` | `internal/ytcast/types/video.go` | `VideoContext.Extra` carries the upstream `& Record<string, any>` overflow |
| `src/lib/Player.ts` | `internal/ytcast/player/player.go` | Interface for the `do*` hooks only; the state machine that wraps them lives with the Phase 3 orchestrator |
| n/a | `internal/ytcast/player/events.go` | Go-only `State`, `StateEvent`, and channel-based `EventBus` extracted from upstream's EventEmitter |
| `src/lib/dial/DialServer.ts` | `internal/ytcast/dial/server.go` | Server struct + Options + Start/Stop lifecycle; folds peer-dial's `Delegate.{getApp,launchApp,stopApp}` callbacks into Server.OnLaunch/OnStop/SetState because we serve only `YouTube` |
| `src/lib/dial/DialServer.ts` (SSDP portion) | `internal/ytcast/dial/ssdp.go` | Go-only (was: peer-dial / peer-ssdp dep). Stdlib `net.ListenMulticastUDP` responder for M-SEARCH on 239.255.255.250:1900 + periodic NOTIFY ssdp:alive ticker + ssdp:byebye on shutdown. Advertises `upnp:rootdevice`, `uuid:<UUID>`, `urn:dial-multiscreen-org:device:dial:1`, `urn:dial-multiscreen-org:service:dial:1` |
| `src/lib/dial/DialServer.ts` (HTTP / UPnP description portion) | `internal/ytcast/dial/upnp.go` | Go-only (was: peer-dial / express dep). net/http ServeMux with the four DIAL routes (GET /apps, GET/POST /apps/:appName, DELETE /apps/:appName/:pid) + encoding/xml renderers for `device-desc.xml` and `app-desc.xml` byte-equivalent to peer-dial's EJS templates |
| `src/lib/dial/DialServer.ts` (launch body parsing) | `internal/ytcast/dial/launch_payload.go` | Go-only. Parses the `application/x-www-form-urlencoded` DIAL launch body and extracts `pairingCode` plus any spillover sender params. Replaces the inline parsing upstream YouTubeApp.parseLaunchData does |
| n/a | `internal/ytcast/dial/dial_test.go` | Go-only tests. Pin SSDP packet shapes, app/device-desc XML, HTTP routing. The full Start/Stop lifecycle test is `t.Skip`-gated because IGMP is often unavailable in CI containers |
| n/a | `internal/ytcast/lounge/doc.go` | Go-only package doc for the lounge protocol layer |
| n/a | `internal/ytcast/lounge/events.go` | Go-only session-level event bus (SessionConnected/Disconnected, MessageReceived, PairingCodeReady/Error, RPCConnectionTerminated) extracted from upstream's per-class EventEmitter usage |
| `src/lib/app/Message.ts` | `internal/ytcast/lounge/message.go` | Regex parser (Go RE2 port of `/\[(\d+),\["(.+?)"(?:,(.*?))?\]\]/g`) plus outgoing message constructors (`NewNowPlaying`, `NewOnStateChange`, `NewOnVolumeChanged`, `NewOnAutoplayModeChanged`, `NewOnHasPreviousNextChanged`, `NewAutoplayUpNext`, `NewLoungeScreenDisconnected`). The nested `Message.X` classes upstream become factory functions |
| `src/lib/app/BindParams.ts` | `internal/ytcast/lounge/bindparams.go` | Mutable query-param builder; AID/RID/SID/gsessionid arithmetic preserved byte-for-byte (off-by-ones here silently desync the protocol). `zx` is 12-hex-char crypto/rand (not uuid-v4 truncation, but indistinguishable on the wire); `RID` is crypto/rand in [41000,49999] |
| `src/lib/app/RPCConnection.ts` | `internal/ytcast/lounge/rpcconnection.go` | HTTP long-poll using stdlib `net/http` + `bufio.Scanner` (1 MiB max line). Auto-reconnect on remote close, MAX_RETRIES=3 on dial failure. Callbacks (`SetOnMessages`/`SetOnTerminate`) replace EventEmitter `messages`/`terminate` |
| `src/lib/app/Session.ts` | `internal/ytcast/lounge/session.go` | Per-sender lifecycle (Begin/End/Restart, lounge-token fetch+refresh, init-session handshake, RPC drive). Uses `asyncq.Queue` for serial outgoing send, retry-once-after-refresh-then-end semantics preserved. `MDXContext` is persisted through the `datastore.DataStore` interface |
| `src/lib/app/Playlist.ts` | `internal/ytcast/lounge/playlist.go` | In-memory queue, autoplay mode, `setPlaylist`/`updatePlaylist` handling. Per-event EventEmitter listeners become a single `PlaylistListener` struct of callbacks; abort-signal flow is `context.Context` |
| `src/lib/app/PlaylistRequestHandler.ts` | `internal/ytcast/lounge/playlistreq.go` | Abstract interface preserved; `BaseHandler` embeddable struct provides SetLogger/Logger/Reset boilerplate. `getPreviousNextVideosAbortable` is a free function over the interface so implementors don't need to embed a base class |
| `src/lib/app/DefaultPlaylistRequestHandler.ts` | `internal/ytcast/lounge/playlistreq_default.go` | **Stubbed** — upstream uses `youtubei.js` (Node-only third-party) for metadata + autoplay neighbours; sonuntius only needs the video id, so the stub returns empty neighbours and logs a debug breadcrumb. Real impl is a Phase 2.1 TODO |
| `src/lib/app/PairingCodeRequestService.ts` | `internal/ytcast/lounge/pairing.go` | 5-minute (30-second when not ready) refresh loop in a goroutine. Listener callbacks replace EventEmitter `request`/`response`/`error`; the response/error also flow into the session-level event bus |
| n/a | `internal/ytcast/lounge/message_test.go` + `bindparams_test.go` | Round-trip parser tests for representative real frames; AID/RID arithmetic tests including 100-iteration progression to catch off-by-ones |

### Phase 3 — orchestrator + wrapper

| Upstream file | Go destination | Notes |
| --- | --- | --- |
| `src/lib/app/YouTubeApp.ts` | `internal/ytcast/youtubeapp.go` | Lifecycle (STOPPED → STARTING → RUNNING → STOPPING), session map (one active session at a time, switching resets player), DIAL launch handler, incoming-message dispatcher, sender connect/disconnect tracking, autoplay mode reconciliation by sender capabilities, mute policy, player state → lounge message fan-out. Event re-emission uses a typed `AppEventBus` instead of `EventEmitter` |
| `src/lib/YouTubeCastReceiver.ts` | `internal/ytcast/receiver.go` | Public-API root. Wires DIAL `OnLaunch` → `YouTubeApp.Launch`, exposes `Start`/`Stop`/`SetLogLevel`/`PairingCodeService`/`ConnectedSenders`. Re-exports `AppEventBus` as `ReceiverBus` so hosts don't need a sub-import |
| `src/lib/Player.ts` (state machine portion) | `internal/ytcast/engine.go` | Go-only support file. The `do*` hooks live in `internal/ytcast/player` (host contract); this file ports the `play`/`pause`/`resume`/`stop`/`seek`/`next`/`previous`/`setVolume`/`reset` wrappers and the `#setStatusAndEmit` state-event broadcaster. CPN is 16 hex chars from `crypto/rand` instead of `uuidv4().substring(0, 16)` (indistinguishable on the wire) |
| `src/index.ts` | `internal/ytcast/exports.go` | Thin re-export — re-aliases `constants.Status`, `UpstreamCommit`, `UpstreamVersion`. Go has no barrel-file convention so this is mostly a Maps-to anchor |
| n/a | `cmd/yt-cast/main.go` | Go-only sonuntius binary. Reads addon options via `internal/config`, derives a stable receiver UUID under `/data/sonuntius/`, dials the ma-bridge IPC socket with backoff, builds the receiver with retry-with-backoff on `Start` failure so the addon container stays healthy even when its dependencies aren't. Logs the pinned upstream commit short SHA on startup |
| n/a | `cmd/yt-cast/player.go` | Go-only sonuntius Player adapter satisfying `internal/ytcast/player.Player`. Translates `DoPlay`/`DoPause`/`DoResume`/`DoStop`/`DoSeek`/`DoSetVolume` into `events.PlayIntent` / `events.TransportCommand` / `events.VolumeCommand` over IPC. Caches the latest `events.PlayerState` for `DoGetVolume`/`DoGetPosition`/`DoGetDuration` so the receiver can answer sender queries without round-tripping to MA. Maps `Client.Theme == "m"` → "ytmusic", `"cl"` → "youtube" |
| n/a | `cmd/yt-cast/options.go` | Go-only option loader. Wraps `internal/config.Load` and adds the receiver UUID + DIAL friendly name + data dir derivations |

Subsequent phases will append to this table.

## Style invariants

- Every `.go` file in this tree opens with a `// Maps to:` header naming the
  upstream source file (or `N/A — Go-only ...` for files with no upstream
  counterpart). Multi-file ports list each upstream file separated by ` + `.
- Stdlib only. The wider `sonuntius` module already depends on
  `golang.org/x/net`, but the foundation layer here does not pull it in.
- Errors are plain `error` values; no `panic` in library code.
- Long-running operations take a `context.Context` as the first argument.
