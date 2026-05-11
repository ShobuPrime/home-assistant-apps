package health

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func pickPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pickPort: %v", err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("pickPort close: %v", err)
	}
	return addr
}

func startServer(t *testing.T, s *Server) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

func get(t *testing.T, url string) (*http.Response, []byte) {
	t.Helper()
	var lastErr error
	for range 10 {
		resp, err := http.Get(url)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return resp, body
		}
		lastErr = err
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("GET %s: %v", url, lastErr)
	return nil, nil
}

func TestSnapshot_SortedByName(t *testing.T) {
	t.Parallel()
	s := NewServer("127.0.0.1:0", slog.New(slog.NewTextHandler(os.Stderr, nil)))
	s.Set("zeta", true, "")
	s.Set("alpha", true, "")
	s.Set("mu", false, "broken")

	snap := s.Snapshot()
	want := []string{"alpha", "mu", "zeta"}
	if len(snap) != len(want) {
		t.Fatalf("snap len = %d, want %d", len(snap), len(want))
	}
	for i, st := range snap {
		if st.Name != want[i] {
			t.Errorf("snap[%d] = %q, want %q", i, st.Name, want[i])
		}
	}
}

func TestHealthEndpoint_ReportsOk(t *testing.T) {
	t.Parallel()
	addr := pickPort(t)
	s := NewServer(addr, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	startServer(t, s)
	s.Set("ipc", true, "listening on /run/sonuntius/events.sock")
	s.Set("dispatcher", true, "ma_player_id=media_player.sendspin")

	resp, body := get(t, "http://"+addr+"/health")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got healthResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if got.Status != "ok" {
		t.Errorf("status = %q, want %q", got.Status, "ok")
	}
	if len(got.Components) != 2 {
		t.Errorf("components = %d, want 2", len(got.Components))
	}
	if got.UptimeSecs < 0 {
		t.Errorf("uptime = %f, want >= 0", got.UptimeSecs)
	}
}

func TestHealthEndpoint_ReportsDegraded(t *testing.T) {
	t.Parallel()
	addr := pickPort(t)
	s := NewServer(addr, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	startServer(t, s)
	s.Set("ipc", true, "ok")
	s.Set("dispatcher", false, "ma_player_id unset")

	_, body := get(t, "http://"+addr+"/health")
	if !strings.Contains(string(body), `"status": "degraded"`) {
		t.Errorf("expected degraded status in body, got: %s", body)
	}
}

func TestRootHandler(t *testing.T) {
	t.Parallel()
	addr := pickPort(t)
	s := NewServer(addr, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	startServer(t, s)

	resp, body := get(t, "http://"+addr+"/")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(string(body), "sonuntius health endpoint") {
		t.Errorf("expected banner in body, got: %s", body)
	}
}
