// Maps to: src/lib/utils/DefaultLogger.ts
//
// DefaultLogger is the stdlib-only Logger implementation that ships with the
// port. Upstream uses `console.{error,warn,info,debug}` and util.inspect for
// objects; we use log/slog with a text handler writing to stderr, which gets
// the user the same human-readable output without pulling in an extra dep.
package logger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/shobuprime/sonuntius/internal/ytcast/yterrors"
)

// DefaultLogger ports the upstream `DefaultLogger` class. The Color flag is
// preserved for API parity but does nothing today — we leave it in so callers
// configuring it the upstream way don't break, and so we have a place to wire
// ANSI colour output later if desired.
type DefaultLogger struct {
	mu     sync.Mutex
	level  LogLevel
	color  bool
	handler slog.Handler
	out    io.Writer
}

// NewDefaultLogger constructs a DefaultLogger writing to stderr. Pass
// `color=true` to match the upstream constructor default; it is recorded but
// has no visible effect (see struct comment).
func NewDefaultLogger(color bool) *DefaultLogger {
	out := os.Stderr
	return &DefaultLogger{
		level:   LogLevelInfo,
		color:   color,
		out:     out,
		handler: slog.NewTextHandler(out, &slog.HandlerOptions{Level: slog.LevelDebug}),
	}
}

// Error implements Logger.
func (l *DefaultLogger) Error(msg ...any) { l.process(LogLevelError, msg) }

// Warn implements Logger.
func (l *DefaultLogger) Warn(msg ...any) { l.process(LogLevelWarn, msg) }

// Info implements Logger.
func (l *DefaultLogger) Info(msg ...any) { l.process(LogLevelInfo, msg) }

// Debug implements Logger.
func (l *DefaultLogger) Debug(msg ...any) { l.process(LogLevelDebug, msg) }

// SetLevel implements Logger.
func (l *DefaultLogger) SetLevel(value LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = value
}

// Level returns the currently configured level (helper for tests / callers
// that want to introspect).
func (l *DefaultLogger) Level() LogLevel {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.level
}

// checkLevel ports DefaultLogger.checkLevel — true when targetLevel should be
// emitted given the configured level.
func (l *DefaultLogger) checkLevel(targetLevel LogLevel) bool {
	l.mu.Lock()
	current := l.level
	l.mu.Unlock()
	return logLevelIndex(targetLevel) <= logLevelIndex(current)
}

// process ports DefaultLogger.process — gate on level then emit.
func (l *DefaultLogger) process(targetLevel LogLevel, msg []any) {
	if !l.checkLevel(targetLevel) {
		return
	}
	l.toOutput(targetLevel, l.toStrings(msg))
}

// toStrings ports DefaultLogger.toStrings — render each argument the same way
// the TypeScript reduce() does. YouTubeCastReceiverError values get the chain
// of causes plus a stack trace marker; plain Errors get their message + chain;
// plain values fall through to fmt.Sprint.
func (l *DefaultLogger) toStrings(msg []any) []string {
	out := make([]string, 0, len(msg))
	for _, m := range msg {
		var be *yterrors.BaseError
		switch {
		case asBaseError(m, &be):
			causes := append([]error{be}, be.GetCauses()...)
			for i, e := range causes {
				prefix := ""
				if i > 0 {
					prefix = strings.Repeat("---", i) + ">"
				}
				name, message, info := errorParts(e)
				out = append(out, fmt.Sprintf("%s(%s) %s", prefix, name, message))
				if info != nil {
					indent := ""
					if i > 0 {
						indent = strings.Repeat("   ", i) + " "
					}
					out = append(out, fmt.Sprintf("%sError info: %#v", indent, info))
				}
			}
		case isError(m):
			err := m.(error)
			out = append(out, fmt.Sprintf("(%s) %s", goErrName(err), err.Error()))
		default:
			out = append(out, fmt.Sprint(m))
		}
	}
	return out
}

// toOutput ports DefaultLogger.toOutput — picks the slog level corresponding
// to the upstream console.* call. We collapse all four upstream channels onto
// slog because Go's standard library has no separate "info/warn/debug" pipes.
func (l *DefaultLogger) toOutput(targetLevel LogLevel, msg []string) {
	var slogLevel slog.Level
	switch targetLevel {
	case LogLevelError:
		slogLevel = slog.LevelError
	case LogLevelWarn:
		slogLevel = slog.LevelWarn
	case LogLevelInfo:
		slogLevel = slog.LevelInfo
	case LogLevelDebug:
		slogLevel = slog.LevelDebug
	default:
		return
	}
	r := slog.NewRecord(time.Now(), slogLevel, strings.Join(msg, " "), 0)
	_ = l.handler.Handle(context.Background(), r)
}

// logLevelIndex returns the position of `lvl` in constants.LogLevelOrder, or
// 0 (LogLevelNone) if not present.
func logLevelIndex(lvl LogLevel) int {
	for i, candidate := range []LogLevel{
		LogLevelNone,
		LogLevelError,
		LogLevelWarn,
		LogLevelInfo,
		LogLevelDebug,
	} {
		if candidate == lvl {
			return i
		}
	}
	return 0
}

// asBaseError unwraps `m` into a *yterrors.BaseError when possible.
func asBaseError(m any, target **yterrors.BaseError) bool {
	if e, ok := m.(error); ok {
		return errors.As(e, target)
	}
	return false
}

// isError reports whether `m` satisfies the error interface.
func isError(m any) bool {
	_, ok := m.(error)
	return ok
}

// errorParts pulls (Name, Message, Info) out of an error, descending into a
// *yterrors.BaseError if available.
func errorParts(e error) (name, message string, info map[string]any) {
	var be *yterrors.BaseError
	if errors.As(e, &be) {
		return be.Name, be.Message, be.Info
	}
	return goErrName(e), e.Error(), nil
}

// goErrName returns the dynamic type name for non-BaseError errors so the
// logger output remains symmetric with the TS `e.name` field.
func goErrName(e error) string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%T", e)
}
