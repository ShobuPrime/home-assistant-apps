package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Container list elements as dockerd writes them. The visible ones carry a
// number past 2^53 and characters a JSON round trip would re-escape: they
// must come out byte for byte.
const (
	ctMine       = `{"Id":"aaa1","Names":["/mine"],"ImageID":"sha256:shared","State":"running","SizeRw":9007199254740993,"Labels":{"com.docker.compose.project":"mine","note":"a<b>&c"}}`
	ctPlain      = `{"Id":"aaa2","Names":["/plain"],"ImageID":"sha256:plain","State":"exited","SizeRw":10,"Labels":{}}`
	ctCore       = `{"Id":"bbb1","Names":["/homeassistant"],"ImageID":"sha256:core","State":"running","SizeRw":100,"Labels":{"supervisor_managed":"","io.hass.type":"core"}}`
	ctApp        = `{"Id":"bbb2","Names":["/app_x"],"ImageID":"sha256:thirdparty","State":"exited","SizeRw":1000,"Labels":{"supervisor_managed":""}}`
	ctSupervisor = `{"Id":"bbb3","Names":["/hassio_supervisor"],"ImageID":"sha256:supervisor","State":"running","SizeRw":5,"Labels":{"io.hass.type":"supervisor"}}`
	ctChild      = `{"Id":"bbb4","Names":["/stack-front-1"],"ImageID":"sha256:shared","State":"running","SizeRw":7,"Labels":{"io.github.shobuprime.app-managed":"true","com.docker.compose.project":"stack"}}`
	ctChild2     = `{"Id":"bbb5","Names":["/stack-db-1"],"ImageID":"sha256:childonly","State":"running","SizeRw":3,"Labels":{"io.github.shobuprime.app-managed":"true"}}`
	// Started by hand from a Home Assistant image: not the Supervisor's, so it stays.
	ctByHand = `{"Id":"aaa3","Names":["/tinker"],"ImageID":"sha256:core","State":"running","SizeRw":1,"Labels":{"io.hass.type":"core"}}`

	imShared     = `{"Id":"sha256:shared","Containers":2,"Size":500,"SharedSize":100,"Labels":null}`
	imPlain      = `{"Id":"sha256:plain","Containers":0,"Size":300,"SharedSize":50,"Labels":{"maintainer":"x"}}`
	imCore       = `{"Id":"sha256:core","Containers":2,"Size":9000,"SharedSize":0,"Labels":{"io.hass.type":"core"}}`
	imStoppedApp = `{"Id":"sha256:stoppedapp","Containers":0,"Size":4000,"SharedSize":1000,"Labels":{"io.hass.type":"app"}}`
	imThirdParty = `{"Id":"sha256:thirdparty","Containers":1,"Size":700,"SharedSize":0,"Labels":{}}`
	imChildOnly  = `{"Id":"sha256:childonly","Containers":1,"Size":800,"SharedSize":0,"Labels":null}`
)

var (
	allContainers = []string{ctMine, ctCore, ctPlain, ctApp, ctSupervisor, ctChild, ctChild2, ctByHand}
	allImages     = []string{imShared, imCore, imPlain, imStoppedApp, imThirdParty, imChildOnly}
)

func jsonArray(elements ...string) string { return "[" + strings.Join(elements, ",") + "]" }

// seen is one request as the stand-in dockerd received it.
type seen struct {
	method, path, rawQuery, body string
	contentLength                int64
	transferEncoding             []string
}

// dockerd is a stand-in for the daemon on a unix socket.
type dockerd struct {
	socket string
	mu     sync.Mutex
	reqs   []seen
	// answers by path without its version prefix, for the tests that need
	// another one than the default
	answer map[string]func(http.ResponseWriter, *http.Request)
}

func (d *dockerd) last(t *testing.T) seen {
	t.Helper()
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.reqs) == 0 {
		t.Fatal("dockerd received no request")
	}
	return d.reqs[len(d.reqs)-1]
}

func (d *dockerd) count(path string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, r := range d.reqs {
		if r.path == path {
			n++
		}
	}
	return n
}

func (d *dockerd) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	p := r.URL.Path
	if m := versionPrefix.FindStringSubmatch(p); m != nil {
		p = m[1]
	}
	d.mu.Lock()
	d.reqs = append(d.reqs, seen{r.Method, r.URL.Path, r.URL.RawQuery, string(body), r.ContentLength, r.TransferEncoding})
	custom := d.answer[p]
	d.mu.Unlock()

	w.Header().Set("Api-Version", "1.56")
	if custom != nil {
		custom(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case p == "/_ping":
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "OK")
	case p == "/containers/json":
		fmt.Fprintln(w, jsonArray(allContainers...))
	case p == "/images/json":
		fmt.Fprintln(w, jsonArray(allImages...))
	case p == "/system/df":
		fmt.Fprintln(w, diskUsageAnswer)
	case strings.HasSuffix(p, "/prune"):
		fmt.Fprintln(w, `{"SpaceReclaimed":0}`)
	case lifecycle.MatchString(p):
		w.WriteHeader(http.StatusNoContent)
	case p == "/exec/abc/start":
		hj, _ := w.(http.Hijacker)
		conn, rw, _ := hj.Hijack()
		defer conn.Close()
		rw.WriteString("HTTP/1.1 101 UPGRADED\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
		rw.Flush()
		io.Copy(conn, rw) // echo
	default:
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, `{"message":"page not found"}`)
	}
}

const diskUsageAnswer = `{"LayersSize":123456789012345678,` +
	`"Images":[` + imShared + `,` + imCore + `,` + imPlain + `,` + imStoppedApp + `,` + imThirdParty + `,` + imChildOnly + `],` +
	`"Containers":[` + ctMine + `,` + ctCore + `,` + ctPlain + `,` + ctApp + `,` + ctSupervisor + `,` + ctChild + `,` + ctChild2 + `,` + ctByHand + `],` +
	`"Volumes":[{"Name":"v1","UsageData":{"Size":9007199254740993,"RefCount":1}}],"BuildCache":[],` +
	`"ImageUsage":{"ActiveCount":4,"TotalCount":6,"Reclaimable":3250,"TotalSize":123456789012345678,"Items":[` + imShared + `,` + imCore + `,` + imPlain + `,` + imStoppedApp + `,` + imThirdParty + `,` + imChildOnly + `]},` +
	`"ContainerUsage":{"ActiveCount":6,"TotalCount":8,"Reclaimable":1010,"TotalSize":9007199254741119,"Items":[` + ctMine + `,` + ctCore + `,` + ctPlain + `,` + ctApp + `,` + ctSupervisor + `,` + ctChild + `,` + ctChild2 + `,` + ctByHand + `]},` +
	`"VolumeUsage":{"ActiveCount":1,"TotalCount":1,"Reclaimable":0,"TotalSize":9007199254740993,"Items":null},` +
	`"BuildCacheUsage":{"ActiveCount":0,"TotalCount":0,"Reclaimable":0,"TotalSize":0}}`

// start runs the stand-in dockerd and the filter in front of it, and returns
// a client that reaches dockerd through the filter.
func start(t *testing.T) (*dockerd, *http.Client, string) {
	t.Helper()
	dir := t.TempDir()
	d := &dockerd{socket: filepath.Join(dir, "docker.sock"), answer: map[string]func(http.ResponseWriter, *http.Request){}}
	serve := func(socket string, h http.Handler) {
		ln, err := net.Listen("unix", socket)
		if err != nil {
			t.Fatal(err)
		}
		srv := &http.Server{Handler: h}
		go srv.Serve(ln)
		t.Cleanup(func() { srv.Close() })
	}
	serve(d.socket, d)
	front := filepath.Join(dir, "filter.sock")
	serve(front, newFilter(d.socket))
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", front)
		},
		DisableCompression: true,
	}}
	return d, client, front
}

func get(t *testing.T, c *http.Client, target string) (int, string) {
	t.Helper()
	resp, err := c.Get("http://docker" + target)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(body)
}

func TestRule(t *testing.T) {
	containers := []struct {
		labels map[string]string
		hidden bool
	}{
		{map[string]string{"supervisor_managed": ""}, true},
		{map[string]string{"supervisor_managed": "", "io.hass.type": "app"}, true},
		{map[string]string{"io.hass.type": "supervisor"}, true},
		{map[string]string{"io.github.shobuprime.app-managed": "true"}, true},
		{map[string]string{"io.hass.type": "addon"}, false}, // an image label alone: started by hand
		{map[string]string{"io.github.shobuprime.app-managed": "false"}, false},
		{map[string]string{"com.docker.compose.project": "mine"}, false},
		{nil, false},
	}
	for _, c := range containers {
		if got := hiddenContainer(c.labels); got != c.hidden {
			t.Errorf("hiddenContainer(%v) = %v, want %v", c.labels, got, c.hidden)
		}
	}
	if !hiddenImage(map[string]string{"io.hass.type": "base"}) || hiddenImage(map[string]string{"maintainer": "x"}) || hiddenImage(nil) {
		t.Error("hiddenImage: any io.hass.type hides an image, nothing else does")
	}
}

func TestContainerList(t *testing.T) {
	_, c, _ := start(t)
	// The list is the list however dockerd would route to it.
	for _, target := range []string{"/containers/json?all=1", "/v1.56/containers/json", "/v01.56/containers/json", "/v1.41/containers/%6Ason"} {
		status, body := get(t, c, target)
		want := jsonArray(ctMine, ctPlain, ctByHand) + "\n"
		if status != 200 || body != want {
			t.Errorf("%s:\n got %d %s\nwant 200 %s", target, status, body, want)
		}
	}
}

func TestListUntouchedWhenNothingIsHidden(t *testing.T) {
	d, c, _ := start(t)
	const original = "[ " + ctMine + " ,\n " + ctPlain + " ]\n" // dockerd's own spacing must survive too
	d.answer["/containers/json"] = func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, original) }
	if _, body := get(t, c, "/containers/json"); body != original {
		t.Errorf("got %q, want dockerd's bytes unchanged", body)
	}
}

func TestOtherPathsPassUntouched(t *testing.T) {
	d, c, _ := start(t)
	const inspect = `{"Id":"bbb1","Config":{"Labels":{"supervisor_managed":""}},"SizeRw":9007199254740993}` + "\n"
	d.answer["/containers/bbb1/json"] = func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, inspect) }
	// Hiding is not enforcement: a hidden container still answers by ID.
	if status, body := get(t, c, "/containers/bbb1/json"); status != 200 || body != inspect {
		t.Errorf("inspect by ID: got %d %q", status, body)
	}
	// A path dockerd would redirect is not the list, and is not rewritten.
	get(t, c, "/containers//json")
	if got := d.last(t).path; got != "/containers//json" {
		t.Errorf("dockerd saw path %q, want it as sent", got)
	}
	resp, err := c.Head("http://docker/_ping")
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.Header.Get("Api-Version"); got != "1.56" {
		t.Errorf("Api-Version through the filter = %q: clients choose their version from it", got)
	}
}

func TestImageList(t *testing.T) {
	_, c, _ := start(t)
	// Gone: both labelled images, and the two that only hidden containers
	// run. Kept: the image a hidden and a visible container share.
	want := jsonArray(imShared, imPlain) + "\n"
	if status, body := get(t, c, "/v1.56/images/json"); status != 200 || body != want {
		t.Errorf("got %d %s\nwant 200 %s", status, body, want)
	}
}

func TestDiskUsage(t *testing.T) {
	_, c, _ := start(t)
	status, body := get(t, c, "/v1.56/system/df")
	if status != 200 {
		t.Fatalf("status %d: %s", status, body)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, body)
	}
	want := map[string]string{
		"LayersSize": `123456789012345678`, // shared layers: not recounted
		"Images":     jsonArray(imShared, imPlain),
		"Containers": jsonArray(ctMine, ctPlain, ctByHand),
		"Volumes":    `[{"Name":"v1","UsageData":{"Size":9007199254740993,"RefCount":1}}]`,
		"BuildCache": `[]`,
		// 2 images: one in use; the other reclaims 300-50.
		"ImageUsage": `{"ActiveCount":1,"TotalCount":2,"Reclaimable":250,"TotalSize":123456789012345678,"Items":` + jsonArray(imShared, imPlain) + `}`,
		// 3 containers, 2 running; sizes 9007199254740993 + 10 + 1, of which the stopped one is reclaimable.
		"ContainerUsage":  `{"ActiveCount":2,"TotalCount":3,"Reclaimable":10,"TotalSize":9007199254741004,"Items":` + jsonArray(ctMine, ctPlain, ctByHand) + `}`,
		"VolumeUsage":     `{"ActiveCount":1,"TotalCount":1,"Reclaimable":0,"TotalSize":9007199254740993,"Items":null}`,
		"BuildCacheUsage": `{"ActiveCount":0,"TotalCount":0,"Reclaimable":0,"TotalSize":0}`,
	}
	for key, w := range want {
		if string(got[key]) != w {
			t.Errorf("%s:\n got %s\nwant %s", key, got[key], w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("answer has %d keys, want %d", len(got), len(want))
	}
}

func TestDiskUsageUntouchedWhenNothingIsHidden(t *testing.T) {
	d, c, _ := start(t)
	const original = `{"LayersSize":1, "Images":[` + imPlain + `], "Containers":null, "ContainerUsage":{"TotalCount":0}}` + "\n"
	d.answer["/system/df"] = func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, original) }
	d.answer["/containers/json"] = func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "[]\n") }
	if _, body := get(t, c, "/system/df"); body != original {
		t.Errorf("got %q, want dockerd's bytes unchanged", body)
	}
}

func TestFailClosed(t *testing.T) {
	d, c, _ := start(t)
	cases := map[string]func(http.ResponseWriter, *http.Request){
		"not JSON": func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "<html>") },
		"an element that is no object": func(w http.ResponseWriter, _ *http.Request) {
			io.WriteString(w, `[`+ctCore+`,"x"]`)
		},
		"an encoding it cannot read": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Encoding", "gzip")
			io.WriteString(w, jsonArray(ctCore))
		},
	}
	for name, answer := range cases {
		d.answer["/containers/json"] = answer
		status, body := get(t, c, "/containers/json")
		if status != http.StatusBadGateway || !strings.Contains(body, `"message"`) || strings.Contains(body, "homeassistant") {
			t.Errorf("%s: got %d %q, want a 502 in dockerd's error shape and nothing of the list", name, status, body)
		}
	}
	// dockerd's own errors are not the filter's to judge.
	d.answer["/containers/json"] = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"message":"boom"}`)
	}
	if status, body := get(t, c, "/containers/json"); status != 500 || body != `{"message":"boom"}` {
		t.Errorf("dockerd's 500 came back as %d %q", status, body)
	}
	// The image list depends on a container list of the filter's own.
	d.answer["/containers/json"] = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }
	if status, body := get(t, c, "/images/json"); status != http.StatusBadGateway || strings.Contains(body, "sha256:") {
		t.Errorf("image list without the container list: got %d %q, want a 502", status, body)
	}
}

func TestEvents(t *testing.T) {
	const (
		evMine   = `{"Type":"container","Action":"start","Actor":{"ID":"aaa1","Attributes":{"name":"mine","com.docker.compose.project":"mine"}},"timeNano":1795180000000000001}`
		evCore   = `{"Type":"container","Action":"health_status: healthy","Actor":{"ID":"bbb1","Attributes":{"name":"homeassistant","supervisor_managed":""}}}`
		evChild  = `{"Type":"container","Action":"die","Actor":{"ID":"bbb4","Attributes":{"io.github.shobuprime.app-managed":"true"}}}`
		evExec   = `{"Type":"container","Action":"exec_start: sh","Actor":{"ID":"bbb3","Attributes":{"io.hass.type":"supervisor","execID":"e1"}}}`
		evImage  = `{"Type":"image","Action":"pull","Actor":{"ID":"busybox","Attributes":{"name":"busybox"}}}`
		evNet    = `{"Type":"network","Action":"connect","Actor":{"ID":"n1","Attributes":{"container":"bbb1","name":"hassio"}}}`
		evByHand = `{"Type":"container","Action":"start","Actor":{"ID":"aaa3","Attributes":{"io.hass.type":"core"}}}`
	)
	all := []string{evMine, evCore, evImage, evChild, evExec, evNet, evByHand}
	kept := []string{evMine, evImage, evNet, evByHand}

	for _, framing := range []struct{ name, before string }{{"one record per line", ""}, {"json-seq", "\x1e"}} {
		d, c, _ := start(t)
		release := make(chan struct{})
		d.answer["/events"] = func(w http.ResponseWriter, _ *http.Request) {
			io.WriteString(w, framing.before+evCore+"\n"+framing.before+evMine+"\n")
			w.(http.Flusher).Flush()
			<-release // the stream stays open, as dockerd's does
			for _, ev := range all[2:] {
				io.WriteString(w, framing.before+ev+"\n")
			}
		}
		resp, err := c.Get("http://docker/v1.56/events")
		if err != nil {
			t.Fatal(err)
		}
		lines := bufio.NewReader(resp.Body)
		// The first kept record has to arrive while the stream is still open.
		first := make(chan string, 1)
		go func() { line, _ := lines.ReadString('\n'); first <- line }()
		select {
		case line := <-first:
			if line != framing.before+evMine+"\n" {
				t.Errorf("%s: first record %q, want the visible container's", framing.name, line)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: no record arrived while the stream was open: the filter is buffering", framing.name)
		}
		close(release)
		rest, _ := io.ReadAll(lines)
		resp.Body.Close()
		want := ""
		for _, ev := range kept[1:] {
			want += framing.before + ev + "\n"
		}
		if string(rest) != want {
			t.Errorf("%s:\n got %q\nwant %q", framing.name, rest, want)
		}
	}
}

func TestPrune(t *testing.T) {
	d, c, _ := start(t)
	post := func(target, contentType, body string) (int, string) {
		t.Helper()
		resp, err := c.Post("http://docker"+target, contentType, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	sentFilters := func() map[string]map[string]bool {
		t.Helper()
		got := d.last(t)
		if got.body != "" || got.contentLength != 0 || len(got.transferEncoding) != 0 {
			t.Errorf("dockerd got a body with the prune (%q, length %d, %v): it would read filters from it", got.body, got.contentLength, got.transferEncoding)
		}
		q, _ := url.ParseQuery(got.rawQuery)
		var f map[string]map[string]bool
		if err := json.Unmarshal([]byte(q.Get("filters")), &f); err != nil {
			t.Fatalf("filters dockerd got are not JSON: %q", q.Get("filters"))
		}
		return f
	}
	enc := url.QueryEscape

	// No filters at all: the exclusion is the only one.
	post("/v1.56/containers/prune", "", "")
	if f := sentFilters(); len(f) != 1 || !f["label!"]["supervisor_managed"] {
		t.Errorf("container prune: dockerd got %v, want only label!=supervisor_managed", f)
	}
	// The tool's own filters stay, in the current form...
	post("/images/prune?filters="+enc(`{"dangling":{"false":true},"until":{"24h":true}}`), "", "")
	if f := sentFilters(); !f["label!"]["io.hass.type"] || !f["dangling"]["false"] || !f["until"]["24h"] || len(f) != 3 {
		t.Errorf("image prune: dockerd got %v", f)
	}
	// ...and in the older list form.
	post("/images/prune?filters="+enc(`{"dangling":["true"],"label":["keep=no"]}`), "", "")
	if f := sentFilters(); !f["label!"]["io.hass.type"] || !f["dangling"]["true"] || !f["label"]["keep=no"] {
		t.Errorf("image prune, list form: dockerd got %v", f)
	}
	// dockerd prefers a form body to the query, so the body is read and not sent.
	post("/containers/prune?filters="+enc(`{"until":{"1h":true}}`), "application/x-www-form-urlencoded", "filters="+enc(`{"until":{"99h":true}}`))
	if f := sentFilters(); !f["until"]["99h"] || f["until"]["1h"] || !f["label!"]["supervisor_managed"] {
		t.Errorf("prune with a form body: dockerd got %v, want the body's filters plus the exclusion", f)
	}
	// A second label! would spare less, so such a prune is refused, not sent.
	before := d.count("/images/prune")
	status, body := post("/images/prune?filters="+enc(`{"label!":{"keep":true}}`), "", "")
	if status != http.StatusBadRequest || !strings.Contains(body, `"message"`) || d.count("/images/prune") != before {
		t.Errorf("prune with its own label!: got %d %q, sent on: %v", status, body, d.count("/images/prune") != before)
	}
	status, _ = post("/containers/prune?filters=%7Bnot-json", "", "")
	if status != http.StatusBadRequest {
		t.Errorf("unreadable filters: got %d, want 400", status)
	}
	// The other prunes have nothing of Home Assistant's to spare by label.
	post("/volumes/prune?filters="+enc(`{"all":{"true":true}}`), "", "")
	if got := d.last(t).rawQuery; got != "filters="+enc(`{"all":{"true":true}}`) {
		t.Errorf("volume prune query changed to %q", got)
	}
}

func TestLifecycleVerbsGoOutWithoutBody(t *testing.T) {
	d, c, _ := start(t)
	for _, verb := range []string{"start", "stop", "restart", "kill", "pause", "unpause", "wait"} {
		// A reader of unknown length makes the client send it chunked, as
		// Home Assistant's ingress does.
		req, _ := http.NewRequest(http.MethodPost, "http://docker/v1.44/containers/abc/"+verb+"?t=5", io.NopCloser(strings.NewReader("{}")))
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		got := d.last(t)
		if got.body != "" || got.contentLength != 0 || len(got.transferEncoding) != 0 || got.rawQuery != "t=5" {
			t.Errorf("%s: dockerd got body %q, length %d, %v, query %q", verb, got.body, got.contentLength, got.transferEncoding, got.rawQuery)
		}
	}
	// A body that belongs stays: exec carries its command.
	resp, err := c.Post("http://docker/containers/abc/exec", "application/json", strings.NewReader(`{"Cmd":["ls"]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := d.last(t); got.body != `{"Cmd":["ls"]}` {
		t.Errorf("exec create: dockerd got body %q", got.body)
	}
}

func TestUpgradedConnectionIsTunnelled(t *testing.T) {
	_, _, front := start(t)
	conn, err := net.Dial("unix", front)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	body := `{"Detach":false,"Tty":false}`
	fmt.Fprintf(conn, "POST /v1.56/exec/abc/start HTTP/1.1\r\nHost: docker\r\nConnection: Upgrade\r\nUpgrade: tcp\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
	r := bufio.NewReader(conn)
	resp, err := http.ReadResponse(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status %d, want 101", resp.StatusCode)
	}
	// dockerd's stand-in echoes what arrives after the request.
	for _, typed := range []string{"typed\n", "\x00\x01binary\xff"} {
		io.WriteString(conn, typed)
		got := make([]byte, len(typed))
		if _, err := io.ReadFull(r, got); err != nil || string(got) != typed {
			t.Errorf("through the tunnel: sent %q, got %q, %v", typed, got, err)
		}
	}
}

func TestNewerThanReviewed(t *testing.T) {
	for api, want := range map[string]bool{"1.56": false, "1.40": false, "1.57": true, "2.0": true, "": false} {
		if got := newerThanReviewed(api); got != want {
			t.Errorf("newerThanReviewed(%q) = %v, want %v", api, got, want)
		}
	}
}
