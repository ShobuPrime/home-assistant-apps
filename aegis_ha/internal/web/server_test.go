package web

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"

	"github.com/shobuprime/aegis_ha/internal/alarm"
	"github.com/shobuprime/aegis_ha/internal/store"
)

func newTestServer(t *testing.T) (*Server, *alarm.Engine) {
	t.Helper()
	st, err := store.Open(t.TempDir(), store.Policy{PINMin: 4, PINMax: 8, LockoutThreshold: 5, LockoutDuration: time.Minute})
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if _, err := st.AddUser(store.User{Name: "Anthony", HAUserID: "u-1", Role: "admin"}, "1234"); err != nil {
		t.Fatalf("add user: %v", err)
	}
	eng := alarm.New(alarm.Config{ExitDelay: 0, ArmModes: []string{"away", "home"}}, nil)
	go eng.Run(t.Context())
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := New(log, ":0", Options{
		Engine: eng, Store: st, ArmModes: []string{"away", "home"},
		DisarmRequiresCode: true, EnableUI: true, Version: "test",
	})
	return s, eng
}

func doForm(s *Server, method, path, userID string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if userID != "" {
		req.Header.Set("X-Remote-User-Id", userID)
	}
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec
}

func TestHealthIsOpen(t *testing.T) {
	s, _ := newTestServer(t)
	rec := doForm(s, "GET", "/health", "", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"ok"`) {
		t.Fatalf("health: code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestIndexRequiresIngressIdentity(t *testing.T) {
	s, _ := newTestServer(t)
	if rec := doForm(s, "GET", "/", "", nil); rec.Code != 403 {
		t.Fatalf("no identity should be 403, got %d", rec.Code)
	}
	rec := doForm(s, "GET", "/", "u-1", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "AegisHA") {
		t.Fatalf("with identity: code=%d", rec.Code)
	}
}

func TestArmViaKeypad(t *testing.T) {
	s, eng := newTestServer(t)
	rec := doForm(s, "POST", "/arm", "u-1", url.Values{"code": {"1234"}, "mode": {"away"}})
	if rec.Code != 200 {
		t.Fatalf("arm: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := eng.Current().State; got != alarm.StateArmedAway {
		t.Fatalf("engine state = %s, want armed_away", got)
	}
}

func TestWrongPinDenied(t *testing.T) {
	s, eng := newTestServer(t)
	rec := doForm(s, "POST", "/disarm", "u-1", url.Values{"code": {"0000"}})
	if !strings.Contains(rec.Body.String(), "Denied") {
		t.Fatalf("wrong pin should be denied, body=%s", rec.Body.String())
	}
	// Arm first, then a wrong-pin disarm must NOT disarm.
	eng.Arm("away", alarm.Actor{Name: "x"}, false)
	doForm(s, "POST", "/disarm", "u-1", url.Values{"code": {"0000"}})
	if eng.Current().State != alarm.StateArmedAway {
		t.Fatal("wrong pin disarmed the system")
	}
}

func TestAdminGating(t *testing.T) {
	s, _ := newTestServer(t)
	// u-2 has no user record → not admin.
	if rec := doForm(s, "GET", "/admin", "u-2", nil); rec.Code != 403 {
		t.Fatalf("non-admin should be 403, got %d", rec.Code)
	}
	if rec := doForm(s, "GET", "/admin", "u-1", nil); rec.Code != 200 {
		t.Fatalf("admin should be 200, got %d", rec.Code)
	}
}

func TestWebSocketLiveState(t *testing.T) {
	s, eng := newTestServer(t)
	go s.runHub(t.Context())
	srv := httptest.NewServer(s.mux)
	defer srv.Close()

	cfg, err := websocket.NewConfig("ws://"+strings.TrimPrefix(srv.URL, "http://")+"/ws", "http://localhost")
	if err != nil {
		t.Fatalf("ws config: %v", err)
	}
	cfg.Header.Set("X-Remote-User-Id", "u-1")
	ws, err := websocket.DialConfig(cfg)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer ws.Close()

	read := func() string {
		_ = ws.SetReadDeadline(time.Now().Add(2 * time.Second))
		var m string
		if err := websocket.Message.Receive(ws, &m); err != nil {
			t.Fatalf("ws receive: %v", err)
		}
		return m
	}

	if got := read(); !strings.Contains(got, "aegis_ha-state") || !strings.Contains(got, "disarmed") {
		t.Fatalf("initial frame missing disarmed state: %s", got)
	}

	eng.Arm("away", alarm.Actor{Name: "x"}, false)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if strings.Contains(read(), "armed_away") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("did not observe armed_away over the websocket")
		}
	}
}

func TestAddUserViaAdmin(t *testing.T) {
	s, _ := newTestServer(t)
	rec := doForm(s, "POST", "/admin/users", "u-1", url.Values{
		"name": {"Guest"}, "pin": {"5678"}, "role": {"guest"},
	})
	if rec.Code != 303 {
		t.Fatalf("add user should redirect, got %d", rec.Code)
	}
	if s.opts.Store.Count() != 2 {
		t.Fatalf("want 2 users, got %d", s.opts.Store.Count())
	}
}
