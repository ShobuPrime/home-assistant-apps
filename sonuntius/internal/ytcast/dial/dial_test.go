// Maps to: N/A — Go-only tests for the DIAL port.
//
// Upstream yt-cast-receiver has no DIAL-server-level tests (peer-dial /
// peer-ssdp are exercised end-to-end by hooking up a real Cast sender).
// The tests in this file pin down the wire shape of the SSDP packets and
// HTTP responses so future drift is caught before it reaches the network.
//
// Tests that bind to the multicast group are gated behind t.Skip so CI
// containers without IGMP support do not break the build.
package dial

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shobuprime/sonuntius/internal/ytcast/constants"
)

// TestParseLaunchPayload covers the form-encoded body parser that pulls the
// pairing code out of the DIAL launch POST.
func TestParseLaunchPayload(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantCode    string
		wantSenders map[string]string
	}{
		{
			name:        "empty body",
			body:        "",
			wantCode:    "",
			wantSenders: map[string]string{},
		},
		{
			name:        "just pairing code",
			body:        "pairingCode=ABCD-EFGH",
			wantCode:    "ABCD-EFGH",
			wantSenders: map[string]string{},
		},
		{
			name:        "pairing code plus theme",
			body:        "pairingCode=12345&theme=cl&v=2",
			wantCode:    "12345",
			wantSenders: map[string]string{"theme": "cl", "v": "2"},
		},
		{
			name:        "duplicate field gets joined",
			body:        "extras=a&extras=b",
			wantCode:    "",
			wantSenders: map[string]string{"extras": "a,b"},
		},
		{
			name:        "garbage still preserves raw",
			body:        "%%%not-form-encoded%%%",
			wantCode:    "",
			wantSenders: map[string]string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := parseLaunchPayload([]byte(tc.body))
			if req.PairingCode != tc.wantCode {
				t.Fatalf("PairingCode = %q, want %q", req.PairingCode, tc.wantCode)
			}
			if !equalStringMap(req.Sender, tc.wantSenders) {
				t.Fatalf("Sender = %#v, want %#v", req.Sender, tc.wantSenders)
			}
			if string(req.RawBody) != tc.body {
				t.Fatalf("RawBody = %q, want %q", req.RawBody, tc.body)
			}
		})
	}
}

// TestBuildSearchResponse asserts the wire shape of the SSDP M-SEARCH
// reply. The header set must include CACHE-CONTROL, EXT, LOCATION, SERVER,
// ST, USN — without these, real Cast senders ignore the device.
func TestBuildSearchResponse(t *testing.T) {
	pkt := buildSearchResponse(serviceTypeDial,
		"uuid:abc::"+serviceTypeDial,
		"http://192.0.2.1:3000/ytcr/ssdp/device-desc.xml")
	got := string(pkt)
	want := []string{
		"HTTP/1.1 200 OK\r\n",
		"CACHE-CONTROL: max-age=1800\r\n",
		"EXT: \r\n",
		"LOCATION: http://192.0.2.1:3000/ytcr/ssdp/device-desc.xml\r\n",
		"ST: " + serviceTypeDial + "\r\n",
		"USN: uuid:abc::" + serviceTypeDial + "\r\n",
	}
	for _, line := range want {
		if !strings.Contains(got, line) {
			t.Errorf("missing line %q in response\n--full--\n%s", line, got)
		}
	}
	if !strings.HasSuffix(got, "\r\n\r\n") {
		t.Errorf("response must terminate with CRLFCRLF, got %q", got[len(got)-4:])
	}
}

// TestBuildNotifyAlive checks the ssdp:alive NOTIFY format.
func TestBuildNotifyAlive(t *testing.T) {
	pkt := buildNotify("ssdp:alive", rootDeviceType,
		"uuid:abc::"+rootDeviceType,
		"http://192.0.2.1:3000/ytcr/ssdp/device-desc.xml")
	got := string(pkt)
	for _, line := range []string{
		"NOTIFY * HTTP/1.1\r\n",
		"HOST: 239.255.255.250:1900\r\n",
		"NT: " + rootDeviceType + "\r\n",
		"NTS: ssdp:alive\r\n",
		"USN: uuid:abc::" + rootDeviceType + "\r\n",
		"CACHE-CONTROL: max-age=1800\r\n",
		"LOCATION: http://192.0.2.1:3000/ytcr/ssdp/device-desc.xml\r\n",
	} {
		if !strings.Contains(got, line) {
			t.Errorf("missing line %q in NOTIFY\n--full--\n%s", line, got)
		}
	}
}

// TestBuildNotifyByebye checks that the byebye variant omits LOCATION /
// CACHE-CONTROL — required by the UPnP spec.
func TestBuildNotifyByebye(t *testing.T) {
	pkt := buildNotify("ssdp:byebye", serviceTypeDial,
		"uuid:abc::"+serviceTypeDial, "")
	got := string(pkt)
	if strings.Contains(got, "LOCATION:") {
		t.Errorf("byebye must not include LOCATION header, got:\n%s", got)
	}
	if strings.Contains(got, "CACHE-CONTROL:") {
		t.Errorf("byebye must not include CACHE-CONTROL header, got:\n%s", got)
	}
	for _, line := range []string{
		"NOTIFY * HTTP/1.1\r\n",
		"NTS: ssdp:byebye\r\n",
		"NT: " + serviceTypeDial + "\r\n",
	} {
		if !strings.Contains(got, line) {
			t.Errorf("missing line %q in byebye\n--full--\n%s", line, got)
		}
	}
}

// TestParseSSDPRequest sanity-checks the textproto-backed parser.
func TestParseSSDPRequest(t *testing.T) {
	pkt := []byte("M-SEARCH * HTTP/1.1\r\n" +
		"HOST: 239.255.255.250:1900\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 2\r\n" +
		"ST: urn:dial-multiscreen-org:service:dial:1\r\n\r\n")
	method, hdr, ok := parseSSDPRequest(pkt)
	if !ok {
		t.Fatal("parseSSDPRequest returned ok=false on a valid M-SEARCH")
	}
	if method != "M-SEARCH" {
		t.Errorf("method = %q, want M-SEARCH", method)
	}
	if got := strings.Trim(hdr.Get("Man"), "\""); got != "ssdp:discover" {
		t.Errorf("Man = %q, want ssdp:discover", got)
	}
	if got := hdr.Get("St"); got != serviceTypeDial {
		t.Errorf("St = %q, want %q", got, serviceTypeDial)
	}
}

// TestRenderDeviceDesc verifies that the device descriptor contains every
// element peer-dial's EJS template emits.
func TestRenderDeviceDesc(t *testing.T) {
	body, err := renderDeviceDesc(deviceDescParams{
		URLBase:      "http://192.0.2.1:3000/ytcr",
		FriendlyName: "Sonuntius (YouTube)",
		Manufacturer: "ShobuPrime",
		ModelName:    "Sonuntius",
		UUID:         "uuid-1234",
	})
	if err != nil {
		t.Fatalf("renderDeviceDesc: %v", err)
	}
	if !bytes.HasPrefix(body, []byte(xmlHeader)) {
		t.Errorf("device-desc must start with XML prologue, got: %s", body[:64])
	}
	// Round-trip through encoding/xml to make sure the doc is well-formed.
	var got deviceDescDocument
	if err := xml.Unmarshal(body, &got); err != nil {
		t.Fatalf("device-desc not well-formed: %v", err)
	}
	if got.Device.DeviceType != deviceTypeDial {
		t.Errorf("DeviceType = %q, want %q", got.Device.DeviceType, deviceTypeDial)
	}
	if got.Device.UDN != "uuid:uuid-1234" {
		t.Errorf("UDN = %q, want uuid:uuid-1234", got.Device.UDN)
	}
	if got.Device.ServiceList.Service.ServiceType != serviceTypeDial {
		t.Errorf("ServiceType = %q, want %q",
			got.Device.ServiceList.Service.ServiceType, serviceTypeDial)
	}
}

// TestRenderAppDesc verifies the app descriptor matches peer-dial's EJS
// template — namespace, dialVer="1.7", state, allowStop attribute.
func TestRenderAppDesc(t *testing.T) {
	body, err := renderAppDesc(appDescParams{
		Name:      "YouTube",
		AllowStop: false,
		State:     "running",
	})
	if err != nil {
		t.Fatalf("renderAppDesc: %v", err)
	}
	got := string(body)
	for _, want := range []string{
		`xmlns="urn:dial-multiscreen-org:schemas:dial"`,
		`dialVer="1.7"`,
		`<name>YouTube</name>`,
		`allowStop="false"`,
		`<state>running</state>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in app desc:\n%s", want, got)
		}
	}
}

// TestHTTPRoutes spins up the HTTP-side routes via httptest (no SSDP) and
// exercises GET /apps, GET /apps/YouTube, POST /apps/YouTube, DELETE.
func TestHTTPRoutes(t *testing.T) {
	srv := NewServer(Options{
		Port:         3000, // ignored by httptest
		FriendlyName: "Sonuntius (YouTube)",
		UUID:         "uuid-test",
	})

	var launched LaunchRequest
	srv.OnLaunch(func(_ context.Context, req LaunchRequest) error {
		launched = req
		return nil
	})

	mux := srv.newMux()
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// GET /ytcr/apps → 204
	resp, err := http.Get(ts.URL + "/ytcr/apps")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("/apps status = %d, want 204", resp.StatusCode)
	}
	resp.Body.Close()

	// GET /ytcr/apps/YouTube → 200 + XML
	resp, err = http.Get(ts.URL + "/ytcr/apps/YouTube")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /apps/YouTube status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "<state>stopped</state>") {
		t.Errorf("expected stopped state in initial app desc, got:\n%s", body)
	}

	// POST /ytcr/apps/YouTube with pairing code
	resp, err = http.Post(ts.URL+"/ytcr/apps/YouTube",
		"application/x-www-form-urlencoded",
		strings.NewReader("pairingCode=XYZ&theme=cl"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("POST /apps/YouTube (initial) status = %d, want 201", resp.StatusCode)
	}
	resp.Body.Close()
	if launched.PairingCode != "XYZ" {
		t.Errorf("OnLaunch saw PairingCode = %q, want XYZ", launched.PairingCode)
	}
	if launched.Sender["theme"] != "cl" {
		t.Errorf("OnLaunch Sender[theme] = %q, want cl", launched.Sender["theme"])
	}

	// SetState(running) so the second POST returns 200 not 201.
	srv.SetState(AppStateRunning)
	resp, err = http.Post(ts.URL+"/ytcr/apps/YouTube",
		"application/x-www-form-urlencoded",
		strings.NewReader("pairingCode=XYZ"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("POST /apps/YouTube (running) status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// DELETE without AllowStop → 405
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/ytcr/apps/YouTube/run", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("DELETE without AllowStop status = %d, want 405", resp.StatusCode)
	}
	resp.Body.Close()

	// Unknown app → 404
	resp, err = http.Get(ts.URL + "/ytcr/apps/Spotify")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown app status = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestHTTPDeviceDesc serves the device-desc.xml endpoint and asserts the
// Application-URL header peer-dial advertises.
func TestHTTPDeviceDesc(t *testing.T) {
	srv := NewServer(Options{
		FriendlyName: "Sonuntius",
		UUID:         "uuid-test",
	})
	mux := srv.newMux()
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ytcr/ssdp/device-desc.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("device-desc status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Application-URL"); !strings.HasSuffix(got, "/ytcr/apps/") {
		t.Errorf("Application-URL = %q, want suffix /ytcr/apps/", got)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "xml") {
		t.Errorf("Content-Type = %q, want xml", ct)
	}
}

// TestStopWithoutStart is a smoke check: calling Stop on a never-started
// server should be a no-op (matches upstream's "stop called but not RUNNING"
// guard).
func TestStopWithoutStart(t *testing.T) {
	srv := NewServer(Options{UUID: "x"})
	if err := srv.Stop(); err != nil {
		t.Fatalf("Stop on stopped server: %v", err)
	}
	if srv.Status() != constants.StatusStopped {
		t.Errorf("Status after no-op Stop = %v, want stopped", srv.Status())
	}
}

// TestServerLifecycle exercises Start → Stop end-to-end. Gated behind a
// multicast capability check because container CI environments often lack
// IGMP support.
func TestServerLifecycle(t *testing.T) {
	t.Skip("requires multicast — run manually with `go test -run TestServerLifecycle -count=1`")

	srv := NewServer(Options{
		Port:            0, // 0 → kernel picks a free port; we don't care here
		FriendlyName:    "Sonuntius (YouTube)",
		UUID:            "uuid-test",
		AdvertisePeriod: 100 * time.Millisecond,
	})
	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if srv.Status() != constants.StatusRunning {
		t.Errorf("Status after Start = %v, want running", srv.Status())
	}
	time.Sleep(300 * time.Millisecond)
	if err := srv.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if srv.Status() != constants.StatusStopped {
		t.Errorf("Status after Stop = %v, want stopped", srv.Status())
	}
}

func equalStringMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
