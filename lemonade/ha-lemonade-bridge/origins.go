package main

// Browser-Origin allowlist, enforced per request.
//
// Two sets. `static` is whatever the user typed into `allowed_origins`, passed
// in by cont-init. `derived` is everything the add-on can work out for itself —
// the device's own addresses and Home Assistant's configured URLs — refreshed
// at runtime by watch(), because none of it is reliably knowable at container
// start: Supervisor starts this add-on before Core, and a host address can
// change under DHCP.
//
// lemond is bound to loopback with LEMONADE_ALLOWED_ORIGINS=* and never gates
// anything itself. See lemonade/CLAUDE.md for why.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// originGate is safe for concurrent use: read per request, rewritten by watch.
type originGate struct {
	mu       sync.RWMutex
	wildcard bool
	static   map[string]struct{}
	derived  map[string]struct{}
}

func newOriginGate(static []string) *originGate {
	g := &originGate{
		static:  make(map[string]struct{}),
		derived: make(map[string]struct{}),
	}
	for _, o := range static {
		o = normalizeOrigin(o)
		if o == "" {
			continue
		}
		if o == "*" {
			g.wildcard = true
			continue
		}
		g.static[o] = struct{}{}
	}
	return g
}

// normalizeOrigin reduces a value to what a browser sends: scheme://host[:port],
// lowercased, no trailing slash. Origins have no path, so lowercasing all of it
// is safe.
func normalizeOrigin(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "/")
	return strings.ToLower(s)
}

// originOf reduces a full URL to its origin. HA's configured URLs may carry a path.
func originOf(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return normalizeOrigin(u.Scheme + "://" + u.Host)
}

// allowed reports whether a request carrying this Origin may proceed. An empty
// origin passes: non-browser clients (HA's Ollama integration, curl) send none,
// and only browsers are being constrained here.
func (g *originGate) allowed(origin string) bool {
	if origin == "" {
		return true
	}
	origin = normalizeOrigin(origin)

	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.wildcard {
		return true
	}
	if _, ok := g.static[origin]; ok {
		return true
	}
	_, ok := g.derived[origin]
	return ok
}

// setDerived replaces the discovered origins, reporting whether the set changed
// so the caller logs only real transitions.
func (g *originGate) setDerived(origins []string) bool {
	next := make(map[string]struct{}, len(origins))
	for _, o := range origins {
		if o = normalizeOrigin(o); o != "" {
			next[o] = struct{}{}
		}
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if len(next) == len(g.derived) {
		same := true
		for o := range next {
			if _, ok := g.derived[o]; !ok {
				same = false
				break
			}
		}
		if same {
			return false
		}
	}
	g.derived = next
	return true
}

// list returns the full allowlist, sorted, for logging.
func (g *originGate) list() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.wildcard {
		return []string{"* (all origins)"}
	}
	out := make([]string, 0, len(g.static)+len(g.derived))
	seen := make(map[string]struct{}, len(out))
	for _, set := range []map[string]struct{}{g.static, g.derived} {
		for o := range set {
			if _, dup := seen[o]; dup {
				continue
			}
			seen[o] = struct{}{}
			out = append(out, o)
		}
	}
	sort.Strings(out)
	return out
}

type httpStatusError struct{ code int }

func (e *httpStatusError) Error() string {
	return "HTTP " + strconv.Itoa(e.code)
}

// supervisorGET decodes a Supervisor endpoint into out. Non-200 is an error, not
// an empty result — "the API is not up yet" must never look like "nothing is
// configured".
func supervisorGET(ctx context.Context, client *http.Client, api, token, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &httpStatusError{code: resp.StatusCode}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// fetchHAOrigins reads HA's configured URLs. Errors while Core is still
// starting, which is normal for the first minute of a host boot.
func fetchHAOrigins(ctx context.Context, client *http.Client, api, token string) ([]string, error) {
	var cfg struct {
		InternalURL string `json:"internal_url"`
		ExternalURL string `json:"external_url"`
	}
	if err := supervisorGET(ctx, client, api, token, "/core/api/config", &cfg); err != nil {
		return nil, err
	}

	var out []string
	for _, raw := range []string{cfg.InternalURL, cfg.ExternalURL} {
		if o := originOf(raw); o != "" {
			out = append(out, o)
		}
	}
	return out, nil
}

// fetchHostAddresses lists every address the device answers on. Link-local is
// skipped: a browser cannot use one as an origin without a zone index.
func fetchHostAddresses(ctx context.Context, client *http.Client, api, token string) ([]string, error) {
	var info struct {
		Data struct {
			Interfaces []struct {
				IPv4 struct {
					Address []string `json:"address"`
				} `json:"ipv4"`
				IPv6 struct {
					Address []string `json:"address"`
				} `json:"ipv6"`
			} `json:"interfaces"`
		} `json:"data"`
	}
	if err := supervisorGET(ctx, client, api, token, "/network/info", &info); err != nil {
		return nil, err
	}

	var out []string
	for _, iface := range info.Data.Interfaces {
		for _, addr := range append(iface.IPv4.Address, iface.IPv6.Address...) {
			ip, _, _ := strings.Cut(addr, "/")
			if ip == "" || strings.HasPrefix(ip, "fe80:") || strings.HasPrefix(ip, "169.254.") {
				continue
			}
			out = append(out, ip)
		}
	}
	return out, nil
}

// fetchPublishedPort reports the host port 13305 is published on. Users remap it
// — ports_description suggests 11434 for Ollama client auto-detection — and the
// browser's Origin carries the published port, not lemond's.
func fetchPublishedPort(ctx context.Context, client *http.Client, api, token string) (string, error) {
	var info struct {
		Data struct {
			Network map[string]*int `json:"network"`
		} `json:"data"`
	}
	if err := supervisorGET(ctx, client, api, token, "/addons/self/info", &info); err != nil {
		return "", err
	}
	if p := info.Data.Network["13305/tcp"]; p != nil && *p > 0 {
		return strconv.Itoa(*p), nil
	}
	return "13305", nil
}

// buildDerived expands the discovered facts into concrete origins.
//
// BOTH schemes are emitted for every address. Home Assistant is commonly served
// over TLS, and an origin matches exactly — a device reached at
// https://<ip>:8123 does not match the http:// form, which is precisely the gap
// that made direct-IP access 403 while the configured URL worked.
//
// Emitting these is safe. An attacker's page carries ITS OWN origin in the
// header, so allowing the device's own addresses grants nothing to anyone who
// is not already serving from Home Assistant.
func buildDerived(addresses []string, publishedPort string, haOrigins []string) []string {
	ports := map[string]struct{}{"8123": {}} // Home Assistant's default
	if publishedPort != "" {
		ports[publishedPort] = struct{}{}
	}
	// A non-default Core port, learned from the URLs Home Assistant reports.
	for _, o := range haOrigins {
		if u, err := url.Parse(o); err == nil && u.Port() != "" {
			ports[u.Port()] = struct{}{}
		}
	}

	hosts := append([]string{}, addresses...)
	hosts = append(hosts, "homeassistant.local")

	out := append([]string{}, haOrigins...)
	for _, h := range hosts {
		if strings.Contains(h, ":") {
			h = "[" + h + "]" // IPv6 literals are bracketed in an origin
		}
		for p := range ports {
			out = append(out, "http://"+h+":"+p, "https://"+h+":"+p)
		}
	}
	return out
}

// watch keeps the derived origins current.
//
// Each source is fetched independently and the last good value is kept, so Core
// being down during boot does not also drop the host addresses — those come
// from Supervisor, which is necessarily up because it started us.
//
// Two cadences: fast until Core first answers, slow afterwards.
func (g *originGate) watch(client *http.Client, api, token string, boot, steady time.Duration) {
	if token == "" {
		logf("origins: no SUPERVISOR_TOKEN — cannot discover addresses; only 'allowed_origins' will be accepted")
		return
	}

	var addresses, haOrigins []string
	var port string
	warned := false

	for {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if v, err := fetchHostAddresses(ctx, client, api, token); err == nil {
			addresses = v
		}
		if v, err := fetchPublishedPort(ctx, client, api, token); err == nil {
			port = v
		}
		v, err := fetchHAOrigins(ctx, client, api, token)
		cancel()

		if err == nil {
			haOrigins, warned = v, false
		} else if !warned {
			logf("origins: Home Assistant not reachable yet (%v) — retrying every %s", err, boot)
			warned = true
		}

		if g.setDerived(buildDerived(addresses, port, haOrigins)) {
			logf("origins: allowlist now %s", strings.Join(g.list(), ", "))
		}

		if err != nil {
			time.Sleep(boot)
			continue
		}
		time.Sleep(steady)
	}
}

// rejectOrigin mirrors lemond's own refusal byte for byte, so clients that
// already handle it keep working.
func rejectOrigin(w http.ResponseWriter, origin string) {
	logf("origins: refused %q — add it to the add-on's 'allowed_origins' option if this is you", origin)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error": "Origin not allowed"}`))
}
