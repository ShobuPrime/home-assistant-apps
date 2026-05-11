// Maps to: N/A — Go-only commit pin for the upstream yt-cast-receiver source.
//
// Bump these constants whenever UPSTREAM.md is updated to point at a newer
// commit; the values are surfaced in version banners and are checked in the
// CI smoke test to make sure the port doesn't drift from the pinned source.
package constants

// Upstream metadata for the yt-cast-receiver source this package is ported
// from. Keep in sync with internal/ytcast/UPSTREAM.md.
const (
	// UpstreamCommit is the full SHA of the upstream commit the port targets.
	UpstreamCommit = "83d61fa169e33c5e0046c2440b99a17cd9493e73"

	// UpstreamTag is the upstream git tag that points at UpstreamCommit, if any.
	UpstreamTag = "v2.1.1"

	// UpstreamVersion is the npm version published from UpstreamCommit.
	UpstreamVersion = "2.1.1"
)
