// Command ha-lemonade-bridge is the Home Assistant sidecar for Lemonade that sits in front of
// Lemonade's server (lemond) and gives it long-term memory backed by MuninnDB.
//
// Lemonade has no memory of its own, and Home Assistant only keeps a short
// rolling window (20 messages / 1 hour) per conversation, so nothing survives
// across sessions. This proxy closes that gap without requiring a custom Home
// Assistant integration: it speaks no protocol of its own, it just observes
// chat traffic on its way through.
//
//	Home Assistant ──► bridge   :13305 ──► lemond 127.0.0.1:13306
//	                       │
//	                       ├─ before: ACTIVATE MuninnDB, inject top-N memories
//	                       └─ after:  store the exchange (async, fire-and-forget)
//
// Everything that is not a chat completion — the web UI, /live, /api/tags,
// /mcp, WebSocket upgrades — is proxied through untouched.
//
// Design rule: memory is best-effort. Every failure path falls back to plain
// pass-through, because an assistant that stops answering when its memory
// store is unreachable is worse than one that briefly forgets.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const maxRequestBody = 32 << 20 // 32 MiB; vision requests carry base64 images

func logf(format string, args ...any) {
	log.Printf(format, args...)
}

type config struct {
	listen   string
	upstream string

	memoryEnabled bool
	memoryURL     string
	memoryVault   string
	memoryAPIKey  string
	recallCount   int
	recallTokens  int
	timeout       time.Duration
	minChars      int

	// Speech-to-text bridge (see wyoming.go). Home Assistant discovers speech
	// engines over Wyoming, so this is what puts Lemonade in the
	// Speech-to-text dropdown.
	sttEnabled bool
	sttListen  string
	sttModel   string
	sttTimeout time.Duration

	// See origins.go.
	allowedOrigins  []string
	supervisorAPI   string
	supervisorToken string
}

func envStr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		logf("config: %s=%q is not a number, using %d", key, v, def)
	}
	return def
}

func envBool(key string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	}
	return def
}

func loadConfig() config {
	return config{
		listen:        envStr("BRIDGE_LISTEN", ":13305"),
		upstream:      envStr("BRIDGE_UPSTREAM", "http://127.0.0.1:13306"),
		memoryEnabled: envBool("MEMORY_ENABLED", false),
		memoryURL:     envStr("MEMORY_URL", ""),
		memoryVault:   envStr("MEMORY_VAULT", "home_assistant"),
		memoryAPIKey:  envStr("MEMORY_API_KEY", ""),
		recallCount:   envInt("MEMORY_RECALL_COUNT", 3),
		recallTokens:  envInt("MEMORY_RECALL_TOKENS", 300),
		timeout:       time.Duration(envInt("MEMORY_TIMEOUT_MS", 400)) * time.Millisecond,
		minChars:      envInt("MEMORY_MIN_CHARS", 16),

		sttEnabled: envBool("STT_ENABLED", false),
		sttListen:  envStr("STT_LISTEN", ":10600"),
		sttModel:   envStr("STT_MODEL", "Moonshine-Small-Streaming"),
		// Generous: a cold model load costs several seconds, and a voice
		// pipeline that gives up mid-load is worse than one that waits.
		sttTimeout: time.Duration(envInt("STT_TIMEOUT_S", 120)) * time.Second,

		allowedOrigins:  splitList(envStr("BRIDGE_ALLOWED_ORIGINS", "")),
		supervisorAPI:   envStr("SUPERVISOR_API", "http://supervisor"),
		supervisorToken: envStr("SUPERVISOR_TOKEN", ""),
	}
}

// splitList parses a comma-separated env value. Spaces are trimmed, not treated
// as separators.
func splitList(s string) []string {
	var out []string
	for part := range strings.SplitSeq(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// chatPaths are the endpoints carrying a chat exchange worth remembering.
// Lemonade serves the OpenAI surface under both /v1 and /api/v1.
var chatPaths = map[string]string{
	"/v1/chat/completions":     "openai",
	"/api/v1/chat/completions": "openai",
	"/api/chat":                "ollama",
}

type server struct {
	cfg     config
	proxy   *httputil.ReverseProxy
	client  *http.Client
	mem     *Memory
	target  *url.URL
	origins *originGate
}

func main() {
	log.SetFlags(0)
	cfg := loadConfig()

	target, err := url.Parse(cfg.upstream)
	if err != nil {
		log.Fatalf("bridge: invalid upstream %q: %v", cfg.upstream, err)
	}

	srv := &server{
		cfg:     cfg,
		target:  target,
		proxy:   newSingleHostProxy(target),
		origins: newOriginGate(cfg.allowedOrigins),
		client: &http.Client{
			// No global timeout: streaming completions are long-lived. The
			// per-phase timeouts below bound the parts that can actually hang.
			Transport: &http.Transport{
				DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
				ResponseHeaderTimeout: 10 * time.Minute,
				MaxIdleConns:          32,
				MaxIdleConnsPerHost:   16,
				IdleConnTimeout:       90 * time.Second,
			},
		},
	}

	srv.proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if errors.Is(err, context.Canceled) {
			return // client hung up; normal
		}
		logf("proxy: %s %s: %v", r.Method, r.URL.Path, err)
		w.WriteHeader(http.StatusBadGateway)
	}

	if cfg.memoryEnabled && cfg.memoryURL != "" {
		srv.mem = NewMemory(cfg.memoryURL, cfg.memoryVault, cfg.memoryAPIKey,
			cfg.recallCount, cfg.recallTokens, cfg.timeout)
		logf("bridge: memory ENABLED — vault %q at %s (recall top-%d, %d token budget, %s timeout)",
			cfg.memoryVault, cfg.memoryURL, cfg.recallCount, cfg.recallTokens, cfg.timeout)
	} else {
		logf("bridge: memory disabled — pure pass-through")
	}
	// Speech-to-text bridge. Started before the HTTP server so a failure to
	// bind is reported at boot rather than on the user's first voice command.
	if cfg.sttEnabled {
		startWyoming(wyomingConfig{
			listen:   cfg.sttListen,
			upstream: cfg.upstream,
			model:    cfg.sttModel,
			apiKey:   envStr("LEMONADE_API_KEY", ""),
			timeout:  cfg.sttTimeout,
		})
	} else {
		logf("bridge: speech-to-text disabled")
	}

	logf("bridge: allowed browser origins at start: %s", strings.Join(srv.origins.list(), ", "))
	go srv.origins.watch(srv.client, cfg.supervisorAPI, cfg.supervisorToken,
		5*time.Second, 5*time.Minute)

	logf("bridge: listening on %s, upstream %s", cfg.listen, cfg.upstream)

	httpSrv := &http.Server{
		Addr:              cfg.listen,
		Handler:           srv,
		ReadHeaderTimeout: 30 * time.Second,
	}

	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
		<-stop
		if srv.mem != nil {
			logf("bridge: shutting down (%s)", srv.mem.Stats())
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
	}()

	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("bridge: %v", err)
	}
}

// newSingleHostProxy builds the pass-through proxy used for everything that is
// not a chat completion: the web UI, /live, /api/tags, /mcp, and WebSocket
// upgrades (ReverseProxy handles 101 Switching Protocols natively).
func newSingleHostProxy(target *url.URL) *httputil.ReverseProxy {
	return httputil.NewSingleHostReverseProxy(target)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Gate every method and path. Navigations and asset loads send no Origin and
	// pass; fetch/XHR/WebSocket always send one.
	if !s.origins.allowed(r.Header.Get("Origin")) {
		rejectOrigin(w, r.Header.Get("Origin"))
		return
	}

	flavour, isChat := chatPaths[r.URL.Path]
	if !isChat || r.Method != http.MethodPost || s.mem == nil {
		s.proxy.ServeHTTP(w, r)
		return
	}
	s.handleChat(w, r, flavour)
}

// handleChat proxies a chat request by hand so the body can be augmented on
// the way in and observed on the way out. Any error before the upstream call
// degrades to a plain proxied request.
func (s *server) handleChat(w http.ResponseWriter, r *http.Request, flavour string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBody))
	if err != nil {
		logf("chat: reading request body: %v", err)
		http.Error(w, "cannot read request body", http.StatusBadRequest)
		return
	}
	_ = r.Body.Close()

	// Decode into a generic map so unknown fields (temperature, tools,
	// response_format, provider extensions...) survive the round trip
	// untouched. Only "messages" is ever rewritten.
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		// Not JSON we understand — hand it to the upstream unchanged.
		s.replayProxy(w, r, body)
		return
	}

	userText := lastUserText(payload)
	model, _ := payload["model"].(string)

	outBody := body
	if userText != "" {
		ctx, cancel := context.WithTimeout(r.Context(), s.cfg.timeout)
		block := s.mem.Recall(ctx, userText)
		cancel()

		if block != "" {
			if injectSystem(payload, block) {
				if b, err := json.Marshal(payload); err == nil {
					outBody = b
				} else {
					logf("chat: re-encoding augmented payload: %v", err)
				}
			}
		}
	}

	resp, err := s.forward(r, outBody)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		logf("chat: upstream: %v", err)
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	copyHeaders(w.Header(), resp.Header)
	// Length changes when we inject, and streaming bodies have none anyway.
	w.Header().Del("Content-Length")
	w.WriteHeader(resp.StatusCode)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(w, resp.Body)
		return
	}

	// Whether the reply streams is decided by the REQUEST, not by sniffing
	// response headers: a non-streamed reply sent with chunked encoding and no
	// Content-Length is indistinguishable from a stream by headers alone, and
	// guessing wrong silently loses the captured text.
	var assistant string
	if wantsStream(payload) || isStream(resp) {
		assistant = s.streamAndCapture(w, resp.Body, flavour)
	} else {
		assistant = s.bufferAndCapture(w, resp.Body, flavour)
	}

	if len(userText) >= s.cfg.minChars && assistant != "" {
		s.mem.Store(userText, assistant, model)
	}
}

// replayProxy forwards a request whose body we already consumed.
func (s *server) replayProxy(w http.ResponseWriter, r *http.Request, body []byte) {
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	s.proxy.ServeHTTP(w, r)
}

func (s *server) forward(r *http.Request, body []byte) (*http.Response, error) {
	target := *s.target
	target.Path = r.URL.Path
	target.RawQuery = r.URL.RawQuery

	req, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, vs := range r.Header {
		if strings.EqualFold(k, "Content-Length") {
			continue
		}
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.ContentLength = int64(len(body))
	return s.client.Do(req)
}

// wantsStream reports whether the client asked for a streamed reply. Both the
// OpenAI and Ollama schemas spell this the same way.
func wantsStream(payload map[string]any) bool {
	b, _ := payload["stream"].(bool)
	return b
}

// isStream is the fallback for replies that stream without having been asked
// to (or when the request could not be parsed).
func isStream(resp *http.Response) bool {
	ct := resp.Header.Get("Content-Type")
	return strings.Contains(ct, "text/event-stream") ||
		strings.Contains(ct, "application/x-ndjson")
}

// streamAndCapture relays a streaming response verbatim, flushing every line so
// token-by-token output is not delayed, while accumulating the assistant text.
func (s *server) streamAndCapture(w http.ResponseWriter, body io.Reader, flavour string) string {
	rc := http.NewResponseController(w)
	reader := bufio.NewReaderSize(body, 16<<10)
	var sb strings.Builder

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if _, werr := w.Write(line); werr != nil {
				return sb.String() // client gone; keep what we have
			}
			_ = rc.Flush()
			if frag := extractDelta(line, flavour); frag != "" {
				sb.WriteString(frag)
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				logf("chat: stream read: %v", err)
			}
			return sb.String()
		}
	}
}

func (s *server) bufferAndCapture(w http.ResponseWriter, body io.Reader, flavour string) string {
	raw, err := io.ReadAll(io.LimitReader(body, maxRequestBody))
	if err != nil {
		logf("chat: reading response: %v", err)
	}
	if _, err := w.Write(raw); err != nil {
		return ""
	}
	return extractFull(raw, flavour)
}

func copyHeaders(dst, src http.Header) {
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func urlQueryEscape(s string) string { return url.QueryEscape(s) }

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
