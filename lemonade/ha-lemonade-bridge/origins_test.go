package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOriginGateExactMatching(t *testing.T) {
	g := newOriginGate([]string{
		"http://192.0.2.10:13305",
		"https://home-assistant.example.com",
	})

	tests := []struct {
		name, origin string
		want         bool
	}{
		{"listed origin", "http://192.0.2.10:13305", true},
		{"listed https origin", "https://home-assistant.example.com", true},

		// Everything below differs from a listed entry in exactly one way.
		// An origin is scheme+host+port and all three are load-bearing:
		// treating any of them loosely would reopen the DNS-rebinding hole
		// this check exists to close.
		{"wrong port", "http://192.0.2.10:8123", false},
		{"no port where one was listed", "http://192.0.2.10", false},
		{"wrong scheme", "https://192.0.2.10:13305", false},
		{"wrong host", "http://192.0.2.11:13305", false},
		{"suffix of a listed host", "https://evil-home-assistant.example.com", false},
		{"listed host as a prefix", "https://home-assistant.example.com.evil.test", false},
		{"unrelated", "https://evil.example", false},

		// Sandboxed iframes and some cross-origin redirects send this
		// literally. It must not be mistaken for "no origin".
		{"literal null", "null", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := g.allowed(tc.origin); got != tc.want {
				t.Errorf("allowed(%q) = %v, want %v", tc.origin, got, tc.want)
			}
		})
	}
}

// A request with no Origin is not a browser request: Home Assistant's Ollama
// integration, the provisioning script and curl all send none. Refusing them
// would break every non-browser client while protecting nothing, since the
// header is only trustworthy because browsers set it and won't let script
// override it.
func TestOriginGateAllowsAbsentOrigin(t *testing.T) {
	if !newOriginGate(nil).allowed("") {
		t.Error("a request with no Origin header must pass")
	}
}

func TestOriginGateWildcard(t *testing.T) {
	g := newOriginGate([]string{"*"})
	if !g.allowed("https://anything.example") {
		t.Error("wildcard must allow any origin")
	}
	if got := g.list(); len(got) != 1 || !strings.HasPrefix(got[0], "*") {
		t.Errorf("list() = %v, want the wildcard called out", got)
	}
}

func TestOriginGateNormalisation(t *testing.T) {
	// Users type these into the add-on's `allowed_origins` option by hand.
	g := newOriginGate([]string{
		" https://HOME-assistant.Example.com/ ",
		"",
	})
	for _, o := range []string{
		"https://home-assistant.example.com",
		"https://HOME-ASSISTANT.EXAMPLE.COM",
		"https://home-assistant.example.com/",
	} {
		if !g.allowed(o) {
			t.Errorf("allowed(%q) = false; scheme and host are case-insensitive and a trailing slash is not part of an origin", o)
		}
	}
}

func TestOriginOf(t *testing.T) {
	tests := []struct{ in, want string }{
		{"https://home-assistant.example.com", "https://home-assistant.example.com"},
		{"https://home-assistant.example.com/", "https://home-assistant.example.com"},
		{"http://192.0.2.10:8123/lovelace/0", "http://192.0.2.10:8123"},
		{"https://abc123.ui.nabu.casa/", "https://abc123.ui.nabu.casa"},
		{"", ""},
		{"not a url", ""},
		{"/relative/only", ""},
	}
	for _, tc := range tests {
		if got := originOf(tc.in); got != tc.want {
			t.Errorf("originOf(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSetDerivedReportsOnlyRealChanges(t *testing.T) {
	g := newOriginGate([]string{"http://192.0.2.10:13305"})

	if !g.setDerived([]string{"https://ha.example"}) {
		t.Fatal("first set must report a change")
	}
	if g.setDerived([]string{"https://ha.example"}) {
		t.Error("setting the identical list must not report a change (it would log every refresh)")
	}
	if g.setDerived([]string{"https://ha.example", "https://ha.example"}) {
		t.Error("a duplicate is the same set and must not report a change")
	}
	if !g.setDerived([]string{"https://other.example"}) {
		t.Fatal("a different list must report a change")
	}
	if g.allowed("https://ha.example") {
		t.Error("the replaced origin must no longer be allowed")
	}
	if !g.allowed("http://192.0.2.10:13305") {
		t.Error("refreshing HA origins must not disturb the static list")
	}
}

func newFakeSupervisor(t *testing.T, token string, cfg func() (int, string)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/core/api/config" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		code, body := cfg()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchHAOrigins(t *testing.T) {
	sup := newFakeSupervisor(t, "tok", func() (int, string) {
		return http.StatusOK, `{"internal_url":"https://ha.example","external_url":"https://remote.example/","location_name":"Home"}`
	})

	got, err := fetchHAOrigins(context.Background(), sup.Client(), sup.URL, "tok")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	want := []string{"https://ha.example", "https://remote.example"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
		}
	}
}

// Core answering 502 is the NORMAL state for the first minute of a host boot:
// Supervisor starts `startup: services` add-ons before Core. It must surface as
// an error to retry, never as an empty-but-successful list — silently accepting
// emptiness is exactly the bug this file exists to fix.
func TestFetchHAOriginsErrorsWhileCoreIsDown(t *testing.T) {
	sup := newFakeSupervisor(t, "tok", func() (int, string) {
		return http.StatusBadGateway, `{"message":"Home Assistant is not running"}`
	})
	if _, err := fetchHAOrigins(context.Background(), sup.Client(), sup.URL, "tok"); err == nil {
		t.Fatal("a 502 from Supervisor must be an error, not an empty result")
	}
}

func TestFetchHAOriginsToleratesUnsetURLs(t *testing.T) {
	sup := newFakeSupervisor(t, "tok", func() (int, string) {
		return http.StatusOK, `{"internal_url":null,"external_url":""}`
	})
	got, err := fetchHAOrigins(context.Background(), sup.Client(), sup.URL, "tok")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}

// The regression this whole change exists to prevent: the add-on starts before
// Core, so the ingress origin is NOT allowed at startup and must become allowed
// on its own, without an add-on restart.
func TestIngressOriginBecomesAllowedOnceCoreIsUp(t *testing.T) {
	const ingress = "https://home-assistant.example.com"

	coreUp := make(chan struct{})
	sup := newFakeSupervisor(t, "tok", func() (int, string) {
		select {
		case <-coreUp:
			return http.StatusOK, `{"internal_url":"` + ingress + `","external_url":null}`
		default:
			return http.StatusBadGateway, `{"message":"Home Assistant is not running"}`
		}
	})

	g := newOriginGate([]string{"http://192.0.2.10:13305"})
	go g.watch(sup.Client(), sup.URL, "tok", 5*time.Millisecond, 5*time.Millisecond)

	if g.allowed(ingress) {
		t.Fatal("precondition: the ingress origin must not be allowed before Core answers")
	}
	close(coreUp)

	deadline := time.Now().Add(3 * time.Second)
	for !g.allowed(ingress) {
		if time.Now().After(deadline) {
			t.Fatalf("ingress origin %q never became allowed; list=%v", ingress, g.list())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Without a token the gate cannot learn Home Assistant's URLs. It must give up
// rather than spin, and must not wrongly widen the allowlist.
func TestWatchWithoutTokenReturns(t *testing.T) {
	g := newOriginGate(nil)
	done := make(chan struct{})
	go func() {
		g.watch(http.DefaultClient, "http://127.0.0.1:1", "", time.Millisecond, time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watch must return immediately when no SUPERVISOR_TOKEN is set")
	}
	if g.allowed("https://anything.example") {
		t.Error("a missing token must not widen the allowlist")
	}
}

// End to end through the handler: the refusal must be indistinguishable from
// lemond's own, because the web app and existing API clients already handle
// that exact response.
func TestServeHTTPGatesOnOrigin(t *testing.T) {
	var reachedUpstream bool
	srv := newTestServer(t, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reachedUpstream = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	srv.origins = newOriginGate([]string{"https://ha.example"})

	t.Run("disallowed origin is refused before the upstream is touched", func(t *testing.T) {
		reachedUpstream = false
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/unload", nil)
		req.Header.Set("Origin", "https://evil.example")
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", rec.Code)
		}
		if body := strings.TrimSpace(rec.Body.String()); body != `{"error": "Origin not allowed"}` {
			t.Errorf("body = %q, want lemond's exact refusal", body)
		}
		if reachedUpstream {
			t.Error("a refused request must never reach lemond")
		}
	})

	t.Run("allowed origin passes through", func(t *testing.T) {
		reachedUpstream = false
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/unload", nil)
		req.Header.Set("Origin", "https://ha.example")
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
		if !reachedUpstream {
			t.Error("an allowed request must reach lemond")
		}
	})

	// lemond 11.5.1 refuses disallowed origins on GET too. The Web UI renders
	// only because navigations and subresource loads send no Origin at all.
	t.Run("GET is gated as well", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		req.Header.Set("Origin", "https://evil.example")
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", rec.Code)
		}
	})

	t.Run("no Origin passes", func(t *testing.T) {
		reachedUpstream = false
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
		if rec.Code != http.StatusOK || !reachedUpstream {
			t.Errorf("status = %d, upstream reached = %v; want 200 and true", rec.Code, reachedUpstream)
		}
	})
}

func TestSplitList(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"a", 1},
		{"a,b,c", 3},
		{" a , b ", 2},
		{"a,,b", 2},
		{",", 0},
	}
	for _, tc := range tests {
		if got := splitList(tc.in); len(got) != tc.want {
			t.Errorf("splitList(%q) = %v, want %d entries", tc.in, got, tc.want)
		}
	}
}

// The regression that made direct-IP access 403 while the configured URL
// worked: Home Assistant is commonly served over TLS, and matching is exact.
func TestBuildDerivedEmitsBothSchemes(t *testing.T) {
	got := buildDerived([]string{"192.0.2.10"}, "13305", []string{"https://ha.example.com"})
	set := map[string]bool{}
	for _, o := range got {
		set[o] = true
	}
	for _, want := range []string{
		"https://192.0.2.10:8123", // <- the one that was missing
		"http://192.0.2.10:8123",
		"https://192.0.2.10:13305",
		"http://192.0.2.10:13305",
		"https://homeassistant.local:8123",
		"http://homeassistant.local:8123",
		"https://ha.example.com",
	} {
		if !set[want] {
			t.Errorf("missing %q from %v", want, got)
		}
	}
}

func TestBuildDerivedBracketsIPv6AndLearnsCorePort(t *testing.T) {
	got := buildDerived([]string{"2001:db8::1"}, "11434", []string{"http://ha.example.com:8443"})
	set := map[string]bool{}
	for _, o := range got {
		set[o] = true
	}
	for _, want := range []string{
		"https://[2001:db8::1]:8123",
		"https://[2001:db8::1]:11434", // remapped published port
		"https://[2001:db8::1]:8443",  // non-default Core port, learned from the URL
	} {
		if !set[want] {
			t.Errorf("missing %q from %v", want, got)
		}
	}
}

// Serves all three endpoints. coreUp gates only /core/api/config, so a test can
// hold Core down while Supervisor answers — the real boot sequence.
func newFakeHA(t *testing.T, coreUp <-chan struct{}) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/network/info":
			_, _ = w.Write([]byte(`{"data":{"interfaces":[
				{"ipv4":{"address":["192.0.2.10/24","169.254.1.1/16"]},
				 "ipv6":{"address":["2001:db8::1/64","fe80::1/64"]}}]}}`))
		case "/addons/self/info":
			_, _ = w.Write([]byte(`{"data":{"network":{"13305/tcp":11434}}}`))
		case "/core/api/config":
			select {
			case <-coreUp:
				_, _ = w.Write([]byte(`{"internal_url":"https://ha.example.com","external_url":null}`))
			default:
				w.WriteHeader(http.StatusBadGateway)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchHostAddressesSkipsLinkLocal(t *testing.T) {
	sup := newFakeHA(t, nil)
	got, err := fetchHostAddresses(context.Background(), sup.Client(), sup.URL, "tok")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	want := []string{"192.0.2.10", "2001:db8::1"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v (link-local is unusable as an origin)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestFetchPublishedPortHonoursRemap(t *testing.T) {
	sup := newFakeHA(t, nil)
	got, err := fetchPublishedPort(context.Background(), sup.Client(), sup.URL, "tok")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if got != "11434" {
		t.Errorf("got %q, want 11434 — the browser's Origin carries the published port", got)
	}
}

// Core is down for the first minute of a host boot, but the device's own
// addresses come from Supervisor, which is necessarily up. They must be allowed
// immediately rather than waiting on Core.
func TestHostOriginsWorkWhileCoreIsDown(t *testing.T) {
	coreUp := make(chan struct{})
	sup := newFakeHA(t, coreUp)

	g := newOriginGate(nil)
	go g.watch(sup.Client(), sup.URL, "tok", 5*time.Millisecond, 5*time.Millisecond)

	waitFor := func(origin string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for !g.allowed(origin) {
			if time.Now().After(deadline) {
				t.Fatalf("%q never became allowed; list=%v", origin, g.list())
			}
			time.Sleep(5 * time.Millisecond)
		}
	}

	waitFor("https://192.0.2.10:8123")
	if g.allowed("https://ha.example.com") {
		t.Fatal("precondition: Core is still down, its URL must not be allowed yet")
	}

	close(coreUp)
	waitFor("https://ha.example.com")

	if !g.allowed("https://192.0.2.10:8123") {
		t.Error("Core coming up must not drop the host addresses")
	}
}
