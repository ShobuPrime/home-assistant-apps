// Maps to: src/index.ts
//
// Upstream's `src/index.ts` re-exports the public surface
// (YouTubeCastReceiver, Player, Constants, error types, ...) at the
// package root. Go doesn't have a barrel-file convention — sub-packages
// are imported directly — so this file is a thin doc-only entry point.
// It exists so the package itself has a Maps-to anchor and so hosts can
// stop here when looking for "where does the receiver entry point
// live?".
//
// The single re-export below — Status — is for hosts that want to
// switch on the receiver's lifecycle state without pulling in the
// constants package alongside.
package ytcast

import "github.com/shobuprime/sonuntius/internal/ytcast/constants"

// Status enumerates the receiver lifecycle states. Re-export of
// constants.Status for ergonomic call sites.
type Status = constants.Status

// Receiver status values mirrored from constants.Status* — handy when
// matching on Receiver.Status() return values.
const (
	StatusStopped  = constants.StatusStopped
	StatusStopping = constants.StatusStopping
	StatusStarting = constants.StatusStarting
	StatusRunning  = constants.StatusRunning
)

// UpstreamCommit re-exports the pinned commit hash so wrapper binaries
// can log it without having to import the constants package.
const UpstreamCommit = constants.UpstreamCommit

// UpstreamVersion re-exports the upstream npm version of the pinned
// commit.
const UpstreamVersion = constants.UpstreamVersion
