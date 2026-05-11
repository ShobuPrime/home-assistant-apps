// Maps to: src/lib/app/ (package-level doc)
//
// Package lounge ports the YouTube Lounge protocol layer from
// yt-cast-receiver v2.1.1. It implements the HTTP long-poll RPC channel
// between the receiver and YouTube's lounge endpoints (`/bind`,
// `generate_screen_id`, `get_lounge_token_batch`, `get_pairing_code`,
// `register_pairing_code`), the line-by-line message parser, the
// per-sender Session lifecycle (lounge-token fetch + refresh, RPC stream,
// outgoing message queue with AID/GSN sequencing), an in-memory Playlist,
// and the pairing-code service used for manual `Link with TV code` flows.
//
// Phase 3's orchestrator (`YouTubeApp`) wires this package up to the DIAL
// server and to a host-supplied Player implementation.
//
// Style: every Go file in this directory has a `// Maps to:` header
// naming the upstream `.ts` source. Helper files with no direct upstream
// counterpart use `Maps to: N/A — Go-only ...`.
package lounge
