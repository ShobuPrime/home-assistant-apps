// Package health is the addon-level health endpoint hosted by ma-bridge
// on 127.0.0.1:8099 per plan §6 Phase 6.
//
// It is a single, plain-text-and-JSON HTTP server that aggregates the
// status of every component in the addon and exposes it at /health.
// HA's addon watchdog can be pointed at this endpoint to monitor the
// addon as a whole. Individual receivers (yt-cast, cast-receiver) are
// already supervised by S6; the health endpoint summarizes their
// liveness from the perspective of the bridge.
//
// Components register themselves at startup and update their status as
// state changes. The server is intentionally lock-free for readers: a
// read takes a copy of the snapshot under the write mutex.
package health

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"
)

// DefaultAddr is the loopback address the health endpoint listens on
// per plan §6 Phase 6.
const DefaultAddr = "127.0.0.1:8099"

// Status is the per-component snapshot returned by /health.
type Status struct {
	Name      string    `json:"name"`
	Healthy   bool      `json:"healthy"`
	Detail    string    `json:"detail,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Server is the HTTP health endpoint.
type Server struct {
	Addr   string
	Logger *slog.Logger

	mu         sync.RWMutex
	started    time.Time
	components map[string]Status
	httpSrv    *http.Server
}

// NewServer builds a Server. addr is the listen address; pass "" for
// the default (127.0.0.1:8099).
func NewServer(addr string, logger *slog.Logger) *Server {
	if addr == "" {
		addr = DefaultAddr
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		Addr:       addr,
		Logger:     logger,
		components: make(map[string]Status),
	}
}

// Set records the latest status for a component. Components are created
// implicitly on first call.
func (s *Server) Set(name string, healthy bool, detail string) {
	s.mu.Lock()
	s.components[name] = Status{
		Name:      name,
		Healthy:   healthy,
		Detail:    detail,
		UpdatedAt: time.Now().UTC(),
	}
	s.mu.Unlock()
}

// Snapshot returns the current component statuses sorted by name.
func (s *Server) Snapshot() []Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Status, 0, len(s.components))
	for _, st := range s.components {
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Start binds the listener and starts the HTTP server in a goroutine.
// ctx cancellation triggers a graceful shutdown.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/", s.handleRoot)

	listener, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("health: listen %s: %w", s.Addr, err)
	}
	s.httpSrv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	s.started = time.Now().UTC()
	s.Logger.Info("health: listening", "addr", s.Addr)

	go func() {
		if err := s.httpSrv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.Logger.Warn("health: server exited", "err", err)
		}
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.httpSrv.Shutdown(shutdownCtx)
	}()
	return nil
}

// healthResponse is the wire shape /health returns.
type healthResponse struct {
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"started_at"`
	UptimeSecs float64   `json:"uptime_seconds"`
	Components []Status  `json:"components"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	snap := s.Snapshot()
	overall := "ok"
	for _, c := range snap {
		if !c.Healthy {
			overall = "degraded"
			break
		}
	}
	resp := healthResponse{
		Status:     overall,
		StartedAt:  s.started,
		UptimeSecs: time.Since(s.started).Seconds(),
		Components: snap,
	}
	w.Header().Set("Content-Type", "application/json")
	if overall == "degraded" {
		// Returning 200 keeps the HA watchdog happy when individual
		// components are misconfigured (e.g. ma_player_id unset). The
		// JSON body still conveys the degraded state for tools that
		// inspect the payload.
		w.WriteHeader(http.StatusOK)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(resp)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(w, "sonuntius health endpoint\n\nGET /health for JSON status\n")
}
