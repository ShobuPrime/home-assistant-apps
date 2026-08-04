// Maps to: src/lib/app/RPCConnection.ts
//
// RPCConnection wraps the GET `/bind` long-poll. Upstream uses
// `fetch + line-by-line` to consume the chunked response body and emits
// parsed messages via an EventEmitter. The Go port keeps the same shape
// with two callbacks (`OnMessages` and `OnTerminate`) and an internal
// goroutine that drives the reader. Auto-reconnect on remote close is
// preserved — the connection retries up to MAX_RETRIES on outbound dial
// failures, then surfaces the error to `OnTerminate`.
package lounge

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/shobuprime/sonuntius/internal/ytcast/constants"
	"github.com/shobuprime/sonuntius/internal/ytcast/logger"
	"github.com/shobuprime/sonuntius/internal/ytcast/yterrors"
)

// rpcMaxRetries mirrors upstream's `MAX_RETRIES = 3`.
const rpcMaxRetries = 3

// rpcStatus tracks the upstream `#status` field exactly.
type rpcStatus int

const (
	rpcStatusDisconnected rpcStatus = iota
	rpcStatusConnecting
	rpcStatusConnected
	rpcStatusReconnecting
	rpcStatusDisconnecting
)

// RPCConnectionOptions ports the upstream `{ bindParams, logger }`
// constructor input.
type RPCConnectionOptions struct {
	BindParams *BindParams
	Logger     logger.Logger
	// HTTPClient is optional; if nil, http.DefaultClient is used. Tests
	// inject a stub here to drive the line reader without real network
	// I/O.
	HTTPClient *http.Client
	// BaseURL overrides the bind URL — defaults to constants.URLBind.
	// Tests use this to point at a local httptest server.
	BaseURL string
}

// RPCConnection ports the upstream `class RPCConnection`.
type RPCConnection struct {
	bindParams *BindParams
	logger     logger.Logger
	client     *http.Client
	baseURL    string

	mu     sync.Mutex
	status rpcStatus
	cancel context.CancelFunc
	body   io.Closer

	// onMessages and onTerminate ports upstream's `messages` /
	// `terminate` listeners. Set with SetOnMessages / SetOnTerminate
	// before Connect. The callbacks fire from the reader goroutine; the
	// orchestrator should not block them.
	onMessages  func(messages []*Message)
	onTerminate func(err error)
}

// NewRPCConnection constructs an RPCConnection. The connection is
// disconnected until Connect is called.
func NewRPCConnection(opts RPCConnectionOptions) *RPCConnection {
	client := opts.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	base := opts.BaseURL
	if base == "" {
		base = constants.URLBind
	}
	return &RPCConnection{
		bindParams: opts.BindParams,
		logger:     opts.Logger,
		client:     client,
		baseURL:    base,
		status:     rpcStatusDisconnected,
	}
}

// SetOnMessages registers the callback invoked when a batch of messages
// is parsed from the stream.
func (c *RPCConnection) SetOnMessages(fn func(messages []*Message)) {
	c.mu.Lock()
	c.onMessages = fn
	c.mu.Unlock()
}

// SetOnTerminate registers the callback invoked when the connection has
// failed terminally (max retries exhausted).
func (c *RPCConnection) SetOnTerminate(fn func(err error)) {
	c.mu.Lock()
	c.onTerminate = fn
	c.mu.Unlock()
}

// Connect ports `connect()`. It is a no-op if the connection isn't
// disconnected. Returns nil once the long-poll is established. The
// caller's context, if cancelled, aborts the dial.
func (c *RPCConnection) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.status != rpcStatusDisconnected {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()
	return c.doConnect(ctx, false, 0)
}

// doConnect ports `#doConnect`. It dials the bind URL, sets up the line
// reader, and (on success) spawns the reader goroutine.
func (c *RPCConnection) doConnect(ctx context.Context, isReconnect bool, retry int) error {
	c.mu.Lock()
	if isReconnect {
		c.status = rpcStatusReconnecting
	} else {
		c.status = rpcStatusConnecting
	}
	c.mu.Unlock()

	qs, err := c.bindParams.ToQueryString(QueryStringTypeRPC, nil)
	if err != nil {
		c.mu.Lock()
		c.status = rpcStatusDisconnected
		c.mu.Unlock()
		return err
	}
	url := fmt.Sprintf("%s?%s", c.baseURL, qs)
	c.debug("[yt-cast-receiver] Connecting to RPC URL:", url)

	dialCtx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.cancel = cancel
	c.mu.Unlock()

	req, err := http.NewRequestWithContext(dialCtx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		c.mu.Lock()
		c.cancel = nil
		c.status = rpcStatusDisconnected
		c.mu.Unlock()
		return err
	}
	resp, err := c.client.Do(req)
	c.mu.Lock()
	c.cancel = nil
	c.mu.Unlock()
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(dialCtx.Err(), context.Canceled) {
			c.debug("[yt-cast-receiver] RPC connection request aborted.")
			c.mu.Lock()
			c.status = rpcStatusDisconnected
			c.mu.Unlock()
			return yterrors.NewAbortError("RPC connection request aborted", url)
		}
		c.error("[yt-cast-receiver] RPC connection error:", err)
		retry++
		if retry <= rpcMaxRetries {
			c.error(fmt.Sprintf("[yt-cast-receiver] Retrying %d / %d", retry, rpcMaxRetries))
			return c.doConnect(ctx, isReconnect, retry)
		}
		c.mu.Lock()
		c.status = rpcStatusDisconnected
		c.mu.Unlock()
		c.error("[yt-cast-receiver] Max retries reached. Giving up...")
		return yterrors.NewConnectionError("RPC connection error", url, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 || resp.Body == nil {
		statusText := http.StatusText(resp.StatusCode)
		_ = resp.Body.Close()
		c.mu.Lock()
		c.status = rpcStatusDisconnected
		c.mu.Unlock()
		return yterrors.NewBadResponseError(
			"RPC connection request returned bad response",
			url,
			yterrors.HTTPResponse{Status: resp.StatusCode, StatusText: statusText},
		)
	}

	c.debug("[yt-cast-receiver] RPC connection established.")
	c.mu.Lock()
	c.status = rpcStatusConnected
	c.body = resp.Body
	c.mu.Unlock()

	go c.readLoop(ctx, resp.Body)
	return nil
}

// readLoop drains the response body, parsing each line into messages and
// invoking onMessages. Mirrors the upstream `line-by-line` reader plus
// the `end` / `error` handlers that funnel into `#handleDisconnect`.
func (c *RPCConnection) readLoop(ctx context.Context, body io.ReadCloser) {
	defer func() { _ = body.Close() }()
	scanner := bufio.NewScanner(body)
	// Lounge frames can be large (full state syncs). Bump the scanner
	// buffer to 1 MiB max line — upstream has no documented cap but the
	// Node `line-by-line` reader supports arbitrarily long lines.
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		c.mu.Lock()
		if c.status != rpcStatusConnected {
			c.mu.Unlock()
			return
		}
		cb := c.onMessages
		c.mu.Unlock()

		msgs := ParseIncoming(line)
		if len(msgs) > 0 && cb != nil {
			cb(msgs)
		}
	}
	scanErr := scanner.Err()
	if scanErr != nil {
		c.error("[yt-cast-receiver] RPC connection reader error:", scanErr)
	}
	c.handleDisconnect(ctx)
}

// handleDisconnect ports `#handleDisconnect`. If the previous status was
// `connected`, we silently reconnect. Otherwise the connection winds
// down quietly.
func (c *RPCConnection) handleDisconnect(ctx context.Context) {
	c.mu.Lock()
	if c.status == rpcStatusDisconnected {
		c.mu.Unlock()
		return
	}
	prev := c.status
	c.status = rpcStatusDisconnected
	c.body = nil
	c.mu.Unlock()

	if prev == rpcStatusConnected {
		c.debug("[yt-cast-receiver] RPC connection disconnected. Reconnecting...")
		go func() {
			if err := c.doConnect(ctx, true, 0); err != nil {
				c.mu.Lock()
				cb := c.onTerminate
				c.mu.Unlock()
				if cb != nil {
					cb(err)
				}
			}
		}()
	} else {
		c.debug("[yt-cast-receiver] RPC connection closed.")
	}
}

// Close ports `close()` — cancels the in-flight request (if any) and
// closes the body, transitioning to disconnected.
func (c *RPCConnection) Close() {
	c.mu.Lock()
	status := c.status
	cancel := c.cancel
	body := c.body
	if status == rpcStatusConnected || status == rpcStatusConnecting || status == rpcStatusReconnecting {
		c.status = rpcStatusDisconnecting
	}
	c.mu.Unlock()

	if status == rpcStatusConnected || status == rpcStatusConnecting || status == rpcStatusReconnecting {
		c.debug("[yt-cast-receiver] Closing RPC connection...")
	}
	if cancel != nil {
		cancel()
	}
	if body != nil {
		_ = body.Close()
	}
}

// Status returns the current connection status. Exposed for tests; the
// upstream class does not expose a getter.
func (c *RPCConnection) Status() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch c.status {
	case rpcStatusDisconnected:
		return "disconnected"
	case rpcStatusConnecting:
		return "connecting"
	case rpcStatusConnected:
		return "connected"
	case rpcStatusReconnecting:
		return "reconnecting"
	case rpcStatusDisconnecting:
		return "disconnecting"
	}
	return "unknown"
}

func (c *RPCConnection) debug(args ...any) {
	if c.logger != nil {
		c.logger.Debug(args...)
	}
}

func (c *RPCConnection) error(args ...any) {
	if c.logger != nil {
		c.logger.Error(args...)
	}
}
