package unifi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type mock struct {
	armProfilesOK     bool // GET /v1/arm-profiles returns 200 (Local mode)
	armProfilesGlobal bool // GET /v1/arm-profiles returns 400 'global' (Global mode)
	enableGlobal      bool // POST /v1/arm-profiles/enable returns 400 'global'
	lastAPIKey        string
}

func newMock(t *testing.T, m *mock) *Client {
	t.Helper()
	mux := http.NewServeMux()
	const p = "/proxy/protect/integration/v1"

	// The live gateway returns the NVR as a single object whose armMode is
	// an object (status/armProfileId), so the mock matches that shape.
	mux.HandleFunc(p+"/nvrs", func(w http.ResponseWriter, r *http.Request) {
		m.lastAPIKey = r.Header.Get("X-API-KEY")
		_ = json.NewEncoder(w).Encode(NVR{
			ID: "nvr1", Name: "UCG Fiber", ModelKey: "nvr",
			ArmMode: &NVRArmMode{Status: "disabled", ArmProfileID: "p-away"},
		})
	})
	mux.HandleFunc(p+"/arm-profiles", func(w http.ResponseWriter, r *http.Request) {
		if m.armProfilesGlobal {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"This NVR is in global alarm manager mode"}`)
			return
		}
		if !m.armProfilesOK {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode([]ArmProfile{{ID: "p-away", Name: "Away"}})
	})
	mux.HandleFunc(p+"/arm-profiles/settings", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc(p+"/arm-profiles/enable", func(w http.ResponseWriter, r *http.Request) {
		if m.enableGlobal {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"This NVR is in global alarm manager mode"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc(p+"/arm-profiles/disable", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc(p+"/sensors", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Sensor{{ID: "s1", Name: "Front Door", MountType: "door", IsOpen: true}})
	})

	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "https://")
	return New(host, "test-key", false, nil)
}

func TestDetectModeLocal(t *testing.T) {
	c := newMock(t, &mock{armProfilesOK: true})
	if m, err := c.DetectMode(t.Context()); err != nil || m != ModeLocal {
		t.Fatalf("got %s err=%v, want local", m, err)
	}
}

func TestDetectModeGlobal(t *testing.T) {
	c := newMock(t, &mock{armProfilesGlobal: true})
	if m, err := c.DetectMode(t.Context()); err != nil || m != ModeGlobal {
		t.Fatalf("got %s err=%v, want global", m, err)
	}
}

func TestDetectModeAppManagedOldFirmware(t *testing.T) {
	c := newMock(t, &mock{armProfilesOK: false})
	if m, err := c.DetectMode(t.Context()); err != nil || m != ModeAppManaged {
		t.Fatalf("got %s err=%v, want app-managed", m, err)
	}
}

func TestArmReturnsGlobalModeError(t *testing.T) {
	c := newMock(t, &mock{enableGlobal: true})
	if err := c.Arm(t.Context(), "p-away"); !errors.Is(err, ErrGlobalMode) {
		t.Fatalf("want ErrGlobalMode, got %v", err)
	}
}

func TestArmDisarmSuccess(t *testing.T) {
	c := newMock(t, &mock{})
	if err := c.Arm(t.Context(), "p-away"); err != nil {
		t.Fatalf("arm: %v", err)
	}
	if err := c.Disarm(t.Context()); err != nil {
		t.Fatalf("disarm: %v", err)
	}
}

func TestAPIKeyHeaderSent(t *testing.T) {
	m := &mock{armProfilesOK: true}
	c := newMock(t, m)
	if _, err := c.GetNVRs(t.Context()); err != nil {
		t.Fatalf("get nvrs: %v", err)
	}
	if m.lastAPIKey != "test-key" {
		t.Fatalf("X-API-KEY = %q, want test-key", m.lastAPIKey)
	}
}

func TestGetSensors(t *testing.T) {
	c := newMock(t, &mock{})
	sensors, err := c.GetSensors(t.Context())
	if err != nil {
		t.Fatalf("sensors: %v", err)
	}
	if len(sensors) != 1 || !sensors[0].IsOpen || sensors[0].Name != "Front Door" {
		t.Fatalf("unexpected sensors: %+v", sensors)
	}
}
