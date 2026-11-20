// ha-docker-filter keeps Home Assistant's containers out of what a Docker
// management tool sees. It listens on a unix socket, forwards to the real
// Docker socket, and changes four things on the way:
//
//   - GET /containers/json, /images/json and /system/df: Home Assistant's
//     entries are removed from the answer
//   - GET /events: records about those containers are dropped
//   - POST /containers/prune and /images/prune: one label exclusion is added,
//     so a prune cannot take a stopped app's container or image
//   - POST /containers/{id}/start and the other lifecycle verbs go out with no
//     body, which dockerd rejects when its length is unknown
//
// Everything else passes untouched, including attach, exec and build streams.
// A hidden container can still be reached by its ID: this hides, it does not
// enforce. rule.go says what counts as Home Assistant's.
//
// Docker Engine API reviewed for: 1.56 (Engine 29.8). A list-like endpoint
// that a later version adds would pass unfiltered, so the start line says
// when dockerd speaks a newer one.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// reviewedAPI is the newest Docker Engine API version whose endpoints were
// checked against this filter.
const reviewedAPI = "1.56"

var (
	// dockerd accepts /v1.56/, /v01.56/ and /v1.56./ alike, so match as loosely.
	versionPrefix = regexp.MustCompile(`^/v[0-9.]+(/.*)$`)
	// The verbs that take no body. dockerd answers 400 to a start whose body
	// length is unknown, before it looks at the container.
	lifecycle = regexp.MustCompile(`^/containers/.+/(start|stop|restart|kill|pause|unpause|wait)$`)
)

type rewriteKey struct{}

type filter struct {
	proxy  *httputil.ReverseProxy
	docker *http.Client // the filter's own questions to dockerd
	images imageCache
}

func newFilter(upstream string) *filter {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", upstream)
		},
		DisableCompression: true,
		MaxIdleConns:       16,
		IdleConnTimeout:    time.Minute,
	}
	f := &filter{docker: &http.Client{Transport: transport, Timeout: 30 * time.Second}}
	f.proxy = &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = "http"
			pr.Out.URL.Host = "docker"
			pr.Out.Host = "docker"
		},
		Transport:     transport,
		FlushInterval: -1, // logs, stats and events are read as they arrive
		ModifyResponse: func(resp *http.Response) error {
			rewrite, _ := resp.Request.Context().Value(rewriteKey{}).(func(*http.Response) error)
			if rewrite == nil || resp.StatusCode != http.StatusOK {
				return nil
			}
			return rewrite(resp)
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if !errors.Is(err, context.Canceled) {
				log.Printf("%s %s: %v", r.Method, r.URL.Path, err)
			}
			writeError(w, http.StatusBadGateway, "ha-docker-filter: "+err.Error())
		},
		ErrorLog: log.New(os.Stderr, "", 0),
	}
	return f
}

// route returns the path dockerd routes on, without its version prefix. It is
// empty for a path dockerd does not serve as written: dockerd then answers
// with a redirect or a 404 of its own, and the filter has nothing to decide.
func route(u *url.URL) string {
	p := u.Path // decoded, which is what dockerd matches: /containers/%6Ason is the list
	if p != path.Clean(p) {
		return ""
	}
	if m := versionPrefix.FindStringSubmatch(p); m != nil {
		p = m[1]
	}
	return p
}

func (f *filter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		// Without this the server may drop the rest of an upload once dockerd
		// starts to answer (a build, an image load).
		_ = http.NewResponseController(w).EnableFullDuplex()
	}

	var rewrite func(*http.Response) error
	switch rt := route(r.URL); {
	case r.Method == http.MethodGet && rt == "/containers/json":
		rewrite = containerList
	case r.Method == http.MethodGet && rt == "/images/json":
		rewrite = f.imageList
	case r.Method == http.MethodGet && rt == "/system/df":
		rewrite = f.diskUsage
	case r.Method == http.MethodGet && rt == "/events":
		rewrite = eventStream
	case r.Method == http.MethodPost && rt == "/containers/prune":
		if !excludeFromPrune(w, r, containerPruneLabel) {
			return
		}
	case r.Method == http.MethodPost && rt == "/images/prune":
		if !excludeFromPrune(w, r, imagePruneLabel) {
			return
		}
	case r.Method == http.MethodPost && lifecycle.MatchString(rt):
		dropBody(r)
	}
	if rewrite != nil {
		r.Header.Del("Accept-Encoding") // the answer has to be readable here
		r = r.WithContext(context.WithValue(r.Context(), rewriteKey{}, rewrite))
	}
	f.proxy.ServeHTTP(w, r)
}

// dropBody sends the request on with no body and Content-Length: 0.
func dropBody(r *http.Request) {
	r.Body = http.NoBody
	r.ContentLength = 0
	r.TransferEncoding = nil
	r.Header.Del("Content-Length")
	r.Header.Del("Content-Type")
}

// writeError answers in the shape dockerd uses for its own errors.
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, "{\"message\":%s}\n", strconv.Quote(message))
}

// newerThanReviewed reports whether dockerd's API version is past the one
// this filter was reviewed for.
func newerThanReviewed(api string) bool {
	parse := func(v string) (int, int) {
		major, minor, _ := strings.Cut(v, ".")
		a, _ := strconv.Atoi(major)
		b, _ := strconv.Atoi(minor)
		return a, b
	}
	a, b := parse(api)
	ra, rb := parse(reviewedAPI)
	return a > ra || (a == ra && b > rb)
}

func main() {
	listen := flag.String("listen", "/run/docker-filter/docker.sock", "unix socket to serve")
	upstream := flag.String("upstream", "/run/docker.sock", "the real Docker socket")
	gid := flag.Int("gid", -1, "group that may use the socket (-1: only this user)")
	flag.Parse()
	log.SetFlags(0)
	log.SetPrefix("ha-docker-filter: ")

	f := newFilter(*upstream)

	// A socket left by a hard kill would keep this start from binding, and
	// the tool would have no Docker at all.
	if err := os.Remove(*listen); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("remove stale socket: %v", err)
	}
	old := syscall.Umask(0o177)
	ln, err := net.Listen("unix", *listen)
	syscall.Umask(old)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	if *gid >= 0 {
		if err := os.Chown(*listen, -1, *gid); err != nil {
			log.Fatalf("chown socket: %v", err)
		}
		if err := os.Chmod(*listen, 0o660); err != nil {
			log.Fatalf("chmod socket: %v", err)
		}
	}

	note := ""
	if resp, err := f.docker.Get("http://docker/_ping"); err != nil {
		note = fmt.Sprintf(" (dockerd not reachable yet: %v)", err)
	} else {
		resp.Body.Close()
		if api := resp.Header.Get("Api-Version"); newerThanReviewed(api) {
			note = fmt.Sprintf(" (dockerd speaks API %s, reviewed for %s: a new list endpoint would pass unfiltered)", api, reviewedAPI)
		}
	}
	log.Printf("serving %s in front of %s%s", *listen, *upstream, note)

	srv := &http.Server{Handler: f, ReadHeaderTimeout: 30 * time.Second, ErrorLog: log.New(os.Stderr, "", 0)}
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-done
		// Close, not Shutdown: an event stream never ends by itself.
		srv.Close()
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve: %v", err)
	}
	os.Remove(*listen)
}
