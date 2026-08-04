// Maps to: src/lib/utils/Logger.ts
//
// Package logger defines the Logger interface that yt-cast-receiver consumers
// implement (or use the slog-backed default from default.go). The interface
// mirrors the upstream `Logger` TypeScript interface verbatim so call sites
// from the rest of the port read the same.
package logger

import (
	"github.com/shobuprime/sonuntius/internal/ytcast/constants"
)

// LogLevel re-exports constants.LogLevel under the logger package for
// ergonomic call sites (`logger.LogLevelDebug`).
type LogLevel = constants.LogLevel

// Re-exported LogLevel values matching constants.LogLevelXxx.
const (
	LogLevelError = constants.LogLevelError
	LogLevelWarn  = constants.LogLevelWarn
	LogLevelInfo  = constants.LogLevelInfo
	LogLevelDebug = constants.LogLevelDebug
	LogLevelNone  = constants.LogLevelNone
)

// Logger is the interface upstream defines in Logger.ts. Methods accept
// variadic arguments matching `(...msg: any[]) => void`; the default impl
// formats them with fmt.Sprint-style spacing to stay close to console.log
// behaviour.
type Logger interface {
	// Error logs at LogLevelError.
	Error(msg ...any)
	// Warn logs at LogLevelWarn.
	Warn(msg ...any)
	// Info logs at LogLevelInfo.
	Info(msg ...any)
	// Debug logs at LogLevelDebug.
	Debug(msg ...any)
	// SetLevel sets the minimum severity that gets emitted. Messages below
	// `value` are dropped. Setting LogLevelNone silences the logger.
	SetLevel(value LogLevel)
}
