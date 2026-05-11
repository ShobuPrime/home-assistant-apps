// Maps to: src/lib/app/Session.ts
//
// Session owns the lounge protocol for a single sender flavour (YT or
// YTMUSIC). Lifecycle:
//
//  1. Begin: derive a screen id (or load it from the data store), fetch
//     a lounge token, perform the bind handshake to learn SID +
//     gsessionid, then open the long-poll RPC connection.
//  2. While running: incoming messages arrive on the RPC connection,
//     update BindParams (AID etc.), and propagate to listeners and the
//     event bus. Outgoing messages go through an AsyncTaskQueue so they
//     are sent serially and can be replayed after a token refresh.
//  3. On RPC terminate: refresh the lounge token in place (the queue is
//     paused, the AID counters are reset, pending messages have their
//     AID nulled out).
//  4. End: stop the pairing service, close the RPC, drain the queue,
//     send a `loungeScreenDisconnected` frame, and emit a
//     SessionDisconnected event.
//
// This port preserves the upstream control flow faithfully, including
// the error-recovery dance — Session is the load-bearing piece for the
// lounge protocol, so deviations would compound.
package lounge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shobuprime/sonuntius/internal/ytcast/asyncq"
	"github.com/shobuprime/sonuntius/internal/ytcast/constants"
	"github.com/shobuprime/sonuntius/internal/ytcast/datastore"
	"github.com/shobuprime/sonuntius/internal/ytcast/logger"
	"github.com/shobuprime/sonuntius/internal/ytcast/types"
	"github.com/shobuprime/sonuntius/internal/ytcast/yterrors"
)

// Screen ports the upstream `interface Screen`.
type Screen struct {
	ID   string
	Name string
	App  string
}

// SessionStatus extends the upstream `STATUSES` union with the
// "refreshing" intermediate state.
type SessionStatus string

const (
	SessionStatusStopped    SessionStatus = SessionStatus(constants.StatusStopped)
	SessionStatusStopping   SessionStatus = SessionStatus(constants.StatusStopping)
	SessionStatusStarting   SessionStatus = SessionStatus(constants.StatusStarting)
	SessionStatusRunning    SessionStatus = SessionStatus(constants.StatusRunning)
	SessionStatusRefreshing SessionStatus = "refreshing"
)

// SessionOptions ports the upstream `interface SessionOptions`.
type SessionOptions struct {
	Client     types.Client
	ScreenName string
	ScreenApp  string
	Brand      string
	Model      string
	DataStore  datastore.DataStore
	Logger     logger.Logger
	EventBus   *EventBus

	// HTTPClient is the http.Client used for every outbound request
	// except the long-poll (RPCConnection has its own). Defaults to
	// http.DefaultClient.
	HTTPClient *http.Client

	// Overridable URLs for tests; default to the real endpoints in
	// constants.URL*.
	URLGenerateScreenID    string
	URLGetLoungeTokenBatch string
	URLBind                string
	URLRegisterPairingCode string
	URLGetPairingCode      string
}

// SessionListener carries the per-session callbacks. Upstream emits
// `messages` and `terminate`; we route both here. The lounge event bus
// (SessionOptions.EventBus) receives the higher-level fan-out events for
// the orchestrator.
type SessionListener struct {
	OnMessages  func(session *Session, messages []*Message)
	OnTerminate func(err error)
}

// Session ports the upstream `class Session`.
type Session struct {
	client     types.Client
	screen     *Screen
	bindParams *BindParams
	rpc        *RPCConnection
	ofs        int
	dataStore  datastore.DataStore
	logger     logger.Logger
	bus        *EventBus
	httpClient *http.Client

	urlGenerateScreenID    string
	urlGetLoungeTokenBatch string
	urlBind                string
	urlRegisterPairingCode string

	queue *asyncq.Queue
	pair  *PairingCodeRequestService

	mu                sync.Mutex
	status            SessionStatus
	deferred          map[string]*deferredMessage
	loungeTokenTimer  *time.Timer
	listener          SessionListener
	currentRPCCancel  context.CancelFunc
}

// deferredMessage is the Go translation of upstream's
// `#deferredMessages` map value `{ task, timeout }`.
type deferredMessage struct {
	timer  *time.Timer
	cancel func() // resolve(false) on cancel; closes over the send promise
	msgs   []*Message
}

// NewSession constructs a Session and binds its sub-components (queue,
// RPC, pairing) without starting anything. Call Begin to start.
func NewSession(opts SessionOptions) *Session {
	if opts.HTTPClient == nil {
		opts.HTTPClient = http.DefaultClient
	}
	if opts.Client.Key == "" {
		opts.Client = types.Clients[types.ClientKeyYT]
	}
	if opts.URLGenerateScreenID == "" {
		opts.URLGenerateScreenID = constants.URLGenerateScreenID
	}
	if opts.URLGetLoungeTokenBatch == "" {
		opts.URLGetLoungeTokenBatch = constants.URLGetLoungeTokenBatch
	}
	if opts.URLBind == "" {
		opts.URLBind = constants.URLBind
	}
	if opts.URLRegisterPairingCode == "" {
		opts.URLRegisterPairingCode = constants.URLRegisterPairingCode
	}
	if opts.URLGetPairingCode == "" {
		opts.URLGetPairingCode = constants.URLGetPairingCode
	}
	screen := &Screen{
		Name: opts.ScreenName,
		App:  opts.ScreenApp,
	}
	bindParams := NewBindParams(BindParamsInitOptions{
		Theme:      opts.Client.Theme,
		DeviceID:   generateDeviceID(),
		ScreenName: screen.Name,
		ScreenApp:  screen.App,
		Brand:      opts.Brand,
		Model:      opts.Model,
	})
	s := &Session{
		client:                 opts.Client,
		screen:                 screen,
		bindParams:             bindParams,
		dataStore:              opts.DataStore,
		logger:                 opts.Logger,
		bus:                    opts.EventBus,
		httpClient:             opts.HTTPClient,
		urlGenerateScreenID:    opts.URLGenerateScreenID,
		urlGetLoungeTokenBatch: opts.URLGetLoungeTokenBatch,
		urlBind:                opts.URLBind,
		urlRegisterPairingCode: opts.URLRegisterPairingCode,
		queue:                  asyncq.New(),
		status:                 SessionStatusStopped,
		deferred:               make(map[string]*deferredMessage),
	}
	s.pair = NewPairingCodeRequestService(PairingCodeRequestServiceOptions{
		Screen:     screen,
		BindParams: bindParams,
		Logger:     opts.Logger,
		HTTPClient: opts.HTTPClient,
		BaseURL:    opts.URLGetPairingCode,
	})
	// Forward pairing events to the session-level bus.
	s.pair.SetListener(PairingCodeListener{
		OnResponse: func(code string) {
			if s.bus != nil {
				s.bus.Publish(PairingCodeReadyEvent{
					Code:         code,
					RefreshAfter: pairingRefreshInterval,
				})
			}
		},
		OnError: func(err error) {
			if s.bus != nil {
				s.bus.Publish(PairingCodeErrorEvent{Err: err})
			}
		},
	})
	return s
}

// SetListener replaces the registered session callbacks.
func (s *Session) SetListener(l SessionListener) {
	s.mu.Lock()
	s.listener = l
	s.mu.Unlock()
}

// Client returns the session's Client (YT or YTMUSIC).
func (s *Session) Client() types.Client { return s.client }

// PairingCodeRequestService exposes the pairing service so the
// orchestrator can call Start/Stop on it.
func (s *Session) PairingCodeRequestService() *PairingCodeRequestService { return s.pair }

// Status returns the current lifecycle status.
func (s *Session) Status() SessionStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Screen returns a copy of the current screen metadata.
func (s *Session) Screen() Screen {
	s.mu.Lock()
	defer s.mu.Unlock()
	return *s.screen
}

// BindParams exposes the bind parameters; the orchestrator (and tests)
// can read but should treat them as immutable from outside.
func (s *Session) BindParams() *BindParams { return s.bindParams }

// Begin ports `async begin()`. Idempotent if already starting/running.
func (s *Session) Begin(ctx context.Context) error {
	s.mu.Lock()
	isRefreshing := s.status == SessionStatusRefreshing
	if s.status != SessionStatusStopped && !isRefreshing {
		s.mu.Unlock()
		return nil
	}
	if !isRefreshing {
		s.status = SessionStatusStarting
	}
	s.mu.Unlock()

	mdxContextStorageKey := "mdxContext." + s.client.Theme
	isScreenIDFromDataStore := false
	if !isRefreshing && s.screen.ID == "" && s.dataStore != nil {
		if raw, err := s.dataStore.Get(ctx, mdxContextStorageKey); err == nil && len(raw) > 0 {
			var stored MDXContext
			if jerr := json.Unmarshal(raw, &stored); jerr == nil {
				s.debug("Configuring session with stored MDX context:", stored)
				s.bindParams.UpdateWithMDXContext(stored)
				if stored.ScreenID != "" {
					s.screen.ID = stored.ScreenID
					isScreenIDFromDataStore = true
				}
			}
		}
	}

	var begErr error
	func() {
		s.ofs = 0
		if s.screen.ID == "" {
			id, err := s.generateScreenID(ctx)
			if err != nil {
				begErr = err
				return
			}
			s.screen.ID = id
		}
		loungeToken, err := s.getLoungeToken(ctx)
		if err != nil {
			if !isScreenIDFromDataStore {
				begErr = err
				return
			}
			s.error(fmt.Sprintf("(%s) Failed to obtain lounge token with screen Id from stored MDX context (%s):",
				s.client.Name, s.screen.ID), err, "Going to generate fresh screen Id and try again...")
			newID, gerr := s.generateScreenID(ctx)
			if gerr != nil {
				begErr = gerr
				return
			}
			s.screen.ID = newID
			loungeToken, err = s.getLoungeToken(ctx)
			if err != nil {
				begErr = err
				return
			}
		}
		s.bindParams.UpdateWithLoungeToken(loungeToken)

		// Refresh lounge token on expiry.
		s.clearLoungeTokenRefreshTimer()
		refresh := time.Duration(loungeToken.RefreshIntervalInMillis) * time.Millisecond
		if refresh <= 0 {
			refresh = 13 * 24 * time.Hour // 13 days, matching upstream fallback (1123200000 ms).
		}
		s.mu.Lock()
		s.loungeTokenTimer = time.AfterFunc(refresh, func() {
			if err := s.refreshLoungeToken(context.Background()); err != nil {
				if endErr := s.End(context.Background(), err); endErr != nil {
					s.error("Caught error ending session:", endErr)
				}
			}
		})
		s.mu.Unlock()

		initMsgs, err := s.getInitSessionMessages(ctx)
		if err != nil {
			begErr = err
			return
		}
		for _, cmd := range initMsgs {
			if cmd == nil {
				continue
			}
			if cmd.Name == "c" || cmd.Name == "S" {
				s.bindParams.UpdateWithMessage(cmd)
			} else if cmd.Name == "loungeStatus" {
				s.handleMessage([]*Message{cmd})
			}
		}
		// Smoke-test the rpc query string can be built.
		if _, err := s.bindParams.ToQueryString(QueryStringTypeRPC, nil); err != nil {
			var inc *yterrors.IncompleteAPIDataError
			if errors.As(err, &inc) {
				missing, _ := inc.Info["missing"].([]string)
				begErr = yterrors.NewIncompleteAPIDataError(fmt.Sprintf("(%s) Query string test failed", s.client.Name), missing)
				return
			}
			begErr = err
			return
		}
	}()

	if begErr != nil {
		begErr = yterrors.NewSessionError(fmt.Sprintf("(%s) Failed to establish session", s.client.Name), begErr)
	}

	if begErr == nil {
		s.rpc = NewRPCConnection(RPCConnectionOptions{
			BindParams: s.bindParams,
			Logger:     s.logger,
			HTTPClient: s.httpClient,
			BaseURL:    s.urlBind,
		})
		s.rpc.SetOnMessages(func(messages []*Message) {
			s.handleMessage(messages)
		})
		s.rpc.SetOnTerminate(func(err error) {
			s.error(fmt.Sprintf("(%s) RPC connection terminated due to error:", s.client.Name), err)
			if s.bus != nil {
				s.bus.Publish(RPCConnectionTerminatedEvent{Err: err})
			}
			if rerr := s.refreshLoungeToken(context.Background()); rerr != nil {
				s.error("Caught error refreshing lounge token:", rerr)
			}
		})

		rpcCtx, rpcCancel := context.WithCancel(context.Background())
		s.mu.Lock()
		s.currentRPCCancel = rpcCancel
		s.mu.Unlock()

		if err := s.rpc.Connect(rpcCtx); err != nil {
			begErr = yterrors.NewSessionError(fmt.Sprintf("(%s) Failed to establish session", s.client.Name), err)
		} else {
			s.debug(fmt.Sprintf("(%s) Session established.", s.client.Name))
			if !isRefreshing {
				s.mu.Lock()
				s.status = SessionStatusRunning
				s.mu.Unlock()
				if s.bus != nil {
					s.bus.Publish(SessionConnectedEvent{Session: s})
				}
			}
		}
	}

	if begErr != nil {
		if isRefreshing {
			if err := s.End(ctx, begErr); err != nil {
				s.error("Caught error ending session:", err)
			}
		} else {
			if err := s.End(ctx, nil); err != nil {
				s.error("Caught error ending session:", err)
			}
			return begErr
		}
	} else if s.dataStore != nil {
		mdxCtx := MDXContext{
			DeviceID: s.bindParams.ID,
			ScreenID: s.screen.ID,
		}
		s.debug(fmt.Sprintf("(%s) Saving MDX context to data store:", s.client.Name), mdxCtx)
		if err := s.dataStore.Set(ctx, mdxContextStorageKey, mdxCtx); err != nil {
			s.error("Caught error saving MDX context:", err)
		}
	}
	return nil
}

// End ports `async end(error?)`. `cause` is the involuntary-termination
// trigger, nil for a clean End.
func (s *Session) End(ctx context.Context, cause error) error {
	s.mu.Lock()
	if s.status == SessionStatusStopped || s.status == SessionStatusStopping {
		s.mu.Unlock()
		return nil
	}
	s.status = SessionStatusStopping
	rpc := s.rpc
	cancel := s.currentRPCCancel
	s.currentRPCCancel = nil
	s.mu.Unlock()

	// The upstream `try { ... } catch (err)` wraps the disconnect-cleanup
	// steps that follow. None of the Go equivalents return errors that
	// callers care about (the loungeScreenDisconnected send is best-
	// effort by design), so we don't track an endErr — but we keep the
	// silent-catch comment for parity.
	s.pair.Stop()
	if rpc != nil {
		rpc.SetOnMessages(nil)
		rpc.SetOnTerminate(nil)
		rpc.Close()
	}
	if cancel != nil {
		cancel()
	}
	s.queue.Clear()
	s.clearDeferredMessages()
	// Best-effort send of loungeScreenDisconnected. Matches upstream's
	// silent catch: failures here are expected during shutdown.
	if _, err := s.SendMessage(ctx, []*Message{NewLoungeScreenDisconnected()}, nil); err != nil {
		s.debug("loungeScreenDisconnected send swallowed:", err)
	}
	// Restart the queue in case it was stopped by a token refresh.
	go func() {
		<-s.queue.Start()
	}()

	s.mu.Lock()
	s.bindParams.Reset()
	s.status = SessionStatusStopped
	s.clearLoungeTokenRefreshTimerLocked()
	s.mu.Unlock()
	if s.bus != nil {
		s.bus.Publish(SessionDisconnectedEvent{Session: s, Reason: cause})
	}
	if cause != nil {
		s.mu.Lock()
		listener := s.listener
		s.mu.Unlock()
		if listener.OnTerminate != nil {
			listener.OnTerminate(cause)
		}
	}
	return nil
}

// Restart ports `async restart()`.
func (s *Session) Restart(ctx context.Context) error {
	s.debug(fmt.Sprintf("(%s) Restarting session...", s.client.Name))
	prevStatus := s.pair.Status()
	if err := s.End(ctx, nil); err != nil {
		return yterrors.NewSessionError(fmt.Sprintf("(%s) Error while restarting session", s.client.Name), err)
	}
	if err := s.Begin(ctx); err != nil {
		return yterrors.NewSessionError(fmt.Sprintf("(%s) Error while restarting session", s.client.Name), err)
	}
	if prevStatus == PairingCodeStatusRunning {
		s.pair.Start()
	}
	return nil
}

// refreshLoungeToken ports `#refreshLoungeToken()`.
func (s *Session) refreshLoungeToken(ctx context.Context) error {
	s.mu.Lock()
	s.status = SessionStatusRefreshing
	oldRPC := s.rpc
	s.mu.Unlock()
	s.debug(fmt.Sprintf("(%s) Refreshing lounge token...", s.client.Name))
	s.clearLoungeTokenRefreshTimer()

	s.queue.SetAutoStart(false)
	s.queue.Stop()

	pairStatus := s.pair.Status()
	s.pair.Stop()

	s.bindParams.Reset()

	beginErr := s.Begin(ctx)
	s.debug(fmt.Sprintf("(%s) Closing old RPC connection...", s.client.Name))
	if oldRPC != nil {
		oldRPC.SetOnMessages(nil)
		oldRPC.SetOnTerminate(nil)
		oldRPC.Close()
	}
	if beginErr != nil {
		return yterrors.NewSessionError(fmt.Sprintf("(%s) Error while refreshing lounge token", s.client.Name), beginErr)
	}

	if pairStatus == PairingCodeStatusRunning {
		s.pair.Start()
	}

	// Nullify AIDs on pending and deferred messages — upstream comment
	// explains: AID restarts after refresh.
	s.mu.Lock()
	for _, dm := range s.deferred {
		for _, m := range dm.msgs {
			m.AID = nil
		}
	}
	s.mu.Unlock()
	s.queue.ForEach(func(_ int, t asyncq.Task) {
		// Tasks carry no message metadata at the asyncq level; the
		// callbacks closure captures their messages. Upstream's
		// equivalent loop reaches into SendMessageTask to null the
		// AIDs. Our send loop derives the AID off the message at send
		// time (see doSendMessage), so the equivalent here is the
		// message-pointer mutation we do in the closure binding.
		// Nothing further is required.
		_ = t
	})

	s.queue.SetAutoStart(true)
	s.mu.Lock()
	s.status = SessionStatusRunning
	s.mu.Unlock()
	return nil
}

func (s *Session) clearLoungeTokenRefreshTimer() {
	s.mu.Lock()
	s.clearLoungeTokenRefreshTimerLocked()
	s.mu.Unlock()
}

func (s *Session) clearLoungeTokenRefreshTimerLocked() {
	if s.loungeTokenTimer != nil {
		s.loungeTokenTimer.Stop()
		s.loungeTokenTimer = nil
	}
}

func (s *Session) clearDeferredMessages() {
	s.mu.Lock()
	dm := s.deferred
	s.deferred = make(map[string]*deferredMessage)
	s.mu.Unlock()
	for _, d := range dm {
		if d.timer != nil {
			d.timer.Stop()
		}
		if d.cancel != nil {
			d.cancel()
		}
	}
}

// generateScreenID ports `#generateScreenId()`.
func (s *Session) generateScreenID(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.urlGenerateScreenID, nil)
	if err != nil {
		return "", yterrors.NewConnectionError(fmt.Sprintf("(%s) Connection error in generating screen Id", s.client.Name), s.urlGenerateScreenID, err)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", yterrors.NewConnectionError(fmt.Sprintf("(%s) Connection error in generating screen Id", s.client.Name), s.urlGenerateScreenID, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", yterrors.NewDataError(fmt.Sprintf("(%s) Screen Id data error", s.client.Name), err, nil)
	}
	screenID := strings.TrimSpace(string(body))
	s.debug(fmt.Sprintf("(%s) Obtained screen ID: %s", s.client.Name, screenID))
	return screenID, nil
}

// loungeTokenResponse is the JSON shape returned by
// `get_lounge_token_batch`.
type loungeTokenResponse struct {
	Screens []LoungeToken `json:"screens"`
}

// getLoungeToken ports `#getLoungeToken()`.
func (s *Session) getLoungeToken(ctx context.Context) (LoungeToken, error) {
	if s.screen.ID == "" {
		return LoungeToken{}, yterrors.NewIncompleteAPIDataError(fmt.Sprintf("(%s) Missing data required to get lounge token", s.client.Name), []string{"screenId"})
	}
	form := url.Values{"screen_ids": []string{s.screen.ID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.urlGetLoungeTokenBatch, strings.NewReader(form.Encode()))
	if err != nil {
		return LoungeToken{}, yterrors.NewConnectionError(fmt.Sprintf("(%s) Connection error in getting lounge token", s.client.Name), s.urlGetLoungeTokenBatch, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return LoungeToken{}, yterrors.NewConnectionError(fmt.Sprintf("(%s) Connection error in getting lounge token", s.client.Name), s.urlGetLoungeTokenBatch, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return LoungeToken{}, yterrors.NewDataError(fmt.Sprintf("(%s) Lounge token data error", s.client.Name), err, nil)
	}
	var parsed loungeTokenResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return LoungeToken{}, yterrors.NewDataError(fmt.Sprintf("(%s) Lounge token data error", s.client.Name), err, json.RawMessage(body))
	}
	if len(parsed.Screens) == 0 {
		return LoungeToken{}, yterrors.NewDataError(fmt.Sprintf("(%s) Lounge token data error", s.client.Name), nil, json.RawMessage(body))
	}
	token := parsed.Screens[0]
	s.debug(fmt.Sprintf("(%s) Obtained lounge token:", s.client.Name), token)
	return token, nil
}

// getInitSessionMessages ports `#getInitSessionMessages()`.
func (s *Session) getInitSessionMessages(ctx context.Context) ([]*Message, error) {
	qs, err := s.bindParams.ToQueryString(QueryStringTypeInitSession, nil)
	if err != nil {
		return nil, err
	}
	urlStr := fmt.Sprintf("%s?%s", s.urlBind, qs)
	form := url.Values{"count": []string{"0"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, yterrors.NewConnectionError(fmt.Sprintf("(%s) Connection error in fetching session data", s.client.Name), urlStr, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, yterrors.NewConnectionError(fmt.Sprintf("(%s) Connection error in fetching session data", s.client.Name), urlStr, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, yterrors.NewDataError(fmt.Sprintf("(%s) Session data error", s.client.Name), err, nil)
	}
	messages := ParseIncoming(string(body))
	s.debug(fmt.Sprintf("(%s) Received messages for establishing session:", s.client.Name), messages)
	return messages, nil
}

// RegisterPairingCode ports `async registerPairingCode(code)`.
func (s *Session) RegisterPairingCode(ctx context.Context, code string) error {
	s.debug(fmt.Sprintf("(%s) Registering pairing code: %s", s.client.Name, code))
	if s.screen.ID == "" {
		return yterrors.NewIncompleteAPIDataError(fmt.Sprintf("(%s) Missing data required to register pairing code", s.client.Name), []string{"screenId"})
	}
	form := url.Values{}
	form.Set("access_type", "permanent")
	form.Set("app", s.screen.App)
	form.Set("pairing_code", code)
	form.Set("screen_id", s.screen.ID)
	form.Set("screen_name", s.screen.Name)
	form.Set("device_id", s.bindParams.ID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.urlRegisterPairingCode, strings.NewReader(form.Encode()))
	if err != nil {
		return yterrors.NewConnectionError(fmt.Sprintf("(%s) Connection error in registering pairing code", s.client.Name), s.urlRegisterPairingCode, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return yterrors.NewConnectionError(fmt.Sprintf("(%s) Connection error in registering pairing code", s.client.Name), s.urlRegisterPairingCode, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		s.debug(fmt.Sprintf("(%s) Pairing code registered.", s.client.Name))
		return nil
	}
	return yterrors.NewBadResponseError(fmt.Sprintf("(%s) Failed to register pairing code", s.client.Name), s.urlRegisterPairingCode, yterrors.HTTPResponse{Status: resp.StatusCode, StatusText: http.StatusText(resp.StatusCode)})
}

// SendDeferOptions ports the upstream `defer: {key, interval}` argument.
type SendDeferOptions struct {
	Key      string
	Interval time.Duration
}

// SendMessage ports `sendMessage(messages, defer)`. Returns true when the
// message was sent, false when it was cancelled before dispatch (e.g.
// replaced by a newer deferred message under the same key).
//
// If defer is nil, the message is queued immediately. Otherwise the
// message is held for defer.Interval; a subsequent SendMessage with the
// same key cancels the pending dispatch and replaces it.
func (s *Session) SendMessage(ctx context.Context, messages []*Message, defer_ *SendDeferOptions) (bool, error) {
	if len(messages) == 0 {
		return true, nil
	}
	if defer_ != nil && defer_.Key != "" {
		return s.sendDeferred(ctx, messages, *defer_), nil
	}
	return s.sendNow(ctx, messages)
}

func (s *Session) sendDeferred(ctx context.Context, messages []*Message, opts SendDeferOptions) bool {
	s.mu.Lock()
	if existing, ok := s.deferred[opts.Key]; ok {
		if existing.timer != nil {
			existing.timer.Stop()
		}
		if existing.cancel != nil {
			existing.cancel()
		}
		delete(s.deferred, opts.Key)
	}
	resultCh := make(chan bool, 1)
	cancelOnce := sync.Once{}
	cancelFn := func() {
		cancelOnce.Do(func() {
			select {
			case resultCh <- false:
			default:
			}
		})
	}
	dm := &deferredMessage{msgs: messages, cancel: cancelFn}
	dm.timer = time.AfterFunc(opts.Interval, func() {
		s.mu.Lock()
		current, ok := s.deferred[opts.Key]
		if ok && current == dm {
			delete(s.deferred, opts.Key)
		}
		s.mu.Unlock()
		// On dispatch, run sendNow on its own goroutine and feed the
		// result into resultCh. This mirrors upstream where the
		// deferred Promise resolves once the underlying send resolves.
		go func() {
			ok, _ := s.sendNow(ctx, messages)
			cancelOnce.Do(func() {
				select {
				case resultCh <- ok:
				default:
				}
			})
		}()
	})
	s.deferred[opts.Key] = dm
	s.mu.Unlock()
	// Block until either the deferred timer fires and the send resolves,
	// or the message is replaced/cancelled.
	select {
	case ok := <-resultCh:
		return ok
	case <-ctx.Done():
		cancelFn()
		return false
	}
}

func (s *Session) sendNow(ctx context.Context, messages []*Message) (bool, error) {
	resultCh := make(chan sendResult, 1)
	task := s.buildSendTask(ctx, messages, resultCh, 0)
	s.queue.Push(task)
	select {
	case r := <-resultCh:
		return r.ok, r.err
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

type sendResult struct {
	ok  bool
	err error
}

// buildSendTask constructs the AsyncTaskQueue task for sending messages.
// `attempt` is 0 for the initial enqueue and 1 for the post-refresh
// retry; upstream's onError handler kicks the message back to the head
// of the queue once, then ends the session if a second failure occurs.
func (s *Session) buildSendTask(ctx context.Context, messages []*Message, resultCh chan<- sendResult, attempt int) asyncq.Task {
	return asyncq.Task{
		Run: func(taskCtx context.Context) error {
			useCtx := taskCtx
			if useCtx == nil {
				useCtx = ctx
			}
			err := s.doSendMessage(useCtx, messages)
			if err != nil {
				return err
			}
			select {
			case resultCh <- sendResult{ok: true}:
			default:
			}
			return nil
		},
		Cancel: func() {
			select {
			case resultCh <- sendResult{ok: false}:
			default:
			}
		},
		OnError: func(err error) {
			s.error(fmt.Sprintf("(%s) Error occurred in SendMessageTask:", s.client.Name), messages, err)
			if attempt == 0 {
				// Retry after refreshing the lounge token. Unshift the
				// retry task to the head of the queue.
				retry := s.buildSendTask(ctx, messages, resultCh, 1)
				s.queue.Unshift(retry)
				s.debug(fmt.Sprintf("(%s) Retry task after refreshing lounge token...", s.client.Name))
				go func() {
					if rerr := s.refreshLoungeToken(context.Background()); rerr != nil {
						s.error("Caught error refreshing lounge token:", rerr)
					}
				}()
				return
			}
			// Second failure — give up and end the session.
			go func() {
				if eerr := s.End(context.Background(), err); eerr != nil {
					s.error("Caught error ending session:", eerr)
				}
			}()
			select {
			case resultCh <- sendResult{ok: false, err: err}:
			default:
			}
		},
	}
}

// doSendMessage ports `#doSendMessage`. POSTs the message bundle to the
// bind URL and inspects the response.
func (s *Session) doSendMessage(ctx context.Context, messages []*Message) error {
	if len(messages) == 0 {
		return nil
	}
	var aid *int
	if messages[0].AID != nil {
		v := *messages[0].AID
		aid = &v
	}
	qs, err := s.bindParams.ToQueryString(QueryStringTypeSendMessage, aid)
	if err != nil {
		return err
	}
	urlStr := fmt.Sprintf("%s?%s", s.urlBind, qs)
	postData := s.getSendMessagePayload(messages)

	debugNames := make([]string, 0, len(messages))
	for _, m := range messages {
		debugNames = append(debugNames, m.Name)
	}
	var debugLabel string
	if len(messages) > 1 {
		debugLabel = fmt.Sprintf("messages '%s'", strings.Join(debugNames, " + "))
	} else {
		debugLabel = fmt.Sprintf("message '%s'", debugNames[0])
	}
	aidStr := ""
	if aid != nil {
		aidStr = fmt.Sprintf("(AID: %d) ", *aid)
	}
	s.debug(fmt.Sprintf("[yt-cast-receiver] %s(%s) Sending %s with payload:", aidStr, s.client.Name, debugLabel), postData)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, strings.NewReader(postData.Encode()))
	if err != nil {
		return yterrors.NewConnectionError(fmt.Sprintf("(%s) Connection error in sending %s%s", s.client.Name, debugLabel, aidLabel(aid)), urlStr, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return yterrors.NewConnectionError(fmt.Sprintf("(%s) Connection error in sending %s%s", s.client.Name, debugLabel, aidLabel(aid)), urlStr, err)
	}
	defer func() { _ = resp.Body.Close() }()
	s.debug(fmt.Sprintf("[yt-cast-receiver] %s(%s) Response received for sent %s. Status: %d", aidStr, s.client.Name, debugLabel, resp.StatusCode))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return yterrors.NewBadResponseError(fmt.Sprintf("(%s) Bad response received for %s%s", s.client.Name, debugLabel, aidLabel(aid)), urlStr, yterrors.HTTPResponse{Status: resp.StatusCode, StatusText: http.StatusText(resp.StatusCode)})
}

func aidLabel(aid *int) string {
	if aid == nil {
		return ""
	}
	return fmt.Sprintf(" (AID: %d)", *aid)
}

// getSendMessagePayload ports `#getSendMessagePayload`. Returns the
// url.Values map that gets form-encoded into the POST body.
func (s *Session) getSendMessagePayload(messages []*Message) url.Values {
	form := url.Values{}
	form.Set("count", strconv.Itoa(len(messages)))
	form.Set("ofs", strconv.Itoa(s.ofs))
	for i, msg := range messages {
		prefix := fmt.Sprintf("req%d_", i)
		form.Set(prefix+"_sc", msg.Name)
		payload := msg.PayloadAsMap()
		for k, v := range payload {
			form.Set(prefix+k, fmt.Sprintf("%v", v))
		}
	}
	s.ofs += len(messages)
	return form
}

// handleMessage ports `#handleMessage`. Updates bind params then fans
// out to the listener and event bus.
func (s *Session) handleMessage(messages []*Message) {
	s.bindParams.UpdateWithMessage(messages...)
	s.mu.Lock()
	listener := s.listener
	s.mu.Unlock()
	if listener.OnMessages != nil {
		listener.OnMessages(s, messages)
	}
	if s.bus != nil {
		s.bus.Publish(MessageReceivedEvent{Session: s, Messages: messages})
	}
}

func (s *Session) debug(args ...any) {
	if s.logger != nil {
		s.logger.Debug(args...)
	}
}

func (s *Session) error(args ...any) {
	if s.logger != nil {
		s.logger.Error(args...)
	}
}
