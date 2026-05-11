// Maps to: src/lib/Constants.ts
//
// Package constants holds the protocol-level URLs, defaults, and enums that
// the upstream yt-cast-receiver library exposes via Constants.ts. Values are
// kept byte-identical with upstream wherever they appear on the wire.
package constants

// YouTube base URL used to build the lounge endpoint URLs below.
const YouTubeBaseURL = "https://www.youtube.com"

// URLs reachable on www.youtube.com that the lounge protocol talks to.
//
// Upstream exposes these through `URLS` in `src/lib/Constants.ts`. Names are
// preserved in PascalCase to keep the mapping obvious.
const (
	URLYouTubeBase           = YouTubeBaseURL
	URLGenerateScreenID      = YouTubeBaseURL + "/api/lounge/pairing/generate_screen_id"
	URLGetLoungeTokenBatch   = YouTubeBaseURL + "/api/lounge/pairing/get_lounge_token_batch"
	URLRegisterPairingCode   = YouTubeBaseURL + "/api/lounge/pairing/register_pairing_code"
	URLGetPairingCode        = YouTubeBaseURL + "/api/lounge/pairing/get_pairing_code?ctx=pair"
	URLBind                  = YouTubeBaseURL + "/api/lounge/bc/bind"
)

// ConfDefaults mirrors the `CONF_DEFAULTS` constants that callers can use to
// fill in unset configuration fields (screen app id, brand, model).
const (
	ConfDefaultScreenApp = "ytcr"
	ConfDefaultBrand     = "Generic"
	ConfDefaultModel     = "SmartTV"
)

// AutoplayMode is one of the `AUTOPLAY_MODES` string values.
type AutoplayMode string

const (
	AutoplayModeEnabled     AutoplayMode = "ENABLED"
	AutoplayModeDisabled    AutoplayMode = "DISABLED"
	AutoplayModeUnsupported AutoplayMode = "UNSUPPORTED"
)

// PlayerStatus is one of the `PLAYER_STATUSES` integer values.
type PlayerStatus int

const (
	PlayerStatusIdle    PlayerStatus = -1
	PlayerStatusPlaying PlayerStatus = 1
	PlayerStatusPaused  PlayerStatus = 2
	PlayerStatusLoading PlayerStatus = 3
	PlayerStatusStopped PlayerStatus = 4
)

// LogLevel is one of the `LOG_LEVELS` string values. Order matters: a logger
// configured at level X emits messages whose severity is at or above X. The
// canonical ordering is encoded in LogLevelOrder below.
type LogLevel string

const (
	LogLevelError LogLevel = "error"
	LogLevelWarn  LogLevel = "warn"
	LogLevelInfo  LogLevel = "info"
	LogLevelDebug LogLevel = "debug"
	LogLevelNone  LogLevel = "none"
)

// LogLevelOrder mirrors upstream's `LOG_LEVEL_ORDER` from DefaultLogger.ts —
// less verbose to more verbose. A message at index i is emitted iff i is less
// than or equal to the logger's configured level index.
var LogLevelOrder = [...]LogLevel{
	LogLevelNone,
	LogLevelError,
	LogLevelWarn,
	LogLevelInfo,
	LogLevelDebug,
}

// Status is one of the `STATUSES` lifecycle values for the receiver root.
type Status string

const (
	StatusStopped  Status = "stopped"
	StatusStopping Status = "stopping"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
)

// MutePolicy is one of the `MUTE_POLICIES` string values.
type MutePolicy string

const (
	MutePolicyZeroVolumeLevel     MutePolicy = "zeroLevel"
	MutePolicyPreserveVolumeLevel MutePolicy = "preserveLevel"
	MutePolicyAuto                MutePolicy = "auto"
)

// ResetPlayerOnDisconnectPolicy is one of the
// `RESET_PLAYER_ON_DISCONNECT_POLICIES` string values.
type ResetPlayerOnDisconnectPolicy string

const (
	ResetPlayerOnDisconnectAllExplicitlyDisconnected ResetPlayerOnDisconnectPolicy = "allExplicitlyDisconnected"
	ResetPlayerOnDisconnectAllDisconnected           ResetPlayerOnDisconnectPolicy = "allDisconnected"
)
