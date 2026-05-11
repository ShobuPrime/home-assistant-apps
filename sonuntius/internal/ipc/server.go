// Package ipc implements the JSON-line Unix-domain-socket broker that
// connects ma-bridge to the Cast/DIAL receivers.
package ipc

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/shobuprime/sonuntius/internal/events"
)

// DefaultSocketPath is the UDS path used unless overridden by the
// SONUNTIUS_IPC_SOCK environment variable.
const DefaultSocketPath = "/run/sonuntius/events.sock"

// SocketPath returns the resolved UDS path (env override or default).
func SocketPath() string {
	if v := os.Getenv("SONUNTIUS_IPC_SOCK"); v != "" {
		return v
	}
	return DefaultSocketPath
}

// Handler receives events read from connected clients.
type Handler func(context.Context, events.Event)

// Server is the broker. One instance per process — ma-bridge owns it.
type Server struct {
	Path    string
	Logger  *slog.Logger
	Handler Handler

	mu       sync.Mutex
	clients  map[*net.UnixConn]*bufio.Writer
	listener *net.UnixListener
}

// NewServer constructs a Server bound to path. If path is empty,
// SocketPath() is used.
func NewServer(path string, logger *slog.Logger) *Server {
	if path == "" {
		path = SocketPath()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		Path:    path,
		Logger:  logger,
		clients: make(map[*net.UnixConn]*bufio.Writer),
	}
}

// Start binds the listener and spawns the accept loop. ctx cancellation
// stops the loop and closes all client connections.
func (s *Server) Start(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	// Remove any stale socket from a previous run.
	if err := os.Remove(s.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	addr := &net.UnixAddr{Name: s.Path, Net: "unix"}
	l, err := net.ListenUnix("unix", addr)
	if err != nil {
		return err
	}
	if err := os.Chmod(s.Path, 0o660); err != nil {
		l.Close()
		return err
	}
	s.listener = l
	s.Logger.Info("ipc: listening", "path", s.Path)

	go s.acceptLoop(ctx)
	go func() {
		<-ctx.Done()
		s.shutdown()
	}()
	return nil
}

func (s *Server) acceptLoop(ctx context.Context) {
	for {
		conn, err := s.listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if errors.Is(err, net.ErrClosed) {
				return
			}
			s.Logger.Warn("ipc: accept error", "err", err)
			continue
		}
		go s.handleClient(ctx, conn)
	}
}

func (s *Server) handleClient(ctx context.Context, conn *net.UnixConn) {
	w := bufio.NewWriter(conn)
	s.mu.Lock()
	s.clients[conn] = w
	s.mu.Unlock()
	s.Logger.Info("ipc: client connected")

	defer func() {
		s.mu.Lock()
		delete(s.clients, conn)
		s.mu.Unlock()
		conn.Close()
		s.Logger.Info("ipc: client disconnected")
	}()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := scanner.Bytes()
		ev, err := events.Unmarshal(line)
		if err != nil {
			s.Logger.Warn("ipc: dropping malformed event", "err", err)
			continue
		}
		if s.Handler != nil {
			s.Handler(ctx, ev)
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, net.ErrClosed) {
		s.Logger.Warn("ipc: client read error", "err", err)
	}
}

// Broadcast sends ev to every connected client. Slow / broken clients
// are dropped so a single misbehaving consumer cannot wedge the server.
func (s *Server) Broadcast(ev events.Event) {
	payload, err := events.Marshal(ev)
	if err != nil {
		s.Logger.Warn("ipc: broadcast marshal failed", "err", err)
		return
	}
	payload = append(payload, '\n')

	s.mu.Lock()
	dead := make([]*net.UnixConn, 0)
	for conn, w := range s.clients {
		if _, err := w.Write(payload); err != nil {
			dead = append(dead, conn)
			continue
		}
		if err := w.Flush(); err != nil {
			dead = append(dead, conn)
		}
	}
	for _, conn := range dead {
		delete(s.clients, conn)
		conn.Close()
	}
	s.mu.Unlock()
}

func (s *Server) shutdown() {
	if s.listener != nil {
		s.listener.Close()
	}
	s.mu.Lock()
	for conn := range s.clients {
		conn.Close()
	}
	s.clients = make(map[*net.UnixConn]*bufio.Writer)
	s.mu.Unlock()
	if err := os.Remove(s.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.Logger.Warn("ipc: socket cleanup failed", "err", err)
	}
}
