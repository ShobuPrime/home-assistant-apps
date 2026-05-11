// Maps to: src/lib/dial/DialServer.ts (HTTP / UPnP description portion via @patrickkfkan/peer-dial → express)
//
// peer-dial registers four routes on its embedded Express app:
//
//   GET    <prefix>/ssdp/device-desc.xml  — UPnP root device descriptor
//   GET    <prefix>/apps                  — 204 No Content
//   GET    <prefix>/apps/:appName         — DIAL app descriptor (XML)
//   POST   <prefix>/apps/:appName         — launch the app
//   DELETE <prefix>/apps/:appName/:pid    — stop the app
//
// This file ports those routes onto net/http and renders the XML using
// encoding/xml. The XML structure is byte-equivalent to peer-dial's EJS
// templates (xml/device-desc.xml + xml/app-desc.xml) — only the rendering
// engine differs.
package dial

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// newMux registers the DIAL HTTP routes against a fresh ServeMux. The
// device-desc handler builds its own base URL from the incoming request's
// Host header so callers reaching us via different interfaces or names see
// matching URLBase / Application-URL values.
func (s *Server) newMux() *http.ServeMux {
	mux := http.NewServeMux()

	// peer-dial's `/ssdp/device-desc.xml`. The path is unique enough that
	// we register it as an exact match (HandleFunc with no trailing slash).
	mux.HandleFunc(s.opts.Prefix+"/ssdp/device-desc.xml", s.handleDeviceDesc)

	// peer-dial's `/apps` — bare GET returns 204.
	mux.HandleFunc(s.opts.Prefix+"/apps", s.handleAppsRoot)

	// `/apps/...` covers GET /apps/:appName, POST /apps/:appName, and
	// DELETE /apps/:appName/:pid. We dispatch by method + path-tail.
	mux.HandleFunc(s.opts.Prefix+"/apps/", s.handleAppsSub)

	return mux
}

// handleAppsRoot maps to `app.get(pref+"/apps", ...)` upstream — sends 204.
func (s *Server) handleAppsRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAppsSub fans out the GET / POST / DELETE handlers for the app
// subtree. Path forms accepted:
//
//	GET    <prefix>/apps/<appName>
//	POST   <prefix>/apps/<appName>
//	DELETE <prefix>/apps/<appName>/<pid>
//	DELETE <prefix>/apps/<appName>/run        ← DIAL 1.7 spec form (we treat
//	                                            "run" as a synonym for the
//	                                            current pid).
func (s *Server) handleAppsSub(w http.ResponseWriter, r *http.Request) {
	tail := strings.TrimPrefix(r.URL.Path, s.opts.Prefix+"/apps/")
	parts := strings.Split(strings.Trim(tail, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	appName := parts[0]
	if appName != s.opts.AppName {
		http.NotFound(w, r)
		return
	}

	// peer-dial routes POST /apps/:appName/dial_data → 501 Not Implemented.
	if len(parts) == 2 && parts[1] == "dial_data" && r.Method == http.MethodPost {
		http.Error(w, "dial_data not implemented", http.StatusNotImplemented)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if len(parts) != 1 {
			http.NotFound(w, r)
			return
		}
		s.handleAppGet(w, r)
	case http.MethodPost:
		if len(parts) != 1 {
			http.NotFound(w, r)
			return
		}
		s.handleAppLaunch(w, r)
	case http.MethodDelete:
		if len(parts) != 2 {
			http.Error(w, "missing pid", http.StatusBadRequest)
			return
		}
		s.handleAppStop(w, r, parts[1])
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleDeviceDesc serves the UPnP device descriptor. Maps to the upstream
// `app.get(pref+"/ssdp/device-desc.xml", ...)` handler.
func (s *Server) handleDeviceDesc(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s%s", scheme, host, s.opts.Prefix)
	body, err := renderDeviceDesc(deviceDescParams{
		URLBase:      baseURL,
		FriendlyName: s.opts.FriendlyName,
		Manufacturer: s.opts.Manufacturer,
		ModelName:    s.opts.ModelName,
		UUID:         s.opts.UUID,
	})
	if err != nil {
		s.log.Error("[dial] render device desc:", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("Application-URL", baseURL+"/apps/")
	_, _ = w.Write(body)
}

// handleAppGet returns the DIAL app descriptor. Maps to `app.get(pref+"/apps/:appName", ...)`.
func (s *Server) handleAppGet(w http.ResponseWriter, _ *http.Request) {
	body, err := renderAppDesc(appDescParams{
		Name:      s.opts.AppName,
		AllowStop: s.opts.AllowStop,
		State:     string(s.State()),
	})
	if err != nil {
		s.log.Error("[dial] render app desc:", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write(body)
}

// handleAppLaunch invokes the host's launch callback. Status codes follow
// peer-dial: 201 if the app was previously stopped, 200 if already running,
// 503 if the callback errored, 413 if body is too large, 404 if app unknown.
func (s *Server) handleAppLaunch(w http.ResponseWriter, r *http.Request) {
	prevState := s.State()

	// Cap body size first so a malicious sender cannot OOM the addon.
	limited := io.LimitReader(r.Body, s.opts.MaxBodyBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	if int64(len(body)) > s.opts.MaxBodyBytes {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}

	req := parseLaunchPayload(body)
	req.RemoteAddr = r.RemoteAddr

	if h := s.launchHandler(); h != nil {
		ctx := r.Context()
		if err := h(ctx, req); err != nil {
			s.log.Error("[dial] launch handler error:", err)
			http.Error(w, "launch failed", http.StatusServiceUnavailable)
			return
		}
	} else {
		s.log.Warn("[dial] launch received but no handler registered")
	}

	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	pid := s.PID()
	if pid == "" {
		// peer-dial omits the pid segment when the delegate does not
		// supply one, but the LOCATION header is still expected. Use
		// "run" — DIAL 1.7 §6.1.4 lists it as the canonical resource.
		pid = "run"
	}
	location := fmt.Sprintf("%s://%s%s/apps/%s/%s",
		scheme, host, s.opts.Prefix, s.opts.AppName, pid)
	w.Header().Set("LOCATION", location)
	if prevState == AppStateStopped {
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

// handleAppStop invokes the host's stop callback when AllowStop is true.
// Mirrors peer-dial's DELETE handler — 405 if stop is disallowed, 200 on
// success, 400 if the callback reports the app could not be stopped.
func (s *Server) handleAppStop(w http.ResponseWriter, r *http.Request, _ string) {
	if !s.opts.AllowStop {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "stop not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h := s.stopHandler(); h != nil {
		if err := h(r.Context()); err != nil {
			s.log.Error("[dial] stop handler error:", err)
			http.Error(w, "stop failed", http.StatusBadRequest)
			return
		}
	}
	s.SetState(AppStateStopped)
	w.WriteHeader(http.StatusOK)
}

// ------------------------------------------------------------------
// XML rendering — equivalent to peer-dial's xml/device-desc.xml and
// xml/app-desc.xml EJS templates.
// ------------------------------------------------------------------

// xmlHeader is the verbatim prologue both templates emit.
const xmlHeader = `<?xml version="1.0" encoding="UTF-8"?>` + "\n"

// deviceDescParams collects every variable peer-dial's device-desc.xml EJS
// template references.
type deviceDescParams struct {
	URLBase      string
	FriendlyName string
	Manufacturer string
	ModelName    string
	UUID         string
}

// deviceDescDocument mirrors the device-desc.xml structure 1:1 — see the
// upstream template at https://github.com/patrickkfkan/peer-dial/blob/master/xml/device-desc.xml
type deviceDescDocument struct {
	XMLName     xml.Name           `xml:"root"`
	XMLNS       string             `xml:"xmlns,attr"`
	SpecVersion deviceSpecVersion  `xml:"specVersion"`
	URLBase     string             `xml:"URLBase"`
	Device      deviceDescDeviceEl `xml:"device"`
}

type deviceSpecVersion struct {
	Major int `xml:"major"`
	Minor int `xml:"minor"`
}

type deviceDescDeviceEl struct {
	DeviceType   string                  `xml:"deviceType"`
	FriendlyName string                  `xml:"friendlyName"`
	Manufacturer string                  `xml:"manufacturer"`
	ModelName    string                  `xml:"modelName"`
	UDN          string                  `xml:"UDN"`
	IconList     deviceDescIconList      `xml:"iconList"`
	ServiceList  deviceDescServiceListEl `xml:"serviceList"`
}

type deviceDescIconList struct {
	Icon deviceDescIcon `xml:"icon"`
}

type deviceDescIcon struct {
	Mimetype string `xml:"mimetype"`
	Width    int    `xml:"width"`
	Height   int    `xml:"height"`
	Depth    int    `xml:"depth"`
	URL      string `xml:"url"`
}

type deviceDescServiceListEl struct {
	Service deviceDescService `xml:"service"`
}

type deviceDescService struct {
	ServiceType string `xml:"serviceType"`
	ServiceID   string `xml:"serviceId"`
	ControlURL  string `xml:"controlURL"`
	EventSubURL string `xml:"eventSubURL"`
	SCPDURL     string `xml:"SCPDURL"`
}

// renderDeviceDesc returns the marshalled device-desc.xml prefixed with the
// XML prologue. The device type and service type values are byte-equivalent
// to peer-dial's template.
func renderDeviceDesc(p deviceDescParams) ([]byte, error) {
	doc := deviceDescDocument{
		XMLNS:       "urn:schemas-upnp-org:device-1-0",
		SpecVersion: deviceSpecVersion{Major: 1, Minor: 0},
		URLBase:     p.URLBase,
		Device: deviceDescDeviceEl{
			DeviceType:   deviceTypeDial,
			FriendlyName: p.FriendlyName,
			Manufacturer: p.Manufacturer,
			ModelName:    p.ModelName,
			UDN:          "uuid:" + p.UUID,
			IconList: deviceDescIconList{
				Icon: deviceDescIcon{
					Mimetype: "image/png",
					Width:    144,
					Height:   144,
					Depth:    32,
					URL:      "/img/icon.png",
				},
			},
			ServiceList: deviceDescServiceListEl{
				Service: deviceDescService{
					ServiceType: serviceTypeDial,
					ServiceID:   "urn:dial-multiscreen-org:serviceId:dial",
					ControlURL:  "/ssdp/notfound",
					EventSubURL: "/ssdp/notfound",
					SCPDURL:     "/ssdp/notfound",
				},
			},
		},
	}
	body, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xmlHeader), body...), nil
}

// appDescParams collects the variables peer-dial's app-desc.xml template
// references. We only emit the fields the YouTube DIAL app uses; the
// optional `<link>` and `<additionalData>` blocks are not surfaced because
// the upstream YouTubeApp does not populate them.
type appDescParams struct {
	Name      string
	AllowStop bool
	State     string
}

// appDescDocument mirrors xml/app-desc.xml. We hand-marshal the
// `dialVer="1.7"` attribute via xml struct tags.
type appDescDocument struct {
	XMLName   xml.Name      `xml:"service"`
	XMLNS     string        `xml:"xmlns,attr"`
	DialVer   string        `xml:"dialVer,attr"`
	Name      string        `xml:"name"`
	Options   appDescOption `xml:"options"`
	State     string        `xml:"state"`
}

type appDescOption struct {
	AllowStop string `xml:"allowStop,attr"`
}

// renderAppDesc marshals the DIAL app descriptor.
func renderAppDesc(p appDescParams) ([]byte, error) {
	allow := "false"
	if p.AllowStop {
		allow = "true"
	}
	doc := appDescDocument{
		XMLNS:   "urn:dial-multiscreen-org:schemas:dial",
		DialVer: "1.7",
		Name:    p.Name,
		Options: appDescOption{AllowStop: allow},
		State:   p.State,
	}
	body, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xmlHeader), body...), nil
}
