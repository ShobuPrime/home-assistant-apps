// Maps to: src/lib/utils/Errors.ts
//
// Package yterrors mirrors the YouTubeCastReceiverError class hierarchy from
// the upstream Node.js library. Every concrete error type carries the same
// fields the TypeScript constructor accepts, plus an `Is(target)` matcher so
// callers can use `errors.Is(err, yterrors.ErrConnection)` to test the kind
// without reaching for `errors.As` and a typed pointer.
package yterrors

import (
	"errors"
	"fmt"
)

// Sentinel kinds — pair each concrete struct type with one of these so
// callers can check the family of an error without caring about the fields.
//
//nolint:revive // names mirror upstream class names.
var (
	ErrYouTubeCastReceiver = errors.New("YouTubeCastReceiverError")
	ErrConnection          = errors.New("ConnectionError")
	ErrAbort               = errors.New("AbortError")
	ErrBadResponse         = errors.New("BadResponseError")
	ErrData                = errors.New("DataError")
	ErrIncompleteAPIData   = errors.New("IncompleteAPIDataError")
	ErrSession             = errors.New("SessionError")
	ErrApp                 = errors.New("AppError")
	ErrDialServer          = errors.New("DialServerError")
	ErrSenderConnection    = errors.New("SenderConnectionError")
)

// SenderConnectionAction enumerates the values the upstream
// `SenderConnectionError` constructor accepts for its `action` argument.
type SenderConnectionAction string

const (
	SenderConnectionActionConnect    SenderConnectionAction = "connect"
	SenderConnectionActionDisconnect SenderConnectionAction = "disconnect"
	SenderConnectionActionUnknown    SenderConnectionAction = "unknown"
)

// BaseError ports the upstream `YouTubeCastReceiverError` base class. All the
// typed errors below embed *BaseError so they inherit Cause, Info, Unwrap,
// and the `getCauses` walker.
type BaseError struct {
	// Name reproduces the upstream `name` property — the error class name.
	Name string
	// Message is the human-readable description.
	Message string
	// Cause is the nested error (any value upstream; we narrow to error since
	// Go callers will always wrap errors, not arbitrary values).
	Cause error
	// Info is the optional metadata bag upstream stores under `info`.
	Info map[string]any
}

// Error implements the error interface.
func (e *BaseError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause != nil {
		return fmt.Sprintf("(%s) %s: %v", e.Name, e.Message, e.Cause)
	}
	return fmt.Sprintf("(%s) %s", e.Name, e.Message)
}

// Unwrap exposes the wrapped cause to the standard errors package.
func (e *BaseError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Is matches the YouTubeCastReceiverError sentinel — every typed subclass
// also reports true here because the upstream class hierarchy is rooted in
// YouTubeCastReceiverError.
func (e *BaseError) Is(target error) bool {
	return target == ErrYouTubeCastReceiver
}

// GetCauses ports `YouTubeCastReceiverError.getCauses()`. It walks the cause
// chain, returning each ancestor in order. Walking stops as soon as a cause
// is not a YouTubeCastReceiverError (matching the upstream check `c
// instanceof YouTubeCastReceiverError ? c.cause : null`).
//
// The walk uses errors.Is(c, ErrYouTubeCastReceiver) rather than errors.As to
// detect family membership: the typed errors below embed *BaseError but are
// distinct concrete types, so errors.As on a **BaseError target would skip
// over them. The included error is appended before the family check so the
// final non-YT error (e.g. a stdlib errors.errorString from a Go API) ends
// up in the slice — matching upstream, where the last cause may be any value
// thrown by the JS runtime.
func (e *BaseError) GetCauses() []error {
	if e == nil {
		return nil
	}
	causes := make([]error, 0)
	c := e.Cause
	for c != nil {
		causes = append(causes, c)
		if !errors.Is(c, ErrYouTubeCastReceiver) {
			break
		}
		c = errors.Unwrap(c)
	}
	return causes
}

// New constructs a generic YouTubeCastReceiverError. Prefer the specific
// constructors below when the failure has a typed flavour upstream.
func New(message string, cause error, info map[string]any) *BaseError {
	return &BaseError{
		Name:    "YouTubeCastReceiverError",
		Message: message,
		Cause:   cause,
		Info:    info,
	}
}

// ConnectionError ports `class ConnectionError` — failure reaching `URL`.
type ConnectionError struct{ *BaseError }

// NewConnectionError constructs a ConnectionError.
func NewConnectionError(message, url string, cause error) *ConnectionError {
	return &ConnectionError{&BaseError{
		Name:    "ConnectionError",
		Message: message,
		Cause:   cause,
		Info:    map[string]any{"url": url},
	}}
}

// Is matches the ConnectionError sentinel as well as the base sentinel.
func (e *ConnectionError) Is(target error) bool {
	return target == ErrConnection || target == ErrYouTubeCastReceiver
}

// AbortError ports `class AbortError` — request to `URL` was aborted.
type AbortError struct{ *BaseError }

// NewAbortError constructs an AbortError.
func NewAbortError(message, url string) *AbortError {
	return &AbortError{&BaseError{
		Name:    "AbortError",
		Message: message,
		Info:    map[string]any{"url": url},
	}}
}

// Is matches the AbortError sentinel as well as the base sentinel.
func (e *AbortError) Is(target error) bool {
	return target == ErrAbort || target == ErrYouTubeCastReceiver
}

// BadResponseError ports `class BadResponseError` — HTTP response from `URL`
// had a non-success `Status` / `StatusText`.
type BadResponseError struct {
	*BaseError
}

// HTTPResponse mirrors the `{status, statusText}` shape upstream stashes in
// the error's `info.response` field.
type HTTPResponse struct {
	Status     int    `json:"status"`
	StatusText string `json:"statusText"`
}

// NewBadResponseError constructs a BadResponseError.
func NewBadResponseError(message, url string, response HTTPResponse) *BadResponseError {
	return &BadResponseError{&BaseError{
		Name:    "BadResponseError",
		Message: message,
		Info:    map[string]any{"url": url, "response": response},
	}}
}

// Is matches the BadResponseError sentinel as well as the base sentinel.
func (e *BadResponseError) Is(target error) bool {
	return target == ErrBadResponse || target == ErrYouTubeCastReceiver
}

// DataError ports `class DataError` — parsing/validating a payload failed.
type DataError struct{ *BaseError }

// NewDataError constructs a DataError. `data` is the offending payload and
// is stored under `info.data` (matching upstream).
func NewDataError(message string, cause error, data any) *DataError {
	return &DataError{&BaseError{
		Name:    "DataError",
		Message: message,
		Cause:   cause,
		Info:    map[string]any{"data": data},
	}}
}

// Is matches the DataError sentinel as well as the base sentinel.
func (e *DataError) Is(target error) bool {
	return target == ErrData || target == ErrYouTubeCastReceiver
}

// IncompleteAPIDataError ports `class IncompleteAPIDataError` — required
// fields were missing from the parsed API payload.
type IncompleteAPIDataError struct{ *BaseError }

// NewIncompleteAPIDataError constructs an IncompleteAPIDataError. `missing`
// names the absent fields and is stored under `info.missing`.
func NewIncompleteAPIDataError(message string, missing []string) *IncompleteAPIDataError {
	return &IncompleteAPIDataError{&BaseError{
		Name:    "IncompleteAPIDataError",
		Message: message,
		Info:    map[string]any{"missing": missing},
	}}
}

// Is matches the IncompleteAPIDataError sentinel as well as the base sentinel.
func (e *IncompleteAPIDataError) Is(target error) bool {
	return target == ErrIncompleteAPIData || target == ErrYouTubeCastReceiver
}

// SessionError ports `class SessionError` — the lounge session is in a bad
// state and needs to be re-established.
type SessionError struct{ *BaseError }

// NewSessionError constructs a SessionError.
func NewSessionError(message string, cause error) *SessionError {
	return &SessionError{&BaseError{
		Name:    "SessionError",
		Message: message,
		Cause:   cause,
	}}
}

// Is matches the SessionError sentinel as well as the base sentinel.
func (e *SessionError) Is(target error) bool {
	return target == ErrSession || target == ErrYouTubeCastReceiver
}

// AppError ports `class AppError` — failure in the YouTubeApp orchestrator.
type AppError struct{ *BaseError }

// NewAppError constructs an AppError.
func NewAppError(message string, cause error) *AppError {
	return &AppError{&BaseError{
		Name:    "AppError",
		Message: message,
		Cause:   cause,
	}}
}

// Is matches the AppError sentinel as well as the base sentinel.
func (e *AppError) Is(target error) bool {
	return target == ErrApp || target == ErrYouTubeCastReceiver
}

// DialServerError ports `class DialServerError` — failure inside the DIAL
// HTTP / SSDP server.
type DialServerError struct{ *BaseError }

// NewDialServerError constructs a DialServerError.
func NewDialServerError(message string, cause error) *DialServerError {
	return &DialServerError{&BaseError{
		Name:    "DialServerError",
		Message: message,
		Cause:   cause,
	}}
}

// Is matches the DialServerError sentinel as well as the base sentinel.
func (e *DialServerError) Is(target error) bool {
	return target == ErrDialServer || target == ErrYouTubeCastReceiver
}

// SenderConnectionError ports `class SenderConnectionError` — the receiver
// could not connect/disconnect a sender.
type SenderConnectionError struct{ *BaseError }

// NewSenderConnectionError constructs a SenderConnectionError. `action` is
// stored under `info.action` (defaulting to `"unknown"` when empty, matching
// upstream).
func NewSenderConnectionError(message string, cause error, action SenderConnectionAction) *SenderConnectionError {
	if action == "" {
		action = SenderConnectionActionUnknown
	}
	return &SenderConnectionError{&BaseError{
		Name:    "SenderConnectionError",
		Message: message,
		Cause:   cause,
		Info:    map[string]any{"action": string(action)},
	}}
}

// Is matches the SenderConnectionError sentinel as well as the base sentinel.
func (e *SenderConnectionError) Is(target error) bool {
	return target == ErrSenderConnection || target == ErrYouTubeCastReceiver
}
