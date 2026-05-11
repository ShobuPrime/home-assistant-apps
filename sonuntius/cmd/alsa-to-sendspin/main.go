// Maps to: N/A — Go-only Sendspin client for the Phase 5 Tidal Connect
// binary fallback path.
//
// Pipeline architecture:
//
//	iFi tidal_connect_application
//	    → writes PCM to ALSA Loopback (hw:Loopback,0)
//	        → arecord reads the capture side (hw:Loopback,1)
//	            → this Go binary forwards PCM frames to the Sendspin
//	              server over WebSocket
//
// The Go binary execs `arecord` as a child process (stdlib `os/exec`),
// pipes its stdout into the WebSocket loop, and forwards bytes to the
// configured Sendspin server URL. Signals (SIGINT / SIGTERM) propagate
// to the arecord child so shutdown is clean.
//
// IMPORTANT — Sendspin frame format:
//
// At the time of writing the Sendspin (formerly Resonate) wire format is
// not finalized in a public spec. This binary currently emits raw PCM
// bytes as WebSocket binary messages, framed by `--bytes-per-frame` to
// keep latency bounded. When the Sendspin spec lands at
// github.com/Sendspin/spec, update `encodeFrame()` (the only place the
// raw bytes are touched on the wire) to wrap PCM in whatever envelope
// the server expects. Everything else — the arecord pipeline, signal
// handling, reconnect-with-backoff — stays the same.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/websocket"

	"github.com/shobuprime/sonuntius/internal/config"
)

type runtimeOptions struct {
	Enabled         bool
	SendspinURL     string
	CaptureDevice   string
	PCMFormat       string
	PCMRate         int
	PCMChannels     int
	BytesPerFrame   int
	ArecordPath     string
	LogLevel        slog.Level
	ReconnectFloor  time.Duration
	ReconnectCeil   time.Duration
}

func main() {
	// Parse CLI flags (override env / config when supplied).
	var (
		captureDevice = flag.String("capture-device", "", "ALSA capture device (default from config / hw:Loopback,1,0)")
		sendspinURL   = flag.String("sendspin-url", "", "Sendspin server WebSocket URL (default from config)")
		pcmFormat     = flag.String("pcm-format", "S16_LE", "PCM format passed to arecord -f")
		pcmRate       = flag.Int("pcm-rate", 44100, "PCM sample rate (Hz) passed to arecord -r")
		pcmChannels   = flag.Int("pcm-channels", 2, "PCM channel count passed to arecord -c")
		bytesPerFrame = flag.Int("bytes-per-frame", 4096, "Bytes to accumulate per WebSocket frame")
		arecordPath   = flag.String("arecord", "arecord", "arecord binary path")
	)
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	opts, err := config.Load(ctx, logger)
	if err != nil {
		logger.Error("config: load failed", "err", err)
		os.Exit(2)
	}
	level := config.ResolveLogLevel(opts.LogLevel)
	logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	if !opts.TidalFallback.Enabled {
		// The S6 run script already gates on this, but keep the binary
		// itself defensive so manual `docker exec` of the binary doesn't
		// inadvertently flood logs.
		logger.Info("alsa-to-sendspin: tidal_fallback.enabled is false — exiting cleanly")
		// Sleep so S6 doesn't restart-loop on the empty exit.
		<-ctx.Done()
		return
	}

	r := runtimeOptions{
		Enabled:        true,
		SendspinURL:    firstNonEmpty(*sendspinURL, opts.TidalFallback.SendspinServerURL),
		CaptureDevice:  firstNonEmpty(*captureDevice, "hw:Loopback,1,0"),
		PCMFormat:      *pcmFormat,
		PCMRate:        *pcmRate,
		PCMChannels:    *pcmChannels,
		BytesPerFrame:  *bytesPerFrame,
		ArecordPath:    *arecordPath,
		LogLevel:       level,
		ReconnectFloor: 2 * time.Second,
		ReconnectCeil:  60 * time.Second,
	}

	if r.SendspinURL == "" {
		logger.Error("alsa-to-sendspin: sendspin_server_url is empty — cannot start. Set tidal_fallback.sendspin_server_url in addon options.")
		// Stay alive so S6 doesn't restart-loop on a misconfiguration.
		<-ctx.Done()
		return
	}

	if _, err := exec.LookPath(r.ArecordPath); err != nil {
		logger.Error("alsa-to-sendspin: arecord not found on PATH", "err", err, "path", r.ArecordPath)
		<-ctx.Done()
		return
	}

	logger.Info("alsa-to-sendspin: starting",
		"capture_device", r.CaptureDevice,
		"sendspin_url", r.SendspinURL,
		"pcm_format", r.PCMFormat,
		"pcm_rate", r.PCMRate,
		"pcm_channels", r.PCMChannels,
		"bytes_per_frame", r.BytesPerFrame,
	)

	runForwarder(ctx, r, logger)
	logger.Info("alsa-to-sendspin: shutting down")
}

// runForwarder is the reconnect-with-backoff outer loop. Each iteration
// opens a fresh arecord child and a fresh WebSocket and pumps until one
// of them errors.
func runForwarder(ctx context.Context, r runtimeOptions, logger *slog.Logger) {
	backoff := r.ReconnectFloor
	for {
		if ctx.Err() != nil {
			return
		}
		err := runForwarderOnce(ctx, r, logger)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			logger.Warn("forwarder cycle ended with error", "err", err, "retry_in", backoff)
		} else {
			logger.Info("forwarder cycle ended cleanly", "retry_in", backoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > r.ReconnectCeil {
			backoff = r.ReconnectCeil
		}
	}
}

func runForwarderOnce(ctx context.Context, r runtimeOptions, logger *slog.Logger) error {
	// Open the WebSocket first. If the Sendspin server is unreachable,
	// we don't want to spawn an arecord child that has nowhere to write.
	wsCfg, err := websocket.NewConfig(r.SendspinURL, wsOrigin(r.SendspinURL))
	if err != nil {
		return fmt.Errorf("ws config: %w", err)
	}
	conn, err := websocket.DialConfig(wsCfg)
	if err != nil {
		return fmt.Errorf("ws dial: %w", err)
	}
	defer conn.Close()
	logger.Info("sendspin: WebSocket connected", "url", r.SendspinURL)

	// Send a JSON hello frame the server can use to negotiate format /
	// session id. Schema is a placeholder — see file-level docstring.
	hello := map[string]any{
		"type":          "hello",
		"client":        "sonuntius/alsa-to-sendspin",
		"format":        r.PCMFormat,
		"rate":          r.PCMRate,
		"channels":      r.PCMChannels,
		"bytes_per_frame": r.BytesPerFrame,
		"sent_at":       time.Now().UTC().Format(time.RFC3339Nano),
	}
	if helloBytes, mErr := json.Marshal(hello); mErr == nil {
		if _, wErr := conn.Write(helloBytes); wErr != nil {
			return fmt.Errorf("ws hello: %w", wErr)
		}
	}

	// Spawn arecord. Child receives the same context, so ctx cancellation
	// or a WS write error terminates it (we hold a CancelFunc that closes
	// both paths).
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(childCtx, r.ArecordPath,
		"-D", r.CaptureDevice,
		"-f", r.PCMFormat,
		"-r", fmt.Sprintf("%d", r.PCMRate),
		"-c", fmt.Sprintf("%d", r.PCMChannels),
		"-t", "raw",
		"--quiet",
	)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("arecord stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("arecord start: %w", err)
	}
	logger.Info("arecord: started", "pid", cmd.Process.Pid)

	// Forward stdout → WebSocket. We accumulate up to BytesPerFrame bytes
	// per WS message to keep frame count manageable while still keeping
	// latency low.
	pumpErr := pumpPCM(childCtx, stdout, conn, r, logger)

	// Make sure arecord is gone before we return so the next iteration
	// can re-open the same device without an EBUSY.
	cancel()
	waitErr := cmd.Wait()
	if waitErr != nil && childCtx.Err() == nil {
		logger.Warn("arecord exited", "err", waitErr)
	}

	return pumpErr
}

// pumpPCM reads from r and writes binary messages to w. encodeFrame is
// the one place the wire format lives — when the Sendspin spec is
// finalized, change just that function.
func pumpPCM(ctx context.Context, r io.Reader, w io.Writer, opts runtimeOptions, logger *slog.Logger) error {
	buf := make([]byte, opts.BytesPerFrame)
	var totalBytes uint64
	var logOnce sync.Once
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, rerr := io.ReadFull(r, buf)
		if n > 0 {
			frame := encodeFrame(buf[:n])
			if _, werr := w.Write(frame); werr != nil {
				return fmt.Errorf("ws write: %w", werr)
			}
			totalBytes += uint64(n)
			logOnce.Do(func() {
				logger.Info("sendspin: first PCM frame forwarded", "bytes", n)
			})
		}
		if rerr != nil {
			if rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
				return fmt.Errorf("arecord stdout closed after %d bytes", totalBytes)
			}
			return fmt.Errorf("arecord stdout: %w", rerr)
		}
	}
}

// encodeFrame is the wire-format hook. The current implementation passes
// raw PCM through as-is so the user can verify the rest of the pipeline
// end-to-end. When the Sendspin frame envelope is documented, update
// this function — it is the only call site that touches outgoing bytes.
func encodeFrame(pcm []byte) []byte {
	return pcm
}

func wsOrigin(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "http://localhost"
	}
	scheme := "http"
	if u.Scheme == "wss" {
		scheme = "https"
	}
	return scheme + "://" + filepath.Base(u.Host)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
