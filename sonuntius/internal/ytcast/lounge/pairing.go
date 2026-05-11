// Maps to: src/lib/app/PairingCodeRequestService.ts
//
// PairingCodeRequestService periodically fetches a fresh pairing code
// from `URL_GET_PAIRING_CODE` so the user can type it into the
// "Link with TV code" page on a phone or desktop. Upstream refreshes
// every 5 minutes (or 30 seconds when the API isn't ready — i.e. before
// we have a lounge token and screen id).
//
// The Go port runs the same loop in a goroutine. Lifecycle controls are
// Start / Stop, mirroring upstream; on error the service stops and
// surfaces the error through the callback (which the Session forwards to
// the lounge event bus).
package lounge

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/shobuprime/sonuntius/internal/ytcast/constants"
	"github.com/shobuprime/sonuntius/internal/ytcast/logger"
	"github.com/shobuprime/sonuntius/internal/ytcast/yterrors"
)

// pairingRefreshInterval ports upstream's `REFRESH_INTERVAL`.
const pairingRefreshInterval = 5 * time.Minute

// pairingRefreshIntervalShort ports upstream's `REFRESH_INTERVAL_SHORT`.
const pairingRefreshIntervalShort = 30 * time.Second

// PairingCodeStatus mirrors upstream's restricted union of `RUNNING |
// STOPPED`.
type PairingCodeStatus int

const (
	PairingCodeStatusStopped PairingCodeStatus = iota
	PairingCodeStatusRunning
)

// PairingCodeListener carries the event hooks upstream emits via
// EventEmitter. The Session wires defaults that forward to the lounge
// event bus.
type PairingCodeListener struct {
	OnRequest  func()
	OnResponse func(code string)
	OnError    func(err error)
}

// PairingCodeRequestService ports the upstream class.
type PairingCodeRequestService struct {
	screen     *Screen
	bindParams *BindParams
	client     *http.Client
	baseURL    string
	logger     logger.Logger

	mu       sync.Mutex
	status   PairingCodeStatus
	listener PairingCodeListener
	cancel   context.CancelFunc
}

// PairingCodeRequestServiceOptions mirrors the upstream constructor input
// plus a few Go-only knobs for tests.
type PairingCodeRequestServiceOptions struct {
	Screen     *Screen
	BindParams *BindParams
	Logger     logger.Logger
	HTTPClient *http.Client
	BaseURL    string
}

// NewPairingCodeRequestService constructs a service in the stopped
// state.
func NewPairingCodeRequestService(opts PairingCodeRequestServiceOptions) *PairingCodeRequestService {
	client := opts.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	base := opts.BaseURL
	if base == "" {
		base = constants.URLGetPairingCode
	}
	return &PairingCodeRequestService{
		screen:     opts.Screen,
		bindParams: opts.BindParams,
		client:     client,
		baseURL:    base,
		logger:     opts.Logger,
		status:     PairingCodeStatusStopped,
	}
}

// SetListener replaces the registered callbacks.
func (s *PairingCodeRequestService) SetListener(l PairingCodeListener) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listener = l
}

// Start ports `start()`. Calling Start while already running is a no-op.
func (s *PairingCodeRequestService) Start() {
	s.mu.Lock()
	if s.status == PairingCodeStatusRunning {
		s.mu.Unlock()
		return
	}
	s.status = PairingCodeStatusRunning
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.mu.Unlock()

	go s.runLoop(ctx)
}

// Stop ports `stop()`.
func (s *PairingCodeRequestService) Stop() {
	s.mu.Lock()
	if s.status == PairingCodeStatusStopped {
		s.mu.Unlock()
		return
	}
	s.status = PairingCodeStatusStopped
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.mu.Unlock()
}

// Status returns the current lifecycle status.
func (s *PairingCodeRequestService) Status() PairingCodeStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// runLoop is the goroutine body. It fetches a code, dispatches the
// `response` event, and waits for the configured interval before doing
// it again. On error, it stops the service.
func (s *PairingCodeRequestService) runLoop(ctx context.Context) {
	// Fetch immediately, mirroring upstream's `void this.#getCodeAndEmit()`
	// inside Start().
	interval := s.fetchOnce(ctx)
	for {
		if interval <= 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
		interval = s.fetchOnce(ctx)
	}
}

// fetchOnce performs a single fetch and returns the wait interval to use
// before the next fetch. Returns 0 if the service should stop.
func (s *PairingCodeRequestService) fetchOnce(ctx context.Context) time.Duration {
	s.mu.Lock()
	if s.status == PairingCodeStatusStopped {
		s.mu.Unlock()
		return 0
	}
	bp := s.bindParams
	scr := s.screen
	listener := s.listener
	s.mu.Unlock()

	if bp == nil || scr == nil || bp.LoungeIDToken == "" || scr.ID == "" {
		// API not ready; check back sooner.
		return pairingRefreshIntervalShort
	}

	form := url.Values{}
	form.Set("access_type", "permanent")
	form.Set("app", scr.App)
	form.Set("lounge_token", bp.LoungeIDToken)
	form.Set("screen_id", scr.ID)
	form.Set("screen_name", scr.Name)
	form.Set("device_id", bp.ID)

	if listener.OnRequest != nil {
		listener.OnRequest()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL, strings.NewReader(form.Encode()))
	if err != nil {
		s.fail(yterrors.NewConnectionError("Connection error in fetching pairing code", s.baseURL, err))
		return 0
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return 0
		}
		s.fail(yterrors.NewConnectionError("Connection error in fetching pairing code", s.baseURL, err))
		return 0
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.fail(yterrors.NewDataError("Pairing code data error", err, nil))
		return 0
	}
	code := strings.TrimSpace(string(body))
	if listener.OnResponse != nil {
		listener.OnResponse(code)
	}
	return pairingRefreshInterval
}

// fail mirrors upstream's `this.stop(); this.emit('error', error);`
// ordering.
func (s *PairingCodeRequestService) fail(err error) {
	s.Stop()
	s.mu.Lock()
	listener := s.listener
	s.mu.Unlock()
	if listener.OnError != nil {
		listener.OnError(err)
	}
}
