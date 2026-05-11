// Maps to: N/A — Go-only YouTube stream URL resolver.
//
// Music Assistant's "URL" provider expects a direct audio stream URL
// (mp3, m4a, googlevideo.com, etc.) — it ffmpeg-probes whatever it
// receives. Handing it a raw YouTube watch URL fails with "Invalid
// data found when processing input" because MA gets HTML instead of
// audio bytes.
//
// We fix this by pre-resolving the watch URL to a direct stream URL
// via `yt-dlp -f bestaudio -g`. The resulting googlevideo.com URL is
// signed and valid for several hours, which is more than enough for
// any reasonable playback session. We do not cache the resolved URL
// across calls — re-resolving is cheap and avoids serving an expired
// signature on the next play.
//
// yt-dlp is installed in the addon image via `apk add yt-dlp`. The
// resolver shells out to the binary because there is no stable
// Go-native YouTube extractor maintained against current YouTube
// changes — yt-dlp's Python implementation is updated weekly and that
// is exactly what we need.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// streamResolver runs yt-dlp to extract a direct audio stream URL from
// a YouTube watch URL.
type streamResolver struct {
	Binary  string        // defaults to "yt-dlp" on PATH
	Timeout time.Duration // defaults to 15s
}

func newStreamResolver() *streamResolver {
	return &streamResolver{
		Binary:  "yt-dlp",
		Timeout: 15 * time.Second,
	}
}

// streamInfo bundles the resolver output: the direct audio stream URL
// and the source video's duration in seconds (0 when yt-dlp didn't
// surface a duration for this format).
type streamInfo struct {
	URL      string
	Duration float64
}

// Resolve returns a direct audio stream URL + duration for the given
// video id. Equivalent shell:
//
//	yt-dlp -f bestaudio -g --print '%(duration)s' <watch-url>
//
// The `-g` flag emits the format URL; `--print '%(duration)s'` adds
// the integer duration. yt-dlp emits the URL first, then the print
// values, on separate lines.
func (r *streamResolver) Resolve(ctx context.Context, videoID string) (streamInfo, error) {
	if videoID == "" {
		return streamInfo{}, errors.New("streamresolve: empty video id")
	}
	bin := r.Binary
	if bin == "" {
		bin = "yt-dlp"
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	subCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	watchURL := "https://www.youtube.com/watch?v=" + videoID
	cmd := exec.CommandContext(subCtx, bin,
		"--no-warnings",
		"--quiet",
		"-f", "bestaudio",
		"-g",
		"--print", "%(duration)s",
		watchURL,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return streamInfo{}, fmt.Errorf("streamresolve: yt-dlp failed: %s", truncateString(msg, 200))
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	var info streamInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "NA" {
			continue
		}
		if info.URL == "" && (strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://")) {
			info.URL = line
			continue
		}
		if info.Duration == 0 {
			if d, err := parseFloat(line); err == nil && d > 0 {
				info.Duration = d
			}
		}
	}
	if info.URL == "" {
		return streamInfo{}, errors.New("streamresolve: yt-dlp returned empty url")
	}
	return info, nil
}

// parseFloat is a thin wrapper to keep the package's import surface
// small; we don't pull strconv in elsewhere here.
func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
