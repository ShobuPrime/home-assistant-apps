# Changelog

## Version 0.2.6 (2026-05-11)

### Fire-and-forget for idempotent MA commands — every rapid press registers

Real engineering bottleneck the user flagged:

> "I would press the button twice really quickly and I have to wonder
> if our existing integration just physically can't handle that speed
> due to the number of hops we have to make."

Yes — there was a real bottleneck. Each volume/transport command was
going through `WSClient.Send`, which:

1. Generates a message_id
2. Registers a response channel in the pending map
3. Sends the WS frame
4. **Waits up to 15 s for MA's matching response** before returning

Meanwhile, the IPC reader in ma-bridge processes events *serially*.
A volume press from yt-cast lands in IPC, the reader pulls it,
calls the dispatcher, the dispatcher calls `WSClient.Send` — which
**blocks for the 20-50 ms WS round-trip**. The next volume press
sits in the IPC buffer until the first one finishes. Two rapid
presses meant ~100 ms before the second even reached MA. Three or
four rapid presses meant some got coalesced or dropped by MA when
they finally arrived as a burst.

**Fix:** new `WSClient.SendFireAndForget(ctx, command, args)` —
writes the WS frame and returns immediately, no pending-map entry,
no response wait. Commands that don't need a response use it:

- `players/cmd/volume_set`
- `players/cmd/volume_mute`
- `players/cmd/play` / `pause` / `stop` / `next` / `previous`

Per-press latency drops from ~25 ms (round-trip) to <1 ms (write).
Two rapid presses each fire-and-forget independently — no
serialisation, no queueing, no coalescing.

`internal/ma/wsclient.go`: new SendFireAndForget plus a refactor
of all idempotent transport / volume / mute methods to use it.
`PlayQueueMedia`, `ClearQueue`, `Seek`, `AddToQueueMedia` keep
using `Send` (we use their responses for error handling).

### What this doesn't change

- `play_media`, `seek`, `clear_queue` still wait for responses. They
  carry meaningful errors (auth required, queue not found) and the
  cast-start flow needs to know whether to fall back to HA REST.
  The cost is one round-trip per cast — not per press.
- HA REST fallback path is unchanged — still synchronous.

## Version 0.2.5 (2026-05-11)

### Suppress HA-WS state when MA-WS has spoken — fixes the paused → idle → reset-to-0 flip

v0.2.4 fixed `player_updated` from MA but left the **HA core WS state
watcher** still broadcasting state with HA's view — which mirrors
MA's `Player.state` (the speaker-on/off flag), reporting `idle` for
a paused queue.

Log timing made it obvious:

    23:15:54.439  queue_updated state=paused        ← MA WS, correct
    23:15:54.440  Pushing status=2 (paused)         ✓
    23:15:54.457  Pushing status=-1 (idle)          ← HA-WS pushed idle 17 ms later
    23:15:54.560  queue_updated state=paused        ← MA reasserts
    23:15:54.560  Pushing status=2                  ✓ again
    23:15:54.580  Pushing status=-1                 ← HA flips it back

The engine flickered between paused and idle every queue tick.
Whichever value the phone latched onto last (often `idle`) decided
whether the next resume preserved position or kicked off a fresh
play_now from 0.

`internal/events/events.go`: new `PlayerState.Source` field
(`"ma-ws"` / `"ha-ws"`) so receivers can prefer the more
authoritative feed.

`cmd/ma-bridge/main.go`: tags MA-WS broadcasts with `Source=ma-ws`.

`internal/state/watcher.go`: tags HA-core-WS broadcasts with
`Source=ha-ws`.

`cmd/yt-cast/player.go::updateCachedState`: when an MA-WS state
event has arrived in the last 15 s, drop the `State` field from
incoming HA-WS events. Everything else (position, title, duration,
volume) is still merged in — HA-WS remains useful for those, and
becomes the full fallback again when MA-WS is silent for >15 s.

Volume responsiveness from v0.2.4 is unchanged and continues to
work correctly (the live log confirmed it).

## Version 0.2.4 (2026-05-11)

### Volume — DoGetVolume returns user intent during input window; player_updated stops driving state

Two cause-and-effect bugs from the v0.2.3 live log.

**1. Phone volume slider stuck.** v0.2.3 stopped firing
`onStateChange` for volume-only updates, but the engine has its own
periodic state push triggers (status transitions, queue ticks)
that ignore that gate. On every such push it calls our
`DoGetVolume`, which returned cachedState.Volume — overwritten by
MA's lagging echo. Phone slider kept snapping back to MA's stale
value.

`cmd/yt-cast/player.go::DoGetVolume`: while in the active-input
window (DoSetVolume in the last 2 s) return `lastUserVolume` — the
value the user just requested. After the window expires, return
cachedState.Volume so MA-side changes (UI in MA, physical buttons)
flow back to the phone normally.

`DoSetVolume` records the user's intent in
`lastUserVolume` + `hasUserVolume`. Engine-driven state pushes
now report the user's slider position during input, MA's value
afterwards.

**2. Pause → cast ended → resume restarts at 0.** Log:

    22:57:35.336  Player.pause()                      ← user pressed pause
    22:57:35.337  Pushing status=2 (paused)           ✓
    22:57:35.596  player_updated state=idle           ← MA's Player goes idle on pause
    22:57:35.601  Pushing status=-1 (idle)            ← phone now thinks cast ended
    22:57:36.095  queue_updated state=paused          ← correct, but too late
    22:57:41.115  Player.play() @ 0s                  ← next resume = new cast from 0

MA's `Player.state` reflects speaker-on/off, not queue
pause/play. When the queue is paused, the Player goes "idle" (no
audio flowing). v0.2.3 was treating that as PlayerStatusIdle and
pushing status=-1 to the phone — at which point the YouTube cast
app treats the next resume as a fresh play_now from position 0,
losing the scrubbed position.

`internal/ma/client.go::PlayerStateFromPlayerEvent`: now leaves
`State` empty entirely. Pause / playing / buffering transitions
come from `queue_updated` only (which correctly reports
`state=paused`). The merge logic in `updateCachedState` preserves
the previous state when player_updated brings only volume / muted
info. The status=-1 spurious push is gone.

This also fixes the user's separate "scrub in MA, pause+play from
phone, restarts at 0" report — same root cause.

## Version 0.2.3 (2026-05-11)

### Pause via NotifyExternalStatus + idle/active → paused mapping + volume race fix

Three concrete fixes for the v0.2.2 live log.

**1. Pause-from-MA never reached the engine's state machine.** v0.2.2
broadcasted `state=paused` to the IPC bus, but the only path back to
the phone was `Receiver.EmitPlayerState` — which re-emits whatever
the *engine* thinks the status is. The engine still had
`PlayerStatusPlaying` (last set when our DoPlay landed); MA's pause
event never propagated.

`internal/ytcast/receiver.go`: new `Receiver.NotifyExternalStatus(ctx,
PlayerStatus)` that calls into the engine's
`NotifyExternalStateChange` — actually *setting* the engine's
internal status and emitting to all senders.

`cmd/yt-cast/main.go`: new `mapMAStateToPlayerStatus(string)` and
`adapt.setOnStateChange` now calls `NotifyExternalStatus` (rather
than `EmitPlayerState`) when the cached state has a known status
mapping.

**2. MA's Universal Player + Sendspin reports "pause" as
`state=idle`.** The live log confirmed: the queue stays
`active=true` with a `current_item`, but `state` shows `idle`
instead of `paused`. The phone naturally interprets `idle` as
"stopped", not "paused".

`internal/ma/client.go::PlayerStateFromQueueEvent`: when MA reports
`state=idle` AND `active=true` AND `current_item != nil`, translate
to `paused`. A truly-stopped queue clears `active` or `current_item`.

**3. Volume — push MA changes to phone, but pause the push during
user-driven input.** The user clarified the desired model:

> "the volume level on the speaker is the source of truth — and
> Music Assistant in my experience is pretty good at remembering it,
> so I think for volume we should get rid of the caching entirely
> and on fresh connection we sync the volume state from MA to the
> phone app"
>
> "why can't we also do the same for speaker volume if I update it
> directly in the MA UI?"

Both true: bidirectional during a session, with no fighting on the
phone-input path.

`cmd/yt-cast/player.go::DoSetVolume`: stops writing to cachedState
(no optimistic echo). Just stamps `lastVolumeSentAt` and forwards.

`cmd/yt-cast/player.go::updateCachedState`: split firing decision —
non-volume changes (state, title, position) **always** fire so
pause-from-MA reaches the phone immediately. Volume changes fire
ONLY when no DoSetVolume happened in the last 2 s. During the
window, MA echoes our recent commands and pushing them back would
snap the slider; after the window, volume from MA's UI (or physical
speaker buttons) propagates to the phone normally.

Result: rapid press-and-hold on phone → no snap-back. Pause in MA UI
→ phone reflects immediately. Volume change in MA UI → phone slider
follows (after ~2 s of input quiet).

### Deferred to v0.2.4 / future

- **Queue mirroring still a no-op for playlists.** The engine's
  `Queue.GetState().Next` and `.Autoplay` remain nil for casts
  even from a YouTube playlist. The engine port's playlist
  tracking likely needs deeper work — separate effort.

## Version 0.2.2 (2026-05-11)

### Real bidirectional sync — handle MA's queue events; merge state updates; optimistic volume echo

Three concrete bugs caught in the v0.2.1 live log.

**1. MA's pause state arrives on `queue_updated`, not `player_updated`.**
v0.2.1 wired `OnEvent` but only handled `player_updated`. MA's
PlayerQueue object (the authoritative source for
playing/paused/idle/buffering transitions) is broadcast on
`queue_updated` and `queue_time_updated`. The log shows dozens of
those events being ignored:

    ma ws event: ignoring component=ma-ws-events event=queue_updated
    ma ws event: ignoring component=ma-ws-events event=queue_time_updated

When the user paused inside MA, MA fired `queue_updated` with
`state=paused` inside the PlayerQueue dict. We dropped it on the
floor, so the phone never saw pause.

Fix: new `PlayerStateFromQueueEvent` decoder in
`internal/ma/client.go` for the `queue_updated` /
`queue_time_updated` payload shape (different from Player).
`cmd/ma-bridge/main.go` dispatches by event name: queue events to
the queue decoder, player events to the player decoder.

**2. `player_updated` was clobbering title/track_id with empty strings.**
Log:

    ma ws event: broadcasting PlayerState event=player_updated state=playing title="" track_id=""
    yt-cast: cachedState updated state=playing title="" artist="" track_id=""

Then the HA core WS state watcher would emit a *full* snapshot a
few ms later with the right title — but the engine had already
pushed an empty title to the phone in that window.

Fix: `updateCachedState` now **merges** the incoming PlayerState
into the previous cached value instead of replacing wholesale.
Empty strings / nil pointers preserve the previous field; only
non-empty fields overwrite. MA's partial events (volume-only,
state-only) leave title/artist/track_id intact.

**3. Volume oscillation persisted despite v0.2.1's input-window suppression.**
The window suppressed updates to cachedState.Volume, but the
*pre-existing* cachedState.Volume was MA's stale echo (e.g. 0.55
while user wanted 0.58), and that stale value kept getting pushed
back to the phone — which then snapped the slider to 55, computed
55+3=58 on the next press, and entered a steady-state oscillation.

Fix: `DoSetVolume` now writes the user-requested value into
`cachedState.Volume` immediately (optimistic echo) so the engine's
next state push to the phone reports the user's intent. The
existing suppression keeps MA's lagging echoes from overriding it
during the active-input window. After the window expires, MA's
echo resyncs us to the actual speaker level.

This re-introduces optimistic echo, BUT only for raw values (no
rounding). The v0.1.14 feedback loop required rounding to be
present (round-and-echo snapped to a bucket, phone got snapped
back inside that bucket). With raw values, echoing what the user
sent doesn't create a feedback loop — the phone's next computation
is `current + step`, not `bucket + offset`.

## Version 0.2.1 (2026-05-11)

### Close the bi-directional sync loop + suppress volume-echo races

Three issues from the v0.2.0 live test.

**1. Pause-in-MA didn't reach the phone.** The user paused playback
inside MA's UI; the YouTube app continued showing the timeline
ticking with a Play icon. Cause: v0.2.0 sent commands via MA WS but
state events still flowed back through HA's core WS state.Watcher,
and HA's MA integration silently drops/coalesces MA-internal state
transitions. The 60-min log search returned **zero** `state=paused`
broadcasts even though the user paused multiple times.

`cmd/ma-bridge/main.go`: the long-lived `WSClient`'s `OnEvent`
handler now decodes MA's `player_updated` (and
`player_queue_time_updated`, `player_added`) events directly and
broadcasts a `PlayerState` over the IPC bus. v0.2.0 passed `OnEvent:
nil` — MA's events were arriving and being silently dropped.

`internal/ma/client.go`: extracted `PlayerStateFromMAEvent(raw)` —
the same translation logic the deprecated `Watcher.translateEvent`
used, now reusable from the bridge.

HA's core-WS state.Watcher remains active as a redundant feed —
useful as a fallback when MA WS drops, and for the few attributes
HA aggregates that MA's player_updated doesn't carry.

**2. Rapid volume presses lose increments.** Live log showed phone
oscillating 49 → 52 → 49 → 52 even when the user pressed "up up up".
MA's volume state events race the phone's next press and snap the
slider back into a previous bucket.

`cmd/yt-cast/player.go`: new `lastVolumeSentAt` timestamp + a
2-second `volumeInputWindow`. While the window is active,
`updateCachedState` preserves the previous `Volume`/`Muted` from
cachedState — so the engine doesn't push `onVolumeChanged` to the
phone using a stale MA echo. After 2 s of input quiet, the next
state event resyncs the slider to the speaker's actual setting.

**3. Queue preload often a no-op.** Log shows
`no upcoming video to preload` for single-video casts because
`Queue.GetState().Next` and `.Autoplay` are both nil — the YouTube
cast app didn't supply an up-next. Not a bug; correct behaviour.
Elevated the log line from debug to info so users can correlate
"queue not added to MA" reports with this reason.

A deeper fix (synthesise a candidate via YouTube's "related videos"
when the cast app doesn't supply one) needs engine-port investigation
and is deferred.

## Version 0.2.0 (2026-05-11)

### Direct MA WebSocket — bypass HA REST for all MA-bound traffic

Major refactor of the command path. ma-bridge now opens a single
long-lived WebSocket to Music Assistant and routes play_media, seek,
transport (play/pause/stop/next/previous), volume, mute, and queue
clear directly to MA. HA REST remains as a fallback when the MA WS
is unavailable.

Motivating bug: v0.1.18 logs showed a `media_seek` via HA REST
taking ~3.4 seconds to return because HA waits for MA's Python
integration to round-trip. Casting at 34 minutes in started the
speaker at 0:00 because the seek arrived seconds after playback
had begun. With the new direct WS path, the seek-after-play
issued on the same connection lands while MA is still loading the
queue item — observed cast-start latencies in the few-hundred-ms
range.

`internal/ma/wsclient.go` (new):
- `WSClient` — long-lived MA WS with auth handshake, reconnect
  with exponential backoff, command/response correlation by
  `message_id`, and event-push fanout to a registered handler.
- Methods: `PlayQueueMedia`, `AddToQueueMedia`, `ClearQueue`,
  `Seek`, `PlayerPlay`, `PlayerPause`, `PlayerStop`, `PlayerNext`,
  `PlayerPrevious`, `SetVolume`, `SetMute`.
- `Send(command, args)` is concurrency-safe; in-flight callers
  receive `ErrDisconnected` if the connection drops.

`internal/dispatcher/dispatcher.go`:
- `SetMAWS(*WSClient, queueID)` replaces the old `SetMAWS(url,
  token, queueID)`.
- `playViaMAWS` now does ClearQueue → PlayQueueMedia → Seek over
  the same connection (no 500 ms HA-REST delay).
- `dispatchTransport` and `dispatchVolume` prefer the WS path and
  fall back to HA REST on failure.
- New `clear_queue` transport command routes to `WSClient.ClearQueue`.

`cmd/ma-bridge/main.go`:
- Constructs and `Start()`s the `WSClient` after `resolveMAQueueID`.
- Removed the redundant `ma.Watcher` connect-probe — the WSClient's
  own connect logs surface schema / server-version / auth state.
- `maClient.Stop()` on shutdown.

`cmd/yt-cast/player.go::resetSession`:
- Now also emits a `clear_queue` transport command so MA's queue
  is wiped when every sender disconnects — preserving the "clean
  slate for the next sender" invariant from v0.1.17 with a
  matching action on the MA side.

### Performance / hops

Before:

    yt-cast → IPC → ma-bridge → HA REST → HA → MA → speaker     (6 hops)
    media_seek over HA REST: ~3.4 s observed

After:

    yt-cast → IPC → ma-bridge → MA WS → speaker                 (4 hops)
    play_queues/seek over MA WS: ~50 ms typical

For volume/transport during steady-state casting, the MA WS path
replaces the HA REST roundtrip entirely — measurably more
responsive on rapid press-and-hold.

### Sliding +1 queue preload

After a successful DoPlay, the adapter spawns a background goroutine
that:

1. Asks the engine for the upcoming video (`receiver.UpcomingVideo`,
   which returns `Queue.GetState().Next` or `.Autoplay`).
2. Resolves its googlevideo stream URL via yt-dlp (1-5 s).
3. Resolves title/channel/thumbnail via oEmbed.
4. Dispatches a new `events.QueueAddIntent` event over IPC.

The dispatcher consumes `QueueAddIntent` and calls
`WSClient.AddToQueueMedia` — MA's queue then contains
`[current, next]`, so when the current item ends MA auto-advances to
the YouTube-app-supplied next instead of falling back to MA's
library autoplay.

A monotonic `preloadGen` counter aborts in-flight preloads when a
newer cast lands (rapid skip-next): the goroutine drops the result
instead of corrupting the freshly-rebuilt queue.

Scope: only `+1` (next) for now. The `-1` (previous) mirroring is a
small future addition — same plumbing, accessor for
`Queue.GetState().Previous`.

`internal/events/events.go`: new `QueueAddIntent` event type.
`internal/dispatcher/dispatcher.go`: new `dispatchQueueAdd` path
plus a shared `buildMediaItem` helper (deduped from `playViaMAWS`).
`internal/ytcast/{youtubeapp,receiver}.go`: new `UpcomingVideo()`
method on the App and Receiver.
`cmd/yt-cast/{player,main}.go`: `setPeekNextVideo` hook,
`preloadUpcoming` goroutine.

### Future work (deferred from this PR)

- **`-1` previous mirroring**: when the user presses "previous"
  on the speaker the prior track is already in MA's queue, no
  re-resolve needed. Same plumbing as `+1`.
- **Dead-code cleanup** of the obsolete `ma.PlayMediaItem`,
  `ma.ClearQueue`, and `ma.Watcher` types now superseded by
  `WSClient`. Mechanical follow-up.

## Version 0.1.18 (2026-05-11)

### Volume passthrough + revert play_media option to "play" (with explicit queue clear)

**1. Strip volume calculations — pass raw value straight to MA.** Per
user direction. Earlier versions tried rounding, delta tracking, and
optimistic echo; every variation introduced a different artifact
(feedback loop in v0.1.14, stuttering ref in v0.1.17 because MA's
state events overwrote the delta-tracking reference). The simplest
behavior — forward the cast sender's raw value untouched — leaves
the cast UI authoritative over its own slider and MA authoritative
over the speaker. No more "press multiple times for it to feel
natural".

Removed: `volumeStep` adapter field, `setVolumeStep`,
`computeVolumeOutput`, `roundToStep`, `volume_step` addon option,
`Options.VolumeStep` / `EffectiveVolumeStep`,
`cmd/yt-cast/volume_test.go`.

**2. `player_queues/play_media` option reverted to `"play"`.** v0.1.14
switched to `"replace"` to keep MA's queue clean per cast, but the
user reported v0.1.17 regression: casting at a non-zero start
position (scrubbed to 7:28 then cast) started the speaker at 0:00.
`"replace"`'s full queue reset was racing the subsequent
`media_seek` and dropping it.

To preserve the clean-queue behavior without `"replace"`, the
dispatcher now sends `player_queues/clear` explicitly before
`play_media`. Two WS round-trips instead of one, but it's a few
extra ms on the cast-start path and avoids the seek race.

New `ma.ClearQueue(ctx, url, token, queueID, log)` in
`internal/ma/client.go`. `dispatcher.dispatchPlay` calls it on
the MA-WS path; on the HA-REST fallback path the queue clear is
skipped (HA's `media_player.play_media` doesn't have an equivalent).

### Future work (still tracked)

- Direct MA WS for volume / transport (the major refactor): cuts
  the 6-hop volume path to 4 hops, ~30-40 ms saved per command.
  Requires long-lived MA WS connection in ma-bridge, fallback to
  HA REST when MA unreachable, reconnect/retry. Significant
  enough to deserve its own PR.
- Queue mirroring from YouTube cast app: needs engine hooks
  into `onAutoplayUpNext` / playlist events, `option: "add"`
  per upcoming video, and reverse "remove from queue"
  translation.

## Version 0.1.17 (2026-05-11)

### Session reset on sender disconnect

When every sender (Cast app on phone) has disconnected, the adapter
now wipes per-cast state — track title, artist, duration, position,
and the local wall-clock estimator. Without this, a second device
connecting after the first briefly saw the previous session's track
info in its first state push, before its own DoPlay landed and
replaced the cache.

Speaker-scoped state (volume, muted) is intentionally preserved —
the physical speaker keeps its setting regardless of which sender
is connected, and surfacing that to a fresh sender is the right
initial value.

New `adapter.resetSession()` and a `watchSenderLifecycle` goroutine
in `cmd/yt-cast/main.go` that subscribes to the receiver's app-event
bus (`SenderConnectedEvent` / `SenderDisconnectedEvent`) and calls
the reset when the connected-sender count drops to zero.

### Performance discussion

The user raised goroutines + mutexes for ordering. Re-evaluated:

- Cast is **single-sender at a time**, so within one stream events
  are naturally serial. A per-event-type mutex doesn't enable
  parallelism — it just adds bookkeeping.
- The **real** single-sender win is cutting the hop count. Today
  volume commands traverse 6 hops (yt-cast → IPC → ma-bridge → HA
  REST → HA → MA → speaker). Routing volume/transport over the
  existing MA WebSocket (already used for play_media) would drop
  to 4 hops and shave 30-40 ms per command. Tracked as a follow-up
  PR — it's a meaningful refactor (long-lived WS connection in
  ma-bridge, fallback path for when MA is unreachable).
- `encoding/json` is the stdlib and our policy is stdlib-only;
  `easyjson`/`sonic` would be 2-5× faster but require third-party
  deps. For our <1KB messages the absolute saving is microseconds —
  not worth the dep.
- `sync.Pool` for IPC scanner buffers is a microsecond-scale win;
  also tracked for follow-up.

## Version 0.1.16 (2026-05-11)

### Volume delta routing + honour sender start position

**1. Volume feedback loop fixed.** With `volume_step = 10`, the
v0.1.14 optimistic echo created a feedback loop: phone sends 47 →
we round to 50 and echo 50 back → phone slider snaps to 50 → user
presses down → phone calculates 50 − 3 = 47 → sends 47 → we round
to 50 again. The user was visibly stuck at 50 even though the phone
log showed many "down" presses. (The user's debug-log inspection
caught this: `Level:47` repeated indefinitely.)

`cmd/yt-cast/player.go::DoSetVolume`: rewritten to use **delta-based
routing**. We compare the incoming raw to the value we last echoed
(stored in `cachedState.Volume`):

  - `|delta| >= step`: slider drag — snap to round(raw, step).
  - `|delta| < step` and `delta > 0`: tap up — bump output by +step.
  - `|delta| < step` and `delta < 0`: tap down — bump output by -step.
  - `delta == 0`: keep output.

Result: every distinct raw value from the phone produces exactly
one step delta on MA, regardless of how the phone's slider step
relates to our rounding step. New `computeVolumeOutput` helper,
new `volume_test.go` covering 18 cases.

**2. Honour the sender's start position.** Casting at 7:28 from the
YouTube app would still start playback at 0:00 on the speaker. The
PlayIntent didn't carry the position, and MA's `play_media` doesn't
accept a start-offset arg. Fix: thread the sender-supplied position
through `PlayIntent.StartPosition` and follow `play_media` with a
delayed `media_seek` from the dispatcher. The 500 ms delay lets MA
ingest the play_media before the seek lands, avoiding a race where
MA drops the seek silently.

`internal/events/events.go`: new `StartPosition float64` field.
`cmd/yt-cast/player.go::DoPlay`: writes `intent.StartPosition`.
`internal/dispatcher/dispatcher.go`: new `maybeSeekAfterPlay` helper
running on both the WS and HA-REST play paths.

### On the parallel-architecture / GPU questions

- **Goroutines / concurrency:** the bridge already runs each
  long-lived component (yt-cast receiver, cast-receiver, ma-bridge
  IPC server, state watcher, MA WS probe) on its own goroutine.
  Within ma-bridge, `dispatcher.Dispatch` is synchronous on the IPC
  read goroutine, but HA REST calls return in ~20 ms (per live log)
  while phone events arrive at ~2–5 Hz — there's slack, not
  back-pressure. Sprinkling goroutines per Dispatch would change
  ordering guarantees (a later "volume down" could overtake an
  earlier "volume up") without measurable throughput gains.
- **GPU acceleration on Pi CM5:** the workload is JSON, HTTP REST,
  WebSocket frames, protobuf framing, mDNS/SSDP. Zero matrix
  arithmetic and no encode/decode happening in-process (yt-dlp/
  ffmpeg run elsewhere and only use the dedicated H.264/H.265
  hardware blocks, not the VideoCore VII GPU). There is nothing
  to offload.

### On the queue-mirroring question

Deferred to a separate effort. Mirroring the YouTube cast app's
up-next list into MA's queue requires hooking the engine's
`onAutoplayUpNext` / playlist-modified events, sending follow-up
`player_queues/play_media` with `option: "add"` per upcoming
video, clearing MA's queue on sender disconnect, and translating
YouTube "remove from queue" back to MA. Each one is small but
the lifecycle handling is delicate enough to warrant its own PR.

## Version 0.1.15 (2026-05-11)

### Position bounds + cast-start state seeding

Two issues from the v0.1.14 live test.

**1. Phone timeline shows `3:44 / 1:27` after track ends.** When MA
reaches the end of the queued item it stops emitting
`media_position` events; HA's state_changed events arrive with no
`media_position` attribute, so our cached `Position` goes nil and
DoGetPosition falls back to the local wall-clock estimator. The
estimator was happily ticking past the end of the video forever —
1m9s after the 87-second track ended, it was reporting 154s, and
by the time the user looked at the phone it had reached 224s
(`3:44`).

Two changes:

- `localEstimateLocked` caps the returned position at the known
  duration. The estimator is a fallback for the early-playback
  gap, not a continuous reference — it should never exceed the
  track length.
- `updateCachedState` clears the estimator (sets
  `playbackStartedAt = nil`) when it sees a transition from an
  active state (`playing`, `paused`, `buffering`, `loading`) into
  an ended state (`idle`, `stopped`, `off`, `unavailable`, empty).
  Helpers `isTrackEndedState` and `isActivePlaybackState` keep
  the rule explicit.

**2. Initial cast shows placeholders / stale data.** Between
`DoPlay` invoking and MA's first state_changed arriving for the
new track (~1s gap), the engine pushes the cached PlayerState to
the phone. That cached state was leftover from the *previous*
cast — wrong title, wrong duration, wrong position — so the
phone briefly rendered "65s / 87s" on a freshly-cast video.

`DoPlay` now replaces the cached state's track-scoped fields
(state="buffering", title, artist, trackID, position=sender-
supplied, duration=yt-dlp value) immediately after seeding the
local estimator, and fires `onStateChange` so the engine pushes
the new info to the phone right away. Speaker-scoped fields
(volume, muted) are preserved across casts.

## Version 0.1.14 (2026-05-11)

### Volume bidirectional sync + queue replace

Two refinements suggested by the user after v0.1.13 worked end-to-end.

**1. Volume bidirectional sync (optimistic echo).** With `volume_step`
quantisation, the phone would drag the slider to e.g. 23 but the
speaker would land on 20. The slider stayed at the raw drag value
because the new MA state took several hundred ms to round-trip back
(HA REST → MA → state_changed → IPC → engine → Lounge → phone).
Subsequent presses arrived while the phone still thought the volume
was 23, so the user's "feel" lagged.

`cmd/yt-cast/player.go::DoSetVolume`: after computing the rounded
value, we now immediately write it into the adapter's
`cachedState.Volume`/`Muted` and fire `onStateChange`. The engine
re-emits player state to the Lounge sender right away, so the phone
slider snaps to the bucket boundary the moment the user lifts their
finger. MA's later state_changed reconciles silently.

**2. `player_queues/play_media` option flipped to `replace`.** MA's
WS API accepts an `option` field on play_media. We had `"play"`
which only replaces the current item — the rest of the queue
remains. With MA's library having tracks queued up (favourites,
prior casts), our cast would end and MA would happily auto-advance
to the next stale item. The user observed this as "automatically
goes to the next item in MA, not whatever YouTube wants".

`internal/ma/client.go::PlayMediaItem`: switched to
`"option": "replace"` which clears the entire queue before adding
our item. When playback finishes, the queue is empty and MA stops.

### Future work (not in this release)

Mirroring the YouTube cast app's own queue into MA (so adding videos
to YouTube's up-next list flows into MA's queue too) requires
hooking the engine's queue-modified events and sending follow-up
`player_queues/play_media` with `option: "add"`. Tracked as a
separate effort because it needs careful sender-disconnect
lifecycle handling to avoid orphaned MA queue items.

## Version 0.1.13 (2026-05-11)

### MA queue skip fix — revert item_id to the URL

v0.1.12's MediaItem reached MA correctly: the queue title was
composed from our `artists[].name` + `name` ("Dads MMO Lab -
Offline RuneScape Install Guide…") so the metadata path was sound.
But MA then could not stream it, logging:

    WARNING [music_assistant.streams.audio] Unable to retrieve info
      for yt_tCW5iRYXnBc (No such file or directory)
    WARNING [music_assistant.player_queues] Skipping unplayable item
      Dads MMO Lab - Offline RuneScape Install Guide...

The synthetic `item_id` (`yt_<videoId>`) made MA's builtin provider's
`get_stream_details` treat the id as a filesystem path. The item
was marked unplayable and the queue auto-advanced to the next track
— exactly what the user observed ("when it tried to resolve the
YouTube video it fails and just automatically tries to play the
next video in the queue").

**Fix**: set both `MediaItem.item_id` and
`ProviderMappings[0].item_id` to the resolved stream URL. MA's
builtin provider goes down its URL/ffmpeg-probe path which
succeeds for a real audio stream. The explicit `name` + `artists`
fields on the MediaItem dict still survive — MA's
`QueueItem.from_media_item` uses our dict's display fields and
ffmpeg metadata only supplements the audio-format streamdetails.

`internal/dispatcher/dispatcher.go::playViaMAWS`: `itemID := uri`,
removed the unused `shortHash` helper.

## Version 0.1.12 (2026-05-11)

### Real metadata fix, fully responsive volume, position-drift guard

Three issues from the v0.1.11 live test.

**1. MA UI still showed the raw URL as the title.** v0.1.11's MA-WS
play_media call succeeded (no more `error_code: 10`) but the MA UI
still rendered `videoplayback?expire=…` instead of the song name —
because the MediaItem we sent had two structural bugs that caused
MA to silently drop our metadata and re-resolve the URL via
ffmpeg-probe:

  - `item_id` was the googlevideo HTTP URL. MA's builtin provider
    treats URL-shaped item_ids as something to probe.
  - `artists` was a `[]string`; MA's deserializer requires a list of
    full Artist objects and silently drops the string form.
  - Image lived under top-level `image`; MA reads from
    `metadata.images[]`.

`internal/ma/client.go`: redesigned `MediaItem` with a synthetic
non-URL `item_id` (`yt_<video_id>`), full `Artist` dicts under
`Artists`, `Metadata.Images[]` with `RemotelyAccessible: true`,
`ProviderMappings[0].URL` carrying the actual stream URL (so the
streams pipeline uses it directly, never calling `parse_item`), and
`Available: true` + `IsPlayable: true` to pass the play_media
availability filter. Added optional `AudioFormat` derived from the
URL `mime=` parameter (webm/m4a).

`internal/dispatcher/dispatcher.go::playViaMAWS`: builds the new
shape; new helpers `guessAudioContentType`, `slugifyChannel`,
`shortHash`.

`cmd/yt-cast/streamresolve.go`: yt-dlp call now also captures the
video duration (`--print "%(duration)s"`) and returns a
`streamInfo{URL, Duration}`. The duration rides through
`PlayIntent.Metadata["duration"]` into the MA MediaItem's
`duration` field so the MA UI shows a proper progress bar from
the moment play_media lands.

**2. "Just make all my button presses work."** v0.1.11's volume
dedup was eating press-and-hold streams from the Android phone:
when the YouTube cast app sends a rapid burst of values during a
hold, all rounded to the same bucket → all dropped. Result: volume
felt frozen during a hold. Removed dedup entirely — every cast
event is rounded and forwarded to MA. Repeated identical
`volume_set` commands are idempotent on MA's end.

`cmd/yt-cast/player.go::DoSetVolume`: dedup logic + state fields
gone.

Also reverted `volume_step` default from 10 → 5 (which was v0.1.10's
default and felt right to the user). The user can still set 10 for
coarser quantisation, or 1 to disable rounding entirely.

**3. Phone progress bar snapping to 0:00 mid-playback.** v0.1.11
logs showed `state: broadcasting state=playing position=0.00` for
state events where MA's media_position attribute was 0, even
though playback was in second 900-ish. The cachedState
overwriting the local estimator with 0 produced the
`drift_seconds=-935` lines and made the phone's UI jump to 0 every
few seconds. We now treat `media_position=0` as "no fresh
position" when the state is `playing` and don't overwrite the
estimator.

`internal/state/watcher.go::playerStateFrom`: only set
`ps.Position` when the value is > 0 or the state is not `playing`.

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
