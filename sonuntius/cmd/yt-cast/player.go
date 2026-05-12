// Maps to: N/A — Go-only sonuntius Player adapter.
//
// The adapter satisfies internal/ytcast/player.Player (the do-only host
// interface). Its job is to translate every Do* call into a sonuntius
// IPC event the existing ma-bridge can dispatch to Music Assistant,
// and to surface the latest PlayerState frames received over IPC back
// to the receiver as cached volume / position / duration values.
//
// Provider mapping (DoPlay):
//
//   - Video.Client.Theme == "m"  → ytmusic://track/<id>
//                                  (Music Assistant's YouTube Music
//                                  native provider).
//   - Video.Client.Theme == "cl" → https://www.youtube.com/watch?v=<id>
//                                  emitted with provider="url" so the
//                                  dispatcher feeds it to MA's stream
//                                  extractor (yt-dlp), which handles
//                                  arbitrary YouTube watch URLs.
//
// The dispatcher in internal/dispatcher accepts provider="url" by
// forwarding URL straight into media_player.play_media as the
// media_content_id, so no dispatcher change is needed for the
// YouTube-classic path.
package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/shobuprime/sonuntius/internal/events"
	"github.com/shobuprime/sonuntius/internal/ipc"
	pkgplayer "github.com/shobuprime/sonuntius/internal/ytcast/player"
	"github.com/shobuprime/sonuntius/internal/ytcast/types"
)

// adapter is the live Player implementation. It is safe for concurrent
// use; mu guards the cached state + the IPC writer.
type adapter struct {
	mu sync.Mutex

	// ipcClient is the persistent connection to the ma-bridge broker.
	// Reset (set to nil + replaced) whenever the broker goes away.
	ipcClient *ipc.Client

	// cachedState holds the latest PlayerState frame we received from
	// the IPC broker. DoGetVolume / DoGetPosition / DoGetDuration read
	// from here so the receiver can answer sender queries without
	// blocking on a round-trip to MA.
	cachedState events.PlayerState

	// source is the "originating receiver" label attached to every
	// outgoing event. Always "yt-cast" for this adapter.
	source string

	// metadata resolves human-readable titles via YouTube's oEmbed
	// endpoint. May be nil — DoPlay falls back to logging just the ID.
	metadata *metadataResolver

	// stream pre-resolves YouTube watch URLs to direct audio stream
	// URLs via yt-dlp, because Music Assistant's URL provider cannot
	// ffmpeg-probe a YouTube watch URL directly. May be nil — DoPlay
	// then emits the bare watch URL and lets MA log the failure.
	stream *streamResolver

	// log is the slog logger used for the async title-resolution
	// callback. May be nil; falls back to slog.Default().
	log *slog.Logger

	// onStateChange fires after every updateCachedState call so the
	// host can ask the engine to re-emit player state — this is how
	// external position / volume / duration updates from Music
	// Assistant flow back to the phone as Lounge messages. May be nil.
	onStateChange func(context.Context)

	// peekNextVideo returns the engine's "what plays next" guess, or
	// nil if no neighbour. Used by DoPlay to pre-load the upcoming
	// track into MA's queue so MA auto-advances to our pick instead
	// of its library autoplay. Set by main.go which has the
	// receiver/engine handle.
	peekNextVideo func() *types.Video

	// preloadGen counts cast events. When DoPlay starts a background
	// preload it captures the current generation; the goroutine
	// aborts if the generation has advanced (a newer cast started),
	// so we never insert a stale +1 into the freshly-rebuilt queue.
	preloadGen uint64

	// lastVolumeSentAt is when we last forwarded a DoSetVolume to MA.
	// updateCachedState uses this to suppress MA's own volume echoes
	// during an active user-input window: while the user is rapidly
	// pressing volume buttons, MA's state events lag the phone's
	// next press and snap the slider back into a previous bucket
	// (the "press up up up only one increment registers" complaint).
	lastVolumeSentAt time.Time

	// Local position tracking. HA's state_changed events for the MA
	// player carry an accurate media_position, but they only fire after
	// MA actually starts streaming (typically 2–10 s after our
	// play_media call) and not at all when the user pauses immediately
	// after pressing play. To prevent the phone's progress bar from
	// snapping to 0:00 in that gap we keep a wall-clock estimate
	// here and only fall back to it when cachedState has no position.
	playbackBasePos    float64    // position passed to DoPlay (sender-supplied)
	playbackStartedAt  *time.Time // when playback began (or resumed)
	playbackPaused     bool       // true between DoPause and DoResume
	playbackPauseStart *time.Time // when the current pause began
	playbackPauseAccum time.Duration // total time spent paused this session
}

// newAdapter constructs an adapter that emits events with source =
// "yt-cast". Pass a connected ipc.Client; the adapter doesn't dial
// itself because the wrapper main keeps connection lifecycle.
func newAdapter(client *ipc.Client) *adapter {
	return &adapter{
		ipcClient: client,
		source:    "yt-cast",
		metadata:  newMetadataResolver(),
		stream:    newStreamResolver(),
	}
}

// setVolumeStep configures the quantisation step for DoSetVolume. 0 or
// negative values fall back to the default (5).
// setLogger attaches a logger used for the async title-resolution
// callback. Optional — without a logger the resolver still populates
// PlayIntent.Metadata, but the human-name log line is skipped.
func (a *adapter) setLogger(l *slog.Logger) {
	a.mu.Lock()
	a.log = l
	a.mu.Unlock()
}

// setOnStateChange registers a callback that fires after the adapter
// caches a new PlayerState from IPC. The host wires this to
// receiver.EmitPlayerState so external state updates (position, volume,
// duration, etc.) propagate back to connected senders as Lounge
// messages.
func (a *adapter) setOnStateChange(fn func(context.Context)) {
	a.mu.Lock()
	a.onStateChange = fn
	a.mu.Unlock()
}

// setPeekNextVideo registers the engine-aware callback used by DoPlay
// to determine the upcoming track to pre-load into MA's queue.
func (a *adapter) setPeekNextVideo(fn func() *types.Video) {
	a.mu.Lock()
	a.peekNextVideo = fn
	a.mu.Unlock()
}

// setIPCClient swaps the underlying writer. nil means "broker is
// offline" — Do* methods return an error in that case so the receiver
// can surface a sensible failure to senders.
func (a *adapter) setIPCClient(c *ipc.Client) {
	a.mu.Lock()
	a.ipcClient = c
	a.mu.Unlock()
}

// resetSession clears all per-cast state in the adapter so the next
// sender starts from a blank slate. Used when all senders disconnect
// — without this, a second device casting after the first would
// briefly see leftover title/artist/duration from the first session
// before its own DoPlay lands.
//
// Speaker-scoped state (volume, muted) is intentionally preserved —
// the speaker keeps its physical setting regardless of which sender
// is connected, and surfacing that to a fresh sender is the right
// initial value.
func (a *adapter) resetSession() {
	a.mu.Lock()
	prevVolume := a.cachedState.Volume
	prevMuted := a.cachedState.Muted
	a.cachedState = events.PlayerState{
		State:  "idle",
		Volume: prevVolume,
		Muted:  prevMuted,
	}
	a.playbackBasePos = 0
	a.playbackStartedAt = nil
	a.playbackPaused = false
	a.playbackPauseStart = nil
	a.playbackPauseAccum = 0
	log := a.log
	a.mu.Unlock()
	if log != nil {
		log.Info("yt-cast: session state reset — adapter ready for next sender")
	}
	// Also clear MA's queue so a brand-new sender that connects in
	// the future doesn't briefly see the previous cast's items in
	// MA's UI. Fire-and-forget — the dispatcher will route to
	// MAWS.ClearQueue when configured.
	if err := a.send(&events.TransportCommand{
		Command: "clear_queue",
		Source:  a.source,
	}); err != nil && log != nil {
		log.Debug("yt-cast: clear_queue dispatch failed (likely IPC offline)",
			"err", err)
	}
}

// updateCachedState replaces the cached PlayerState frame and fires
// the onStateChange callback (if any) so the host can re-emit player
// state to connected senders. The callback runs synchronously on the
// IPC reader goroutine — it should be cheap and non-blocking.
//
// At debug level we log the incoming state plus the delta vs our local
// position estimator — useful for diagnosing timestamp drift on long
// videos where the phone UI and the speaker fall out of sync.
func (a *adapter) updateCachedState(ps events.PlayerState) {
	a.mu.Lock()
	prev := a.cachedState
	// Volume echo suppression: if the user is actively pressing
	// volume buttons (last DoSetVolume within volumeInputWindow),
	// MA's state event reflecting the previous setting will race
	// the next press and snap the phone's slider back. Keep the
	// previous cached volume so the engine doesn't push a stale
	// onVolumeChanged to the sender. The next state event after
	// the window passes will resync.
	if ps.Volume != nil && !a.lastVolumeSentAt.IsZero() &&
		time.Since(a.lastVolumeSentAt) < volumeInputWindow {
		ps.Volume = prev.Volume
		ps.Muted = prev.Muted
	}
	a.cachedState = ps
	// When the track has ended (MA reports idle/stopped/off after
	// previously being playing/buffering/paused) the local estimator
	// should NOT keep ticking — it's a fallback for the early-playback
	// gap, not a continuous reference. Without this clear, MA stops
	// emitting media_position on idle, our cachedState.Position goes
	// nil, and the engine falls back to a runaway estimator showing
	// e.g. "3:44 / 1:27".
	if isTrackEndedState(ps.State) && isActivePlaybackState(prev.State) {
		a.playbackStartedAt = nil
		a.playbackBasePos = 0
		a.playbackPaused = false
		a.playbackPauseStart = nil
		a.playbackPauseAccum = 0
	}
	fn := a.onStateChange
	log := a.log
	localPos, localOK := a.localEstimateLocked()
	a.mu.Unlock()
	if log == nil {
		log = slog.Default()
	}
	// Build a compact debug record showing both what MA sent us and
	// how it compares to our wall-clock estimate. Throttled output
	// would obscure the cause of drift — let debug be verbose.
	attrs := []any{
		"state", ps.State,
		"title", ps.Title,
		"artist", ps.Artist,
		"track_id", ps.TrackID,
	}
	if ps.Position != nil {
		attrs = append(attrs, "ma_position", *ps.Position)
		if localOK {
			attrs = append(attrs, "local_estimate", localPos,
				"drift_seconds", *ps.Position-localPos)
		}
	} else if localOK {
		attrs = append(attrs, "local_estimate_only", localPos)
	}
	if ps.Duration != nil {
		attrs = append(attrs, "duration", *ps.Duration)
	}
	if ps.Volume != nil {
		attrs = append(attrs, "volume", *ps.Volume)
	}
	if ps.Muted != nil {
		attrs = append(attrs, "muted", *ps.Muted)
	}
	if prev.State != "" && prev.State != ps.State {
		attrs = append(attrs, "prev_state", prev.State)
	}
	log.Debug("yt-cast: cachedState updated", attrs...)
	if fn != nil {
		fn(context.Background())
	}
}

// volumeInputWindow is how long after a user-initiated volume change
// we suppress MA's volume state echoes from reaching the phone. The
// echo would otherwise overwrite the user's still-evolving slider
// position when they're pressing rapidly.
const volumeInputWindow = 2 * time.Second

// isTrackEndedState reports whether s is one of the MA / HA state
// strings that indicates the queue item finished or was stopped.
func isTrackEndedState(s string) bool {
	switch s {
	case "idle", "stopped", "off", "unavailable", "":
		return true
	}
	return false
}

// isActivePlaybackState reports whether s indicates an in-flight
// track (regardless of paused/playing/buffering).
func isActivePlaybackState(s string) bool {
	switch s {
	case "playing", "paused", "buffering", "loading":
		return true
	}
	return false
}

// localEstimateLocked returns the local wall-clock position estimate.
// The bool is false when no DoPlay/DoResume/DoSeek has been called yet
// (i.e. the estimator hasn't been seeded). Caller MUST hold a.mu.
func (a *adapter) localEstimateLocked() (float64, bool) {
	if a.playbackStartedAt == nil {
		return 0, false
	}
	now := time.Now()
	elapsed := now.Sub(*a.playbackStartedAt)
	if a.playbackPaused && a.playbackPauseStart != nil {
		elapsed -= a.playbackPauseAccum + now.Sub(*a.playbackPauseStart)
	} else {
		elapsed -= a.playbackPauseAccum
	}
	if elapsed < 0 {
		elapsed = 0
	}
	pos := a.playbackBasePos + elapsed.Seconds()
	// Cap at duration if we know it. The estimator should never run
	// past the end of the track — if it does, the phone sees
	// "3:44 / 1:27" because MA's media_position stops being reported
	// when the queue ends and we fall back to a runaway estimate.
	if a.cachedState.Duration != nil && *a.cachedState.Duration > 0 && pos > *a.cachedState.Duration {
		pos = *a.cachedState.Duration
	}
	return pos, true
}

// snapshotState returns a copy of the cached state.
func (a *adapter) snapshotState() events.PlayerState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cachedState
}

// send is the single send path. Holds the mutex for the duration of
// the write so concurrent emitters can't interleave bytes.
func (a *adapter) send(ev events.Event) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ipcClient == nil {
		return errIPCOffline
	}
	return a.ipcClient.Send(ev)
}

// errIPCOffline is the sentinel returned when no IPC client is
// available. Callers (the receiver) treat it as a transient failure;
// no automatic retry — the wrapper main reconnects on its own.
var errIPCOffline = errors.New("yt-cast: ma-bridge IPC unavailable")

// resolveIntent maps a Cast video onto a dispatcher-ready PlayIntent.
// See package doc comment for the provider mapping rationale.
func resolveIntent(video types.Video, source string) *events.PlayIntent {
	intent := &events.PlayIntent{Source: source}
	switch video.Client.Theme {
	case "m":
		// YouTube Music app — MA's native ytmusic provider takes the
		// raw video/track id.
		intent.Provider = "ytmusic"
		intent.TrackID = video.ID
	case "cl":
		// YouTube classic — hand MA the public watch URL so its stream
		// extractor (yt-dlp) can pull audio.
		intent.Provider = "url"
		intent.URL = "https://www.youtube.com/watch?v=" + video.ID
		intent.TrackID = video.ID
	default:
		// Unknown surface — let the dispatcher's "unknown provider" log
		// surface the issue rather than silently dropping.
		intent.Provider = video.Client.Theme
		intent.TrackID = video.ID
	}
	return intent
}

// DoPlay emits a PlayIntent. For YouTube-classic casts we synchronously
// pre-resolve the watch URL to a direct audio stream URL via yt-dlp —
// Music Assistant's URL provider needs that to actually play the audio
// (a bare watch URL fails MA's ffmpeg-probe with "Invalid data found").
// The resolution is in-band (typically 1–2s) so the receiver's PLAYING
// state matches reality. Title resolution remains async — that path
// is purely cosmetic.
//
// We also seed the local position estimator with the sender-supplied
// start position; see DoGetPosition for how the estimator is used.
func (a *adapter) DoPlay(ctx context.Context, video types.Video, position float64) error {
	intent := resolveIntent(video, a.source)
	intent.StartPosition = position
	a.mu.Lock()
	log := a.log
	a.mu.Unlock()
	if log == nil {
		log = slog.Default()
	}

	// For YouTube-classic casts pre-resolve metadata + stream URL
	// synchronously so the dispatcher can hand MA both rich metadata
	// (title / channel) and a direct audio stream URL.
	if intent.Provider == "url" && video.Client.Theme == "cl" {
		// Title / channel via oEmbed — cheap, cached. ~200ms cold,
		// near-zero on cache hit. We populate Metadata so the
		// dispatcher can attach it to media_player.play_media's
		// `extra.metadata.*` fields and MA's UI shows the real song
		// name instead of the raw URL.
		if a.metadata != nil {
			metaCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			if m, mErr := a.metadata.Resolve(metaCtx, video.ID); mErr == nil {
				if intent.Metadata == nil {
					intent.Metadata = make(map[string]any)
				}
				if m.Title != "" {
					intent.Metadata["title"] = m.Title
				}
				if m.Channel != "" {
					intent.Metadata["channel"] = m.Channel
				}
				intent.Metadata["source"] = "youtube"
				intent.Metadata["video_id"] = video.ID
				// Prefer the thumbnail URL YouTube returned in the oEmbed
				// payload (it's YouTube's officially-recommended preview
				// for the video). Fall back to the well-known
				// `hqdefault.jpg` URL form when oEmbed didn't include
				// one — every public YouTube video has an hqdefault
				// thumbnail at that path.
				if m.ThumbnailURL != "" {
					intent.Metadata["thumbnail"] = m.ThumbnailURL
					if m.ThumbnailWidth > 0 {
						intent.Metadata["thumbnail_width"] = m.ThumbnailWidth
					}
					if m.ThumbnailHeight > 0 {
						intent.Metadata["thumbnail_height"] = m.ThumbnailHeight
					}
				} else {
					intent.Metadata["thumbnail"] = "https://i.ytimg.com/vi/" + video.ID + "/hqdefault.jpg"
				}
			} else {
				log.Debug("yt-cast: metadata pre-resolve failed (non-fatal)",
					"video_id", video.ID, "err", mErr)
			}
			cancel()
		}

		// Stream URL via yt-dlp — the larger ~1–5s cost, but MA needs
		// it to actually play the audio.
		if a.stream != nil {
			info, err := a.stream.Resolve(ctx, video.ID)
			if err != nil {
				log.Error("yt-cast: stream URL pre-resolve failed — MA will reject the watch URL",
					"video_id", video.ID, "err", err)
			} else {
				log.Info("yt-cast: stream URL pre-resolved via yt-dlp",
					"video_id", video.ID,
					"stream_url", truncateString(info.URL, 120),
					"duration", info.Duration)
				intent.URL = info.URL
				if info.Duration > 0 {
					if intent.Metadata == nil {
						intent.Metadata = make(map[string]any)
					}
					intent.Metadata["duration"] = info.Duration
				}
			}
		}
	}

	if err := a.send(intent); err != nil {
		return err
	}
	// Seed the local position estimator AND replace the cached
	// PlayerState with fresh fields for this cast. Without the cached
	// replacement, the engine pushes a stale state to the phone (left
	// over from the previous cast — wrong title, wrong duration, wrong
	// position) for the ~1s before MA's first state_changed arrives.
	// With it, the phone sees the right metadata and a correct
	// position/duration ratio immediately.
	resolvedTitle, _ := intent.Metadata["title"].(string)
	resolvedChannel, _ := intent.Metadata["channel"].(string)
	resolvedDuration, _ := intent.Metadata["duration"].(float64)
	a.mu.Lock()
	now := time.Now()
	a.playbackBasePos = position
	a.playbackStartedAt = &now
	a.playbackPaused = false
	a.playbackPauseStart = nil
	a.playbackPauseAccum = 0
	// Preserve volume/muted across casts (those are speaker-level,
	// not track-level) but replace all track fields.
	prevVolume := a.cachedState.Volume
	prevMuted := a.cachedState.Muted
	posCopy := position
	a.cachedState = events.PlayerState{
		State:    "buffering",
		Title:    resolvedTitle,
		Artist:   resolvedChannel,
		TrackID:  intent.TrackID,
		Position: &posCopy,
		Volume:   prevVolume,
		Muted:    prevMuted,
	}
	if resolvedDuration > 0 {
		d := resolvedDuration
		a.cachedState.Duration = &d
	}
	fn := a.onStateChange
	a.preloadGen++
	preloadGen := a.preloadGen
	peek := a.peekNextVideo
	a.mu.Unlock()
	if fn != nil {
		fn(context.Background())
	}
	go a.logResolvedTitle(video.ID, intent.Provider)
	// Pre-load the engine's "what plays next" into MA's queue so MA
	// auto-advances to YouTube's choice when the current track ends
	// (instead of falling back to MA's library autoplay or stopping).
	// Runs in a goroutine because yt-dlp URL resolution can take
	// 1-5s, and we don't want to block the play path.
	if peek != nil {
		go a.preloadUpcoming(preloadGen, peek)
	}
	return nil
}

// preloadUpcoming resolves the engine's upcoming-video URL via yt-dlp
// and dispatches a QueueAddIntent so the dispatcher appends it to
// MA's queue. gen is the preload generation at the time DoPlay was
// called; if it has advanced (i.e. a newer cast started) we abort
// without dispatching to avoid inserting a stale +1 into the
// freshly-rebuilt queue.
func (a *adapter) preloadUpcoming(gen uint64, peek func() *types.Video) {
	a.mu.Lock()
	log := a.log
	a.mu.Unlock()
	if log == nil {
		log = slog.Default()
	}
	video := peek()
	if video == nil {
		// Engine doesn't have a Next or Autoplay candidate. This is
		// expected for single-video casts where the YouTube app
		// didn't supply an up-next list. Logged at info so the user
		// can correlate "queue not added to MA" reports with this
		// reason in the log.
		log.Info("yt-cast: preload skipped — no upcoming video supplied by YouTube cast app")
		return
	}
	// Currently only YouTube-classic (the cl theme) needs URL
	// resolution; YT Music tracks are routed to MA via the ytmusic
	// provider URI and don't need pre-resolution. Music tracks are
	// also already known to MA so adding to queue is unnecessary
	// (MA would auto-advance through ytmusic anyway).
	if video.Client.Theme != "cl" {
		log.Debug("yt-cast: skipping preload (not yt-classic)",
			"video_id", video.ID,
			"theme", video.Client.Theme)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Stream URL — the expensive call, but necessary so MA's
	// builtin provider can stream the item when transitioned to.
	if a.stream == nil {
		return
	}
	info, err := a.stream.Resolve(ctx, video.ID)
	if err != nil {
		log.Warn("yt-cast: preload stream URL resolve failed (no auto-advance)",
			"video_id", video.ID, "err", err)
		return
	}

	// Abort if a newer cast started while we were resolving — the
	// queue has been rebuilt by that newer DoPlay and adding now
	// would insert a stale entry.
	a.mu.Lock()
	current := a.preloadGen
	a.mu.Unlock()
	if current != gen {
		log.Debug("yt-cast: preload aborted (newer cast superseded)",
			"video_id", video.ID, "started_gen", gen, "current_gen", current)
		return
	}

	metaCtx, metaCancel := context.WithTimeout(context.Background(), 3*time.Second)
	meta, mErr := a.metadata.Resolve(metaCtx, video.ID)
	metaCancel()

	intent := &events.QueueAddIntent{
		Provider: "url",
		TrackID:  video.ID,
		URL:      info.URL,
		Source:   a.source,
		Metadata: map[string]any{
			"source":   "youtube",
			"video_id": video.ID,
		},
	}
	if info.Duration > 0 {
		intent.Metadata["duration"] = info.Duration
	}
	if mErr == nil {
		if meta.Title != "" {
			intent.Metadata["title"] = meta.Title
		}
		if meta.Channel != "" {
			intent.Metadata["channel"] = meta.Channel
		}
		if meta.ThumbnailURL != "" {
			intent.Metadata["thumbnail"] = meta.ThumbnailURL
		} else {
			intent.Metadata["thumbnail"] = "https://i.ytimg.com/vi/" + video.ID + "/hqdefault.jpg"
		}
	}
	if err := a.send(intent); err != nil {
		log.Debug("yt-cast: preload QueueAddIntent dispatch failed",
			"err", err, "video_id", video.ID)
		return
	}
	log.Info("yt-cast: preloaded upcoming track",
		"video_id", video.ID,
		"title", intent.Metadata["title"],
		"duration", info.Duration)
}


// logResolvedTitle runs out-of-band so the play path stays optimistic.
// Resolution failures degrade to a single warn line carrying the ID.
func (a *adapter) logResolvedTitle(videoID, provider string) {
	if a.metadata == nil || videoID == "" {
		return
	}
	a.mu.Lock()
	log := a.log
	a.mu.Unlock()
	if log == nil {
		log = slog.Default()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m, err := a.metadata.Resolve(ctx, videoID)
	if err != nil {
		log.Warn("yt-cast: metadata resolve failed",
			"video_id", videoID, "provider", provider, "err", err)
		return
	}
	log.Info("yt-cast: now playing",
		"video_id", videoID,
		"title", m.Title,
		"channel", m.Channel,
		"provider", provider)
}

// DoPause emits a pause TransportCommand. We also freeze the local
// position estimator at the current effective position so the phone
// doesn't see the timer keep advancing before MA reports back the
// paused state.
func (a *adapter) DoPause(_ context.Context) error {
	a.mu.Lock()
	log := a.log
	var localPos float64
	var hasLocal bool
	if !a.playbackPaused {
		now := time.Now()
		a.playbackPaused = true
		a.playbackPauseStart = &now
		localPos, hasLocal = a.localEstimateLocked()
	}
	a.mu.Unlock()
	if log == nil {
		log = slog.Default()
	}
	if hasLocal {
		log.Debug("yt-cast: DoPause — local estimator frozen", "at", localPos)
	} else {
		log.Debug("yt-cast: DoPause (no local estimate)")
	}
	return a.send(&events.TransportCommand{Command: "pause", Source: a.source})
}

// DoResume emits a play TransportCommand. Resumes the local position
// estimator by absorbing the time spent paused into the accumulator
// so future DoGetPosition calls don't count it as playback.
func (a *adapter) DoResume(_ context.Context) error {
	a.mu.Lock()
	log := a.log
	var pausedFor time.Duration
	if a.playbackPaused && a.playbackPauseStart != nil {
		pausedFor = time.Since(*a.playbackPauseStart)
		a.playbackPauseAccum += pausedFor
	}
	a.playbackPaused = false
	a.playbackPauseStart = nil
	localPos, hasLocal := a.localEstimateLocked()
	a.mu.Unlock()
	if log == nil {
		log = slog.Default()
	}
	if hasLocal {
		log.Debug("yt-cast: DoResume — estimator resumed",
			"paused_for_seconds", pausedFor.Seconds(), "at", localPos)
	} else {
		log.Debug("yt-cast: DoResume (no local estimate)")
	}
	return a.send(&events.TransportCommand{Command: "play", Source: a.source})
}

// DoStop emits a stop TransportCommand and clears the local
// position estimator.
func (a *adapter) DoStop(_ context.Context) error {
	a.mu.Lock()
	a.playbackStartedAt = nil
	a.playbackBasePos = 0
	a.playbackPaused = false
	a.playbackPauseStart = nil
	a.playbackPauseAccum = 0
	a.mu.Unlock()
	return a.send(&events.TransportCommand{Command: "stop", Source: a.source})
}

// DoSeek emits a seek TransportCommand with the requested position
// and rebases the local position estimator on the new value so the
// phone's UI doesn't jump back to the pre-seek position while we
// wait for MA to confirm the new state via HA.
func (a *adapter) DoSeek(_ context.Context, position float64) error {
	a.mu.Lock()
	log := a.log
	now := time.Now()
	a.playbackBasePos = position
	a.playbackStartedAt = &now
	a.playbackPauseAccum = 0
	a.playbackPauseStart = nil
	// Don't change a.playbackPaused — if we were paused, stay paused.
	a.mu.Unlock()
	if log == nil {
		log = slog.Default()
	}
	log.Info("yt-cast: DoSeek", "position", position)
	p := position
	return a.send(&events.TransportCommand{
		Command:  "seek",
		Position: &p,
		Source:   a.source,
	})
}

// DoSetVolume emits a VolumeCommand with the raw value from the
// sender, untouched. Earlier versions tried to be clever (round to
// volume_step, echo bucket boundaries back to the phone, delta-track
// presses) but every variation created a different artifact: the
// optimistic echo caused a feedback loop where the phone slider got
// snapped back inside its current bucket, and delta tracking
// stuttered when MA's state events overwrote our reference. The
// simplest behavior — pass raw straight through — leaves the cast
// app authoritative over its own UI, MA authoritative over the
// speaker, and avoids the user having to "press multiple times for
// it to feel natural" reported on v0.1.17.
func (a *adapter) DoSetVolume(_ context.Context, volume pkgplayer.Volume) error {
	level := float64(volume.Level) / 100.0
	muted := volume.Muted
	a.mu.Lock()
	log := a.log
	a.lastVolumeSentAt = time.Now()
	a.mu.Unlock()
	if log == nil {
		log = slog.Default()
	}
	log.Info("yt-cast: DoSetVolume",
		"raw", volume.Level, "level", level, "muted", muted)
	return a.send(&events.VolumeCommand{
		Level:  &level,
		Muted:  &muted,
		Source: a.source,
	})
}

// DoGetVolume returns the cached volume + muted state, rescaled from
// the dispatcher's 0.0-1.0 wire range back to the receiver's 0-100.
func (a *adapter) DoGetVolume(_ context.Context) (pkgplayer.Volume, error) {
	st := a.snapshotState()
	out := pkgplayer.Volume{}
	if st.Volume != nil {
		out.Level = int(*st.Volume * 100)
	}
	if st.Muted != nil {
		out.Muted = *st.Muted
	}
	return out, nil
}

// DoGetPosition returns the current playback position in seconds.
//
// Preference order:
//
//  1. cachedState.Position — the truth from MA via HA's state_changed
//     events. Available once MA has actually started streaming and HA
//     has emitted at least one media_position update for the entity.
//
//  2. Local wall-clock estimate — derived from when we last called
//     DoPlay / DoSeek / DoResume and how much time has elapsed since,
//     minus any accumulated pause time. This covers the 2-10 second
//     gap between play_media and MA's first state_changed report, and
//     the moment between a pause command and HA reflecting the paused
//     state. Without this fallback the phone's progress bar snaps to
//     0:00 on first pause-after-play, which is jarring.
//
//  3. 0 — nothing else known.
func (a *adapter) DoGetPosition(_ context.Context) (float64, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cachedState.Position != nil {
		return *a.cachedState.Position, nil
	}
	pos, ok := a.localEstimateLocked()
	if !ok {
		return 0, nil
	}
	return pos, nil
}

// DoGetDuration returns the cached duration in seconds.
func (a *adapter) DoGetDuration(_ context.Context) (float64, error) {
	st := a.snapshotState()
	if st.Duration != nil {
		return *st.Duration, nil
	}
	return 0, nil
}

// Compile-time assertion that adapter satisfies the Player contract.
var _ pkgplayer.Player = (*adapter)(nil)
