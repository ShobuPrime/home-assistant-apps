// Maps to: src/lib/app/Video.ts
//
// Video is the playback target the player operates on. Upstream defines it as
// a TypeScript interface with an open `context` map; we mirror that shape
// faithfully (typed top-level fields plus an `Extra` map for the open keys
// upstream allows via `& Record<string, any>`).
package types

// Video matches the upstream `interface Video`.
type Video struct {
	// ID is the YouTube video id (the bare watch?v= identifier).
	ID string `json:"id"`
	// Client identifies which surface (YT vs YTMUSIC) the video belongs to.
	Client Client `json:"client"`
	// Context is the optional structured context attached to a queued video.
	Context *VideoContext `json:"context,omitempty"`
}

// VideoContext mirrors the upstream `context?: { ... } & Record<string, any>`
// inline type. Top-level fields are typed; Extra captures any other keys
// upstream stuffs into the same object.
type VideoContext struct {
	PlaylistID string         `json:"playlistId,omitempty"`
	Params     string         `json:"params,omitempty"`
	Index      *int           `json:"index,omitempty"`
	CTT        string         `json:"ctt,omitempty"`
	Extra      map[string]any `json:"-"`
}
