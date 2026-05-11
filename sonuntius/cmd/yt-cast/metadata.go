// Maps to: N/A — Go-only YouTube metadata resolver.
//
// The Phase 2 yt-cast-receiver port stubs the upstream
// DefaultPlaylistRequestHandler (which fetches video metadata via the
// Node-only `youtubei.js` library), so every Player.play() call sees an
// otherwise-bare Video struct that only carries the 11-char video ID.
//
// This file resolves the human-readable title + channel via YouTube's
// public oEmbed endpoint — a single HTTP GET that returns a small JSON
// envelope, no auth, no third-party SDK:
//
//   GET https://www.youtube.com/oembed?url=https://www.youtube.com/watch?v=<id>&format=json
//
// Results are cached in-process so repeated plays of the same video do
// not re-fetch. Failures are best-effort: callers log the bare ID when
// resolution fails, the play path itself is never blocked.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// videoMetadata is the small subset of the oEmbed payload we surface.
type videoMetadata struct {
	Title           string
	Channel         string
	ThumbnailURL    string
	ThumbnailWidth  int
	ThumbnailHeight int
}

// oEmbedResponse mirrors the YouTube oEmbed JSON envelope fields we care
// about. Extra fields are ignored.
type oEmbedResponse struct {
	Title           string `json:"title"`
	AuthorName      string `json:"author_name"`
	ThumbnailURL    string `json:"thumbnail_url"`
	ThumbnailWidth  int    `json:"thumbnail_width"`
	ThumbnailHeight int    `json:"thumbnail_height"`
}

// metadataResolver fetches and caches YouTube video metadata.
type metadataResolver struct {
	client *http.Client

	mu    sync.Mutex
	cache map[string]videoMetadata
}

// newMetadataResolver constructs a resolver with the default HTTP timeout.
func newMetadataResolver() *metadataResolver {
	return &metadataResolver{
		client: &http.Client{Timeout: 5 * time.Second},
		cache:  make(map[string]videoMetadata),
	}
}

// Resolve returns the metadata for videoID. On cache hit it returns
// immediately; on miss it issues a single oEmbed request. Errors are
// returned verbatim so callers can log the cause and fall back to the
// bare ID.
func (r *metadataResolver) Resolve(ctx context.Context, videoID string) (videoMetadata, error) {
	if videoID == "" {
		return videoMetadata{}, errors.New("metadata: empty video id")
	}
	r.mu.Lock()
	if m, ok := r.cache[videoID]; ok {
		r.mu.Unlock()
		return m, nil
	}
	r.mu.Unlock()

	target := "https://www.youtube.com/oembed?url=" +
		url.QueryEscape("https://www.youtube.com/watch?v="+videoID) +
		"&format=json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return videoMetadata{}, err
	}
	req.Header.Set("Accept", "application/json")
	// User-Agent some YouTube oEmbed responders block empty UAs.
	req.Header.Set("User-Agent", "sonuntius-yt-cast/0.1 (+https://github.com/ShobuPrime/home-assistant-apps)")

	resp, err := r.client.Do(req)
	if err != nil {
		return videoMetadata{}, fmt.Errorf("oembed fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		// Read a short snippet for the error message — the rest is
		// usually an HTML 404 page, never useful in the addon log.
		snip, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return videoMetadata{}, fmt.Errorf("oembed HTTP %d: %s", resp.StatusCode, snip)
	}
	var env oEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return videoMetadata{}, fmt.Errorf("oembed decode: %w", err)
	}
	m := videoMetadata{
		Title:           env.Title,
		Channel:         env.AuthorName,
		ThumbnailURL:    env.ThumbnailURL,
		ThumbnailWidth:  env.ThumbnailWidth,
		ThumbnailHeight: env.ThumbnailHeight,
	}

	r.mu.Lock()
	r.cache[videoID] = m
	r.mu.Unlock()
	return m, nil
}
