package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const testPrefix = "/api/hassio_ingress/abcDEF_123-token"

// A cut-down copy of a page as a SvelteKit app built for the site root serves it.
const shell = `<!doctype html>
<html lang="%lang%">
	<head>
		<meta charset="utf-8" />
		<link rel="icon" href="/api/app/images/favicon" />
		<link rel="manifest" href="/app.webmanifest" />
		<script>
			try { document.documentElement.lang = 'en'; } catch (e) {}
		</script>
		<link href="/_app/immutable/entry/start.Dmo5_DOC.js" rel="modulepreload">
		<link href="/_app/immutable/assets/root.DO1UlrXJ.css" rel="stylesheet">
		<link href="//cdn.example/x.css" rel="stylesheet">
	</head>
	<body data-sveltekit-preload-data="hover">
		<div style="display: contents">
			<script>
				{
					__sveltekit_b9g78x = { base: "", version: "1795018593967" };
					import("/_app/immutable/entry/start.Dmo5_DOC.js").then(async (kit) => {
						const app = await import("/_app/immutable/entry/app.CPFsnE8-.js");
					});
					const script_url = '/service-worker.js';
				}
			</script>
		</div>
	</body>
</html>`

func TestIngressOf(t *testing.T) {
	cases := []struct {
		path, proto string
		ok, https   bool
	}{
		{testPrefix, "https", true, true},
		{testPrefix, "HTTPS", true, true},
		{testPrefix, "http", true, false},
		{testPrefix, "", true, false},
		{"", "https", false, false},
		{"/api/hassio_ingress/short", "", false, false},
		{testPrefix + "/", "", false, false},
		{testPrefix + `"><script>alert(1)</script>`, "", false, false},
		{"/somewhere/else/abcDEF_123-token", "", false, false},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		if c.path != "" {
			r.Header.Set("X-Ingress-Path", c.path)
		}
		if c.proto != "" {
			r.Header.Set("X-Forwarded-Proto", c.proto)
		}
		in, ok := ingressOf(r)
		if ok != c.ok || in.https != c.https || (ok && in.prefix != c.path) {
			t.Errorf("ingressOf(%q, %q) = %+v, %v", c.path, c.proto, in, ok)
		}
	}
}

func TestRewritePage(t *testing.T) {
	out := string(rewritePage([]byte(shell), testPrefix))

	for _, want := range []string{
		`<link rel="icon" href="` + testPrefix + `/api/app/images/favicon" />`,
		`<link href="` + testPrefix + `/_app/immutable/entry/start.Dmo5_DOC.js" rel="modulepreload">`,
		`<link href="` + testPrefix + `/_app/immutable/assets/root.DO1UlrXJ.css" rel="stylesheet">`,
		`import("` + testPrefix + `/_app/immutable/entry/start.Dmo5_DOC.js")`,
		`import("` + testPrefix + `/_app/immutable/entry/app.CPFsnE8-.js")`,
		`<link href="//cdn.example/x.css" rel="stylesheet">`, // another host: untouched
		`__sveltekit_b9g78x = { base: "", version`,           // the app keeps believing it is at the root
		`const script_url = '/service-worker.js';`,           // the shim turns registration off instead
		`window.__haIngressPath="` + testPrefix + `";window.__haIngressOpenAt="";`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rewritten page lacks %q", want)
		}
	}
	for _, bad := range []string{`rel="manifest"`, `href="/_app`, `href="/api/app`, `import("/_app`} {
		if strings.Contains(out, bad) {
			t.Errorf("rewritten page still has %q", bad)
		}
	}

	shim := strings.Index(out, "window.__haIngressPath")
	charset := strings.Index(out, `<meta charset="utf-8"`)
	firstOwn := strings.Index(out, "document.documentElement.lang")
	if !(charset >= 0 && charset < shim && shim < firstOwn) {
		t.Errorf("shim at %d must come after the charset (%d) and before the page's first script (%d)", shim, charset, firstOwn)
	}
	if charset > 1024 {
		t.Errorf("charset declaration at byte %d: browsers only look in the first 1024", charset)
	}
}

// An app may write its entry imports relative to the page. The page's address
// loses the prefix before that script runs, so they have to name it too.
func TestRewritePageRelativeImports(t *testing.T) {
	page := `<html><head><link rel="icon" href="/favicon-32x32.png" /></head><body><script>
		__sveltekit_x = { base: new URL(".", location).pathname.slice(0, -1) };
		Promise.all([import("./_app/immutable/entry/start.js"), import("./_app/immutable/entry/app.js")]);
	</script></body></html>`
	out := string(rewritePage([]byte(page), testPrefix))
	for _, want := range []string{
		`import("` + testPrefix + `/_app/immutable/entry/start.js")`,
		`import("` + testPrefix + `/_app/immutable/entry/app.js")`,
		`href="` + testPrefix + `/favicon-32x32.png"`,
		`base: new URL(".", location)`, // left to the app: by then the address has no prefix
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rewritten page lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, `import("./`) {
		t.Errorf("a relative import is left: it would be asked of Home Assistant itself\n%s", out)
	}
}

func TestOpenAt(t *testing.T) {
	defer func() { openAt = "" }()
	openAt = "/login"
	out := string(rewritePage([]byte(shell), testPrefix))
	if !strings.Contains(out, `window.__haIngressOpenAt="/login";`) {
		t.Errorf("the page to open is not handed to the shim")
	}
	for path, ok := range map[string]bool{"/login": true, "/a/b-c_d.e": true, "": false, "login": false, `/x";alert(1)//`: false, "/x y": false} {
		if got := plainPath.MatchString(path); got != ok {
			t.Errorf("plainPath(%q) = %v, want %v", path, got, ok)
		}
	}
}

func TestAllowHomeAssistantFrame(t *testing.T) {
	h := http.Header{}
	h.Set("X-Frame-Options", "DENY")
	h.Add("Content-Security-Policy", "frame-ancestors 'none'")
	h.Add("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; img-src data:")
	allowHomeAssistantFrame(h)
	if got := h.Get("X-Frame-Options"); got != "SAMEORIGIN" {
		t.Errorf("X-Frame-Options = %q, want SAMEORIGIN", got)
	}
	want := []string{"frame-ancestors 'self'", "default-src 'self'; frame-ancestors 'self'; img-src data:"}
	if got := h.Values("Content-Security-Policy"); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Content-Security-Policy = %q, want %q", got, want)
	}
	// An app that sets neither gets neither.
	none := http.Header{}
	allowHomeAssistantFrame(none)
	if len(none) != 0 {
		t.Errorf("headers were added to an answer that had none: %v", none)
	}
}

func TestRewritePageOtherPages(t *testing.T) {
	// A page that is not the web app itself, such as an API reference, has the same needs.
	docs := `<html><head><title>API</title></head><body><script id="api-reference" data-url="/api/openapi.json"></script><script src="/api/docs/scalar.js"></script></body></html>`
	out := string(rewritePage([]byte(docs), testPrefix))
	if !strings.Contains(out, `<script src="`+testPrefix+`/api/docs/scalar.js">`) {
		t.Errorf("script address not moved: %s", out)
	}
	if !strings.Contains(out, `data-url="/api/openapi.json"`) {
		t.Errorf("data-url must be left for the shim's fetch: %s", out)
	}
	if strings.Index(out, "__haIngressPath") > strings.Index(out, `id="api-reference"`) {
		t.Errorf("shim must load before the page's first script: %s", out)
	}

	plain := `<html><body><a href="/">home</a><p>no scripts</p></body></html>`
	if got := string(rewritePage([]byte(plain), testPrefix)); got != `<html><body><a href="`+testPrefix+`/">home</a><p>no scripts</p></body></html>` {
		t.Errorf("page without scripts: %s", got)
	}
}

func TestShimIsSafeToInline(t *testing.T) {
	if strings.Contains(strings.ToLower(shimJS), "</script") {
		t.Error("shim.js contains </script, which would end the inline script early")
	}
	for i, b := range []byte(shimJS) {
		if b >= 0x80 {
			t.Fatalf("shim.js has a non-ASCII byte at %d: Home Assistant drops the page's charset header", i)
		}
	}
	if !strings.Contains(shimJS, "window.__haIngressPath") {
		t.Error("shim.js does not read window.__haIngressPath")
	}
}

func TestRewriteCSS(t *testing.T) {
	cases := map[string]string{
		`src: url('/fonts/MonaSans.woff2') format('woff2');`: `src: url('` + testPrefix + `/fonts/MonaSans.woff2') format('woff2');`,
		`src:url(/fonts/a.woff2)`:                            `src:url(` + testPrefix + `/fonts/a.woff2)`,
		`background: url( "/img/x.png" )`:                    `background: url( "` + testPrefix + `/img/x.png" )`,
		`url(//cdn.example/x.png)`:                           `url(//cdn.example/x.png)`,
		`url(data:image/png;base64,AAAA/BBB)`:                `url(data:image/png;base64,AAAA/BBB)`,
		`url(../assets/x.woff2)`:                             `url(../assets/x.woff2)`,
		`url("https://example.com/x.png")`:                   `url("https://example.com/x.png")`,
	}
	for in, want := range cases {
		if got := string(rewriteCSS([]byte(in), testPrefix)); got != want {
			t.Errorf("rewriteCSS(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRewriteSetCookie(t *testing.T) {
	plain := ingress{prefix: testPrefix}
	secure := ingress{prefix: testPrefix, https: true}
	cases := []struct {
		in   ingress
		got  string
		want string
	}{
		{plain, "token=abc; Path=/; Max-Age=3600; HttpOnly; SameSite=Lax",
			"ingress.token=abc; Path=" + testPrefix + "/; Max-Age=3600; HttpOnly; SameSite=Lax"},
		{plain, "refresh=xyz; Path=/api/auth; HttpOnly; SameSite=Strict",
			"ingress.refresh=xyz; Path=" + testPrefix + "/api/auth; HttpOnly; SameSite=Strict"},
		{plain, "token.1=chunk; Path=/", "ingress.token.1=chunk; Path=" + testPrefix + "/"},
		{plain, "token=; Path=/; Max-Age=0; HttpOnly", "ingress.token=; Path=" + testPrefix + "/; Max-Age=0; HttpOnly"},
		{plain, "oidc_state=s", "ingress.oidc_state=s; Path=" + testPrefix + "/"},
		{plain, "a=b; Domain=example.com; Path=/", "ingress.a=b; Path=" + testPrefix + "/"},
		{secure, "token=abc; Path=/; HttpOnly", "ingress.token=abc; Path=" + testPrefix + "/; HttpOnly; Secure"},
		{secure, "token=abc; Path=/; Secure", "ingress.token=abc; Path=" + testPrefix + "/; Secure"},
		// A __Host- cookie may only have Path=/, so it must not keep that name.
		{secure, "__Host-token=abc; Path=/; Secure; HttpOnly", "ingress.__Host-token=abc; Path=" + testPrefix + "/; Secure; HttpOnly"},
		{plain, "v=a=b; Path=/", "ingress.v=a=b; Path=" + testPrefix + "/"},
	}
	for _, c := range cases {
		if got := rewriteSetCookie(c.got, c.in); got != c.want {
			t.Errorf("rewriteSetCookie(%q)\n got %q\nwant %q", c.got, got, c.want)
		}
	}
}

func TestPanelCookies(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"ingress_session=ha; ingress.token=mine; token=direct; ingress.refresh=r; locale=en"}, "token=mine; refresh=r"},
		{[]string{"token=direct; refresh=direct"}, ""},
		{[]string{"ingress.token=a", "ingress.token.1=b"}, "token=a; token.1=b"},
		{[]string{"ingress.v=a=b"}, "v=a=b"},
		{nil, ""},
	}
	for _, c := range cases {
		if got := panelCookies(c.in); got != c.want {
			t.Errorf("panelCookies(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// app is a stand-in for the app behind the proxy. It records what reached it.
type app struct {
	*httptest.Server
	last *http.Request
}

func newApp(t *testing.T) *app {
	a := &app{}
	a.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.last = r.Clone(r.Context())
		switch r.URL.Path {
		case "/api/auth/login":
			w.Header().Add("Set-Cookie", "token=t1; Path=/; HttpOnly; SameSite=Lax")
			w.Header().Add("Set-Cookie", "refresh=r1; Path=/api/auth; HttpOnly; SameSite=Strict")
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"path":"/api/x"}`)
		case "/old":
			http.Redirect(w, r, "/new?x=1", http.StatusFound)
		case "/away":
			w.Header().Set("Location", "https://example.com/")
			w.WriteHeader(http.StatusFound)
		case "/_app/immutable/assets/root.css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
			w.Header().Set("ETag", `"abc"`)
			io.WriteString(w, `@font-face{src:url('/fonts/MonaSans.woff2')}`)
		case "/_app/big.css":
			w.Header().Set("Content-Type", "text/css")
			w.Write(bytes.Repeat([]byte("a{b:url(/x)}"), maxRewrite/12+1))
		case "/_app/immutable/entry/start.js":
			w.Header().Set("Content-Type", "text/javascript")
			io.WriteString(w, `fetch("/api/x")`)
		case "/ws":
			conn, buf, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			defer conn.Close()
			io.WriteString(buf, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
			buf.Flush()
			line, _ := buf.ReadString('\n')
			io.WriteString(buf, "echo:"+line)
			buf.Flush()
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("ETag", `"page"`)
			// An app that forbids framing, as a manager of Docker should.
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
			io.WriteString(w, shell)
		}
	}))
	t.Cleanup(a.Close)
	return a
}

func newTestProxy(t *testing.T, a *app) *httptest.Server {
	upstream, err := url.Parse(a.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(newProxy(upstream))
	t.Cleanup(proxy.Close)
	return proxy
}

// get asks the proxy the way Home Assistant does when ingress is true.
func get(t *testing.T, proxy *httptest.Server, path string, ingress bool, header map[string]string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest("GET", proxy.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "homeassistant.example:8123"
	if ingress {
		req.Header.Set("X-Ingress-Path", testPrefix)
		req.Header.Set("X-Forwarded-For", "192.0.2.7, 172.30.32.1")
		req.Header.Set("X-Forwarded-Host", "homeassistant.example:8123")
		req.Header.Set("X-Forwarded-Proto", "http")
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	client := &http.Client{
		Transport:     &http.Transport{DisableCompression: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(body)
}

func TestProxyAdaptsIngressRequests(t *testing.T) {
	a := newApp(t)
	proxy := newTestProxy(t, a)

	resp, body := get(t, proxy, "/containers", true, map[string]string{"Accept-Encoding": "gzip, br"})
	if !strings.Contains(body, `window.__haIngressPath="`+testPrefix+`"`) || strings.Contains(body, `href="/_app`) {
		t.Errorf("page not adapted:\n%s", body)
	}
	if got := resp.Header.Get("Content-Length"); got != fmt.Sprint(len(body)) {
		t.Errorf("Content-Length %q for a body of %d bytes", got, len(body))
	}
	if resp.Header.Get("ETag") != "" {
		t.Error("ETag of the original page was kept")
	}
	if xfo, csp := resp.Header.Get("X-Frame-Options"), resp.Header.Get("Content-Security-Policy"); xfo != "SAMEORIGIN" || csp != "frame-ancestors 'self'" {
		t.Errorf("framing through ingress: X-Frame-Options %q, Content-Security-Policy %q: Home Assistant could not show the panel", xfo, csp)
	}
	if a.last.Host != "homeassistant.example:8123" {
		t.Errorf("the app saw Host %q: its cross-origin check needs the browser's", a.last.Host)
	}
	if got := a.last.Header.Get("Accept-Encoding"); got != "" {
		t.Errorf("Accept-Encoding %q reached the app: the page could come back compressed", got)
	}
	for _, h := range []string{"X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto"} {
		if got := a.last.Header.Get(h); got != "" {
			t.Errorf("%s: %q reached the app", h, got)
		}
	}

	_, body = get(t, proxy, "/_app/immutable/assets/root.css", true, nil)
	if body != `@font-face{src:url('`+testPrefix+`/fonts/MonaSans.woff2')}` {
		t.Errorf("style sheet: %s", body)
	}

	_, body = get(t, proxy, "/_app/immutable/entry/start.js", true, nil)
	if body != `fetch("/api/x")` {
		t.Errorf("a script was changed: %s", body)
	}

	resp, _ = get(t, proxy, "/old", true, nil)
	if got := resp.Header.Get("Location"); got != testPrefix+"/new?x=1" {
		t.Errorf("redirect to %q", got)
	}
	resp, _ = get(t, proxy, "/away", true, nil)
	if got := resp.Header.Get("Location"); got != "https://example.com/" {
		t.Errorf("redirect to another site became %q", got)
	}

	_, body = get(t, proxy, "/_app/big.css", true, nil)
	if len(body) <= maxRewrite || strings.Contains(body, testPrefix) {
		t.Errorf("an oversized body must pass through whole and unchanged: %d bytes", len(body))
	}
}

func TestProxyKeepsThePanelSessionApart(t *testing.T) {
	a := newApp(t)
	proxy := newTestProxy(t, a)

	resp, body := get(t, proxy, "/api/auth/login", true, map[string]string{
		"Cookie": "ingress_session=ha; token=direct; ingress.token=mine; ingress.refresh=r",
	})
	if got := a.last.Header.Get("Cookie"); got != "token=mine; refresh=r" {
		t.Errorf("the app saw Cookie %q", got)
	}
	want := []string{
		"ingress.token=t1; Path=" + testPrefix + "/; HttpOnly; SameSite=Lax",
		"ingress.refresh=r1; Path=" + testPrefix + "/api/auth; HttpOnly; SameSite=Strict",
	}
	if got := resp.Header.Values("Set-Cookie"); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Set-Cookie:\n got %q\nwant %q", got, want)
	}
	if body != `{"path":"/api/x"}` {
		t.Errorf("JSON was changed: %s", body)
	}

	get(t, proxy, "/api/auth/login", true, map[string]string{"Cookie": "token=direct"})
	if got, present := a.last.Header["Cookie"]; present {
		t.Errorf("a cookie of the app's own port reached it through the panel: %q", got)
	}

	resp, _ = get(t, proxy, "/api/auth/login", true, map[string]string{"X-Forwarded-Proto": "https"})
	for _, c := range resp.Header.Values("Set-Cookie") {
		if !strings.HasSuffix(c, "; Secure") {
			t.Errorf("over HTTPS a cookie lacks Secure: %q", c)
		}
	}
}

func TestProxyLeavesOtherRequestsAlone(t *testing.T) {
	a := newApp(t)
	proxy := newTestProxy(t, a)

	resp, body := get(t, proxy, "/", false, map[string]string{"Cookie": "token=direct", "Accept-Encoding": "identity"})
	if body != shell {
		t.Errorf("page changed without the ingress header:\n%s", body)
	}
	if resp.Header.Get("ETag") != `"page"` {
		t.Error("ETag dropped without the ingress header")
	}
	if xfo, csp := resp.Header.Get("X-Frame-Options"), resp.Header.Get("Content-Security-Policy"); xfo != "DENY" || csp != "frame-ancestors 'none'" {
		t.Errorf("framing without the ingress header: X-Frame-Options %q, Content-Security-Policy %q: the app's own protection must stand", xfo, csp)
	}
	if got := a.last.Header.Get("Cookie"); got != "token=direct" {
		t.Errorf("Cookie %q", got)
	}
	if got := a.last.Header.Get("Accept-Encoding"); got != "identity" {
		t.Errorf("Accept-Encoding %q", got)
	}

	resp, _ = get(t, proxy, "/api/auth/login", false, nil)
	if got := resp.Header.Values("Set-Cookie"); len(got) != 2 || !strings.HasPrefix(got[0], "token=t1; Path=/;") {
		t.Errorf("Set-Cookie changed without the ingress header: %q", got)
	}

	// A header that does not look like Supervisor's is not trusted.
	_, body = get(t, proxy, "/", false, map[string]string{"X-Ingress-Path": `/x"><script>alert(1)</script>`})
	if body != shell {
		t.Error("an invalid X-Ingress-Path changed the page")
	}
}

func TestProxyPassesWebSockets(t *testing.T) {
	a := newApp(t)
	proxy := newTestProxy(t, a)

	conn, err := net.Dial("tcp", strings.TrimPrefix(proxy.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "GET /ws HTTP/1.1\r\nHost: homeassistant.example:8123\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n"+
		"X-Ingress-Path: %s\r\nCookie: token=direct; ingress.token=mine\r\n\r\n", testPrefix)
	r := bufio.NewReader(conn)
	status, err := r.ReadString('\n')
	if err != nil || !strings.HasPrefix(status, "HTTP/1.1 101") {
		t.Fatalf("upgrade answered %q, %v", status, err)
	}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\r\n" {
			break
		}
	}
	fmt.Fprint(conn, "hello\n")
	if line, err := r.ReadString('\n'); err != nil || line != "echo:hello\n" {
		t.Errorf("through the tunnel: %q, %v", line, err)
	}
	if got := a.last.Header.Get("Cookie"); got != "token=mine" {
		t.Errorf("handshake Cookie %q", got)
	}
}

func TestProxyWhenTheAppIsDown(t *testing.T) {
	a := newApp(t)
	proxy := newTestProxy(t, a)
	a.Close()

	resp, body := get(t, proxy, "/", true, nil)
	if resp.StatusCode != http.StatusBadGateway || !strings.Contains(body, "not answering") {
		t.Errorf("status %d, body %q", resp.StatusCode, body)
	}
}
