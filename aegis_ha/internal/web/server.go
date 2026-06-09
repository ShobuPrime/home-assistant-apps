// Package web serves AegisHA's HTTP surface on the ingress port (8099):
// the loopback-reachable health endpoints used by the Docker HEALTHCHECK
// and the CI smoke-test, and the ingress keypad + admin UI.
//
// The interactive UI trusts the Supervisor-injected identity headers
// (X-Remote-User-Id is non-spoofable behind ingress) to bind each PIN to
// the logged-in Home Assistant user; requests without that identity are
// refused. Health endpoints stay unauthenticated and loopback-reachable.
// Links are rewritten with X-Ingress-Path so the UI works under the
// tokenized ingress URL. Live state is pushed over Server-Sent Events
// (pure net/http + http.Flusher — no WebSocket dependency).
package web

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"html"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/websocket"

	"github.com/shobuprime/aegis_ha/internal/alarm"
	"github.com/shobuprime/aegis_ha/internal/store"
)

//go:embed templates/*.html
var tmplFS embed.FS

// Options configures the server.
type Options struct {
	Engine              *alarm.Engine
	Store               *store.Store
	ArmModes            []string
	RequireCodeToArm    bool
	RequireCodeToDisarm bool
	EnableUI            bool
	Version             string
}

// Server owns the HTTP listener and the SSE fan-out hub.
type Server struct {
	log  *slog.Logger
	addr string
	opts Options
	tmpl *template.Template
	mux  *http.ServeMux

	hubMu   sync.Mutex
	clients map[chan alarm.Snapshot]struct{}
}

// New constructs a Server bound to addr (e.g. ":8099").
func New(log *slog.Logger, addr string, opts Options) *Server {
	s := &Server{
		log:     log,
		addr:    addr,
		opts:    opts,
		mux:     http.NewServeMux(),
		clients: map[chan alarm.Snapshot]struct{}{},
	}
	s.tmpl = template.Must(template.ParseFS(tmplFS, "templates/*.html"))
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /api/health", s.handleHealth)

	if s.opts.EnableUI && s.opts.Engine != nil && s.opts.Store != nil {
		s.mux.HandleFunc("GET /{$}", s.ui(s.handleIndex))
		s.mux.HandleFunc("GET /ws", s.ui(s.handleWS))
		s.mux.HandleFunc("POST /arm", s.ui(s.handleArm))
		s.mux.HandleFunc("POST /disarm", s.ui(s.handleDisarm))
		s.mux.HandleFunc("POST /trigger", s.ui(s.handleTrigger))
	} else {
		s.mux.HandleFunc("GET /{$}", s.handleRoot)
	}
}

// Run starts the SSE hub and the HTTP server, blocking until ctx is
// cancelled, then shutting down gracefully.
func (s *Server) Run(ctx context.Context) error {
	if s.opts.EnableUI && s.opts.Engine != nil {
		go s.runHub(ctx)
	}
	srv := &http.Server{Addr: s.addr, Handler: s.mux, ReadHeaderTimeout: 10 * time.Second}
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("web: listening", "addr", s.addr, "ui", s.opts.EnableUI)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}

// --- identity / middleware ---

type identity struct {
	ID, Name, Display string
}

func (s *Server) identify(r *http.Request) identity {
	id := r.Header.Get("X-Remote-User-Id")
	name := r.Header.Get("X-Remote-User-Name")
	disp := r.Header.Get("X-Remote-User-Display-Name")
	if disp == "" {
		disp = name
	}
	if disp == "" {
		disp = "user"
	}
	return identity{ID: id, Name: name, Display: disp}
}

// ui gates a handler on a present ingress identity.
func (s *Server) ui(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.identify(r).ID == "" {
			http.Error(w, "Open AegisHA from the Home Assistant sidebar (ingress).", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func ingressBase(r *http.Request) string {
	return strings.TrimRight(r.Header.Get("X-Ingress-Path"), "/")
}

// --- handlers ---

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok","app":"aegis_ha"}`))
}

func (s *Server) handleRoot(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("AegisHA is running. Web UI is disabled.\n"))
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	id := s.identify(r)
	s.render(w, "index", map[string]any{
		"Base":     ingressBase(r),
		"User":     id,
		"ArmModes": s.opts.ArmModes,
		"Snapshot": s.opts.Engine.Current(),
		"Digits":   []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"},
	})
}

// handleWS upgrades the connection and streams live alarm-state fragments
// to the htmx WebSocket extension. golang.org/x/net/websocket is the Go
// team's package (there is no stdlib WebSocket), so this is the most
// native-Go choice; it is the project's single non-stdlib dependency.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	websocket.Handler(s.serveWS).ServeHTTP(w, r)
}

func (s *Server) serveWS(ws *websocket.Conn) {
	defer ws.Close()

	ch := make(chan alarm.Snapshot, 8)
	s.hubMu.Lock()
	s.clients[ch] = struct{}{}
	s.hubMu.Unlock()
	defer func() {
		s.hubMu.Lock()
		delete(s.clients, ch)
		s.hubMu.Unlock()
	}()

	// Reader goroutine: htmx never sends us anything, but reading lets us
	// detect the client disconnecting (Receive returns an error).
	done := make(chan struct{})
	go func() {
		defer close(done)
		var discard string
		for {
			if err := websocket.Message.Receive(ws, &discard); err != nil {
				return
			}
		}
	}()

	if s.writeWS(ws, s.opts.Engine.Current()) != nil {
		return
	}
	for {
		select {
		case <-done:
			return
		case snap := <-ch:
			if s.writeWS(ws, snap) != nil {
				return
			}
		}
	}
}

func (s *Server) writeWS(ws *websocket.Conn, snap alarm.Snapshot) error {
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, "state", snap); err != nil {
		return err
	}
	return websocket.Message.Send(ws, buf.String())
}

func (s *Server) runHub(ctx context.Context) {
	ch := s.opts.Engine.Subscribe()
	for {
		select {
		case <-ctx.Done():
			return
		case snap := <-ch:
			s.hubMu.Lock()
			for c := range s.clients {
				select {
				case c <- snap:
				default:
				}
			}
			s.hubMu.Unlock()
		}
	}
}

func (s *Server) handleArm(w http.ResponseWriter, r *http.Request)     { s.action(w, r, "arm") }
func (s *Server) handleDisarm(w http.ResponseWriter, r *http.Request)  { s.action(w, r, "disarm") }
func (s *Server) handleTrigger(w http.ResponseWriter, r *http.Request) { s.action(w, r, "trigger") }

func (s *Server) action(w http.ResponseWriter, r *http.Request, action string) {
	id := s.identify(r)
	_ = r.ParseForm()
	pin := r.FormValue("code")
	mode := r.FormValue("mode")

	codeRequired := false
	switch action {
	case "arm":
		codeRequired = s.opts.RequireCodeToArm
	case "disarm":
		codeRequired = s.opts.RequireCodeToDisarm
	}
	dec := s.opts.Store.AuthorizeUser(pin, store.Perm{Action: action, Mode: mode, CodeRequired: codeRequired}, time.Now())

	// The actor is always the authenticated Home Assistant user (the
	// non-spoofable ingress identity); the shared code is only an extra gate,
	// never the identity.
	actor := alarm.Actor{Name: id.Display, UserID: id.ID}

	var msg string
	switch {
	case dec.Duress:
		s.opts.Engine.Disarm(actor)
		s.log.Warn("aegis_ha: DURESS code used on ingress keypad", "user", actor.Name)
		msg = "Disarmed."
	case !dec.Allowed:
		msg = "Denied: " + dec.Reason
	default:
		switch action {
		case "arm":
			if res := s.opts.Engine.Arm(mode, actor, false); !res.Accepted {
				msg = "Cannot arm: " + res.Reason
			} else {
				msg = "Arming " + mode + "…"
			}
		case "disarm":
			s.opts.Engine.Disarm(actor)
			msg = "Disarmed."
		case "trigger":
			s.opts.Engine.Trigger(true, actor)
			msg = "Panic triggered."
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprint(w, html.EscapeString(msg))
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := s.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		s.log.Error("web: template render failed", "name", name, "err", err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}
