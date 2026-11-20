// ha-ingress-adapter serves a web app that expects to own the site root
// under Home Assistant ingress.
//
// Such an app names its files from the root, calls /api/... from the root and
// reads the page's path as if nothing came before it. Home Assistant serves a
// panel under /api/hassio_ingress/<token>/ instead.
//
// This proxy listens on the ingress port only. A request that carries Home
// Assistant's X-Ingress-Path header gets:
//
//   - pages: addresses from the site root moved under the prefix, and
//     shim.js loaded ahead of every other script
//   - style sheets: url(/...) moved under the prefix
//   - cookies: renamed and scoped to the prefix, so the panel's session is
//     separate from one opened on the app's own port
//   - redirects: moved under the prefix
//   - framing: allowed for the origin the panel is served from, which is
//     Home Assistant's own, and for no other
//
// Requests without the header pass untouched. The app's own port does not
// pass through here.
//
// Environment: UPSTREAM_URL (the app, required), INGRESS_LISTEN (default
// :8099), INGRESS_OPEN_AT (a path to open in place of "/", for an app whose
// "/" answers a signed-out visitor with a full page load).
package main

import (
	"bytes"
	"context"
	_ "embed"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

//go:embed shim.js
var shimJS string

// cookieMark is put in front of the name of every cookie the app sets through
// ingress. Only marked cookies are passed back to it.
const cookieMark = "ingress."

// maxRewrite bounds a body held in memory for rewriting. The page is ~4 KB
// and the largest style sheet ~0.5 MB; anything larger passes through as is.
const maxRewrite = 8 << 20

var (
	// What Supervisor sends: /api/hassio_ingress/<token>. Anything else is
	// not trusted to be written into a page.
	ingressPath = regexp.MustCompile(`^/api/hassio_ingress/[A-Za-z0-9_-]{8,128}$`)

	// href="/x" and src="/x", but not "//host/x".
	rootAttr = regexp.MustCompile(`\b(href|src)="/((?:[^/"][^"]*)?)"`)
	// import("/_app/...") in the page's inline script, and import("./_app/...")
	// from an app that writes them relative to the page: the page's address
	// loses the prefix before that script runs, so both have to name it.
	rootImport = regexp.MustCompile(`\bimport\("\.?/([^/"])`)
	// The web app manifest names its icons and start page from the site root,
	// and installing a panel as an app makes no sense.
	manifestLink = regexp.MustCompile(`<link\b[^>]*\brel="manifest"[^>]*>\s*`)
	firstScript  = regexp.MustCompile(`<script\b`)
	// url(/fonts/x), url('/x') and url("/x") in a style sheet.
	cssRootURL = regexp.MustCompile(`url\((\s*["']?)/([^/"')\s])`)
	// The frame-ancestors directive of a Content-Security-Policy.
	frameAncestors = regexp.MustCompile(`frame-ancestors[^;]*`)
	// A path the shim may be told to open: nothing that could end a string.
	plainPath = regexp.MustCompile(`^/[A-Za-z0-9/_.-]*$`)
)

// openAt is INGRESS_OPEN_AT: the path opened in place of "/", or empty.
var openAt string

// ingress describes a request that came through Home Assistant.
type ingress struct {
	prefix string // /api/hassio_ingress/<token>
	https  bool   // the browser reached Home Assistant over HTTPS
}

type ingressKey struct{}

// ingressOf reads Home Assistant's headers. ok is false for a request that
// did not come through ingress.
func ingressOf(r *http.Request) (in ingress, ok bool) {
	p := r.Header.Get("X-Ingress-Path")
	if !ingressPath.MatchString(p) {
		return ingress{}, false
	}
	return ingress{prefix: p, https: strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")}, true
}

// rewritePage moves a page's own addresses under prefix and loads the shim
// ahead of its first script.
func rewritePage(page []byte, prefix string) []byte {
	out := manifestLink.ReplaceAll(page, nil)
	out = rootAttr.ReplaceAll(out, []byte(`${1}="`+prefix+`/${2}"`))
	out = rootImport.ReplaceAll(out, []byte(`import("`+prefix+`/${1}`))

	loc := firstScript.FindIndex(out)
	if loc == nil {
		return out // no script, so nothing for the shim to adapt
	}
	shim := "<script>window.__haIngressPath=" + strconv.Quote(prefix) + ";window.__haIngressOpenAt=" + strconv.Quote(openAt) + ";" + shimJS + "</script>"
	var b bytes.Buffer
	b.Grow(len(out) + len(shim))
	b.Write(out[:loc[0]])
	b.WriteString(shim)
	b.Write(out[loc[0]:])
	return b.Bytes()
}

// rewriteCSS moves url(/...) under prefix.
func rewriteCSS(css []byte, prefix string) []byte {
	return cssRootURL.ReplaceAll(css, []byte(`url(${1}`+prefix+`/${2}`))
}

// rewriteSetCookie marks a cookie as the panel's and scopes it to the prefix.
// Path=/api/auth would never match a request under the prefix, and an
// unmarked Path=/ cookie could not be told from the direct port's.
func rewriteSetCookie(header string, in ingress) string {
	parts := strings.Split(header, ";")
	parts[0] = cookieMark + strings.TrimSpace(parts[0])
	hasPath, hasSecure := false, false
	for i := 1; i < len(parts); i++ {
		name, value, _ := strings.Cut(strings.TrimSpace(parts[i]), "=")
		switch {
		case strings.EqualFold(name, "Path"):
			hasPath = true
			if !strings.HasPrefix(value, "/") {
				value = "/"
			}
			parts[i] = " Path=" + in.prefix + value
		case strings.EqualFold(name, "Secure"):
			hasSecure = true
		case strings.EqualFold(name, "Domain"):
			parts[i] = "" // host-only, like every ingress cookie
		}
	}
	kept := parts[:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	out := strings.Join(kept, ";")
	if !hasPath {
		out += "; Path=" + in.prefix + "/"
	}
	if in.https && !hasSecure {
		out += "; Secure"
	}
	return out
}

// panelCookies returns the Cookie header to pass on: the marked cookies with
// the mark removed. Home Assistant's own cookies, and the app's from its own
// port on the same host name, are left out.
func panelCookies(header []string) string {
	var kept []string
	for _, line := range header {
		for _, c := range strings.Split(line, ";") {
			c = strings.TrimSpace(c)
			if rest, ok := strings.CutPrefix(c, cookieMark); ok {
				kept = append(kept, rest)
			}
		}
	}
	return strings.Join(kept, "; ")
}

func rewriteRequest(upstream *url.URL) func(*httputil.ProxyRequest) {
	return func(pr *httputil.ProxyRequest) {
		pr.SetURL(upstream)
		// An app's cross-origin check compares Origin with Host, so it has
		// to see the host the browser used. The X-Forwarded-* headers are
		// not passed on: an app is right to distrust them from a peer it
		// was not told about.
		pr.Out.Host = pr.In.Host

		in, ok := ingressOf(pr.In)
		if !ok {
			return
		}
		pr.Out = pr.Out.WithContext(context.WithValue(pr.Out.Context(), ingressKey{}, in))
		// Bodies are rewritten as plain text. Home Assistant never sends
		// this header to an app anyway.
		pr.Out.Header.Del("Accept-Encoding")
		if cookies := panelCookies(pr.In.Header.Values("Cookie")); cookies != "" {
			pr.Out.Header.Set("Cookie", cookies)
		} else {
			pr.Out.Header.Del("Cookie")
		}
	}
}

func modifyResponse(resp *http.Response) error {
	in, ok := resp.Request.Context().Value(ingressKey{}).(ingress)
	if !ok {
		return nil
	}

	if cookies := resp.Header.Values("Set-Cookie"); len(cookies) > 0 {
		resp.Header.Del("Set-Cookie")
		for _, c := range cookies {
			resp.Header.Add("Set-Cookie", rewriteSetCookie(c, in))
		}
	}
	if loc := resp.Header.Get("Location"); strings.HasPrefix(loc, "/") && !strings.HasPrefix(loc, "//") {
		resp.Header.Set("Location", in.prefix+loc)
	}
	allowHomeAssistantFrame(resp.Header)

	var rewrite func([]byte, string) []byte
	switch kind, _, _ := strings.Cut(resp.Header.Get("Content-Type"), ";"); strings.TrimSpace(kind) {
	case "text/html":
		rewrite = rewritePage
	case "text/css":
		rewrite = rewriteCSS
	default:
		return nil
	}
	if resp.Header.Get("Content-Encoding") != "" || resp.StatusCode == http.StatusNotModified || resp.Request.Method == http.MethodHead {
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRewrite+1))
	if err != nil {
		resp.Body.Close()
		return err
	}
	if len(body) > maxRewrite {
		resp.Body = struct {
			io.Reader
			io.Closer
		}{io.MultiReader(bytes.NewReader(body), resp.Body), resp.Body}
		return nil
	}
	resp.Body.Close()
	body = rewrite(body, in.prefix)
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	// The validators describe the app's bytes, not these.
	resp.Header.Del("ETag")
	resp.Header.Del("Last-Modified")
	return nil
}

// allowHomeAssistantFrame lets the page be framed by its own origin and by no
// other. The panel is a frame on Home Assistant's page, and Home Assistant
// serves the panel from its own origin, so "same origin" is exactly Home
// Assistant. An app that forbids framing altogether would show nothing.
func allowHomeAssistantFrame(h http.Header) {
	if h.Get("X-Frame-Options") != "" {
		h.Set("X-Frame-Options", "SAMEORIGIN")
	}
	if policies := h.Values("Content-Security-Policy"); len(policies) > 0 {
		h.Del("Content-Security-Policy")
		for _, policy := range policies {
			h.Add("Content-Security-Policy", frameAncestors.ReplaceAllString(policy, "frame-ancestors 'self'"))
		}
	}
}

func newProxy(upstream *url.URL) *httputil.ReverseProxy {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true // pass the app's encoding through untouched
	return &httputil.ReverseProxy{
		Rewrite:        rewriteRequest(upstream),
		ModifyResponse: modifyResponse,
		Transport:      transport,
		FlushInterval:  -1, // log and progress streams must not be buffered
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("ingress: %s %s: %v", r.Method, r.URL.Path, err)
			http.Error(w, "The app is not answering yet. Try again in a moment.", http.StatusBadGateway)
		},
	}
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func main() {
	upstream, err := url.Parse(os.Getenv("UPSTREAM_URL"))
	if err != nil || upstream.Host == "" {
		log.Fatalf("ingress: UPSTREAM_URL %q is not the app's address", os.Getenv("UPSTREAM_URL"))
	}
	if openAt = os.Getenv("INGRESS_OPEN_AT"); openAt != "" && !plainPath.MatchString(openAt) {
		log.Fatalf("ingress: INGRESS_OPEN_AT %q is not a plain path", openAt)
	}
	server := &http.Server{
		Addr:              env("INGRESS_LISTEN", ":8099"),
		Handler:           newProxy(upstream),
		ReadHeaderTimeout: 30 * time.Second,
	}
	log.Printf("ingress: serving Home Assistant ingress on %s for %s", server.Addr, upstream)
	log.Fatal(server.ListenAndServe())
}
