// Maps to: src/lib/dial/DialServer.ts (SSDP portion via @patrickkfkan/peer-dial → peer-ssdp)
//
// peer-dial's discovery layer is itself a thin wrapper over peer-ssdp. This
// file ports both: it owns the UDP multicast socket on 239.255.255.250:1900,
// parses M-SEARCH datagrams, sends targeted SSDP responses, and ticks
// NOTIFY ssdp:alive / ssdp:byebye announcements at AdvertisePeriod.
//
// SSDP message formats follow UPnP 1.1 (and the DIAL 1.7 device/service
// types used by peer-dial). Lines are CRLF terminated; an empty line ends
// the headers — same wire shape as HTTP/1.1 minus a body.
package dial

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/shobuprime/sonuntius/internal/ytcast/logger"
)

// SSDP wire constants. Kept unexported so callers cannot accidentally
// reach into them; tests in dial_test.go construct equivalents inline.
const (
	ssdpMulticastAddrV4 = "239.255.255.250:1900"

	serviceTypeDial = "urn:dial-multiscreen-org:service:dial:1"
	deviceTypeDial  = "urn:dial-multiscreen-org:device:dial:1"
	rootDeviceType  = "upnp:rootdevice"

	// cacheMaxAgeSeconds is what we put in CACHE-CONTROL: max-age=...
	// 1800 is the value peer-ssdp emits and the UPnP spec recommends.
	cacheMaxAgeSeconds = 1800
)

// serverProductToken builds the SERVER header value following the UPnP 1.1
// convention "OS/version UPnP/1.1 product/version". We expose the runtime
// OS name so receivers behind a NAT can be identified in packet captures.
func serverProductToken() string {
	return fmt.Sprintf("%s/1 UPnP/1.1 sonuntius/0.1", runtime.GOOS)
}

// ssdpConfig is the subset of Server.Options the responder needs.
type ssdpConfig struct {
	UUID            string
	Location        string
	AdvertiseIP     net.IP
	AdvertisePeriod time.Duration
	Logger          logger.Logger
}

// ssdpResponder owns the multicast listener and the periodic NOTIFY
// broadcaster. Construct with newSSDPResponder, then call Start (which
// returns once the socket is listening) and finally Stop on shutdown.
type ssdpResponder struct {
	cfg ssdpConfig
	log logger.Logger

	// conn is bound to the multicast group (joined via JoinGroup) for
	// receiving M-SEARCH requests. It is also used for sending unicast
	// SSDP responses back to the originating sender.
	conn *net.UDPConn

	// targets is the precomputed list of (NT, USN) pairs we advertise.
	targets []ssdpTarget

	cancel context.CancelFunc
	wg     sync.WaitGroup

	stopOnce sync.Once
}

// ssdpTarget is one (NT, USN) advertisement pair. peer-dial advertises the
// rootdevice + the DIAL service + the DIAL device + the bare uuid; we mirror
// that set so any sender that filters on a specific ST gets a hit.
type ssdpTarget struct {
	// NT is the NT header value (used in NOTIFY) and the ST header value
	// echoed back in M-SEARCH responses.
	NT string
	// USN is the USN header value. Conventionally `uuid:<UUID>::<NT>`
	// except for the uuid-only target, where USN equals "uuid:<UUID>".
	USN string
}

// newSSDPResponder validates the config and computes the advertisement
// target set. Network sockets are not opened until Start.
func newSSDPResponder(cfg ssdpConfig) (*ssdpResponder, error) {
	if cfg.UUID == "" {
		return nil, errors.New("ssdp: UUID is required")
	}
	if cfg.Location == "" {
		return nil, errors.New("ssdp: Location is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = logger.NewDefaultLogger(false)
	}
	if cfg.AdvertisePeriod <= 0 {
		cfg.AdvertisePeriod = DefaultAdvertisePeriod
	}

	r := &ssdpResponder{
		cfg: cfg,
		log: cfg.Logger,
		targets: []ssdpTarget{
			{NT: rootDeviceType, USN: fmt.Sprintf("uuid:%s::%s", cfg.UUID, rootDeviceType)},
			{NT: "uuid:" + cfg.UUID, USN: "uuid:" + cfg.UUID},
			{NT: deviceTypeDial, USN: fmt.Sprintf("uuid:%s::%s", cfg.UUID, deviceTypeDial)},
			{NT: serviceTypeDial, USN: fmt.Sprintf("uuid:%s::%s", cfg.UUID, serviceTypeDial)},
		},
	}
	return r, nil
}

// Start opens the multicast socket and launches the read + advertisement
// goroutines. It returns once the socket is listening.
func (r *ssdpResponder) Start(parent context.Context) error {
	addr, err := net.ResolveUDPAddr("udp4", ssdpMulticastAddrV4)
	if err != nil {
		return fmt.Errorf("ssdp: resolve multicast addr: %w", err)
	}
	// Passing nil for the interface lets the kernel pick the system default
	// multicast interface — which is the right answer when the addon runs
	// on `host_network: true` on a single-NIC HA Yellow. Multi-NIC users
	// can pin the interface via Options.BindInterface (consumed at
	// resolveAdvertiseIP time).
	conn, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		return fmt.Errorf("ssdp: listen multicast %s: %w", ssdpMulticastAddrV4, err)
	}
	// Bump the read buffer so bursty M-SEARCH storms (Cast app launches in
	// noisy networks) don't drop datagrams before we can drain them.
	_ = conn.SetReadBuffer(1 << 20)
	r.conn = conn

	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel

	r.wg.Add(2)
	go r.readLoop(ctx)
	go r.advertiseLoop(ctx)

	// Send an initial alive burst so devices already listening discover us
	// without waiting AdvertisePeriod.
	r.sendAlive()

	return nil
}

// Stop sends ssdp:byebye for every advertisement target and closes the
// socket. Safe to call multiple times.
func (r *ssdpResponder) Stop() error {
	var err error
	r.stopOnce.Do(func() {
		r.sendByebye()
		if r.cancel != nil {
			r.cancel()
		}
		if r.conn != nil {
			err = r.conn.Close()
		}
		r.wg.Wait()
	})
	return err
}

// readLoop drains the multicast socket; for every M-SEARCH whose ST matches
// one of our targets we reply unicast to the originator.
//
// Maps to peer-ssdp's `searchResponse` / `respondToSearch` internals.
func (r *ssdpResponder) readLoop(ctx context.Context) {
	defer r.wg.Done()

	buf := make([]byte, 2048)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		// Set a read deadline so we periodically wake up and re-check ctx
		// — net.UDPConn does not honour context cancellation directly.
		_ = r.conn.SetReadDeadline(time.Now().Add(time.Second))
		n, src, err := r.conn.ReadFromUDP(buf)
		if err != nil {
			var nerr net.Error
			if errors.As(err, &nerr) && nerr.Timeout() {
				continue
			}
			// Closed during shutdown — exit silently.
			if errors.Is(err, net.ErrClosed) {
				return
			}
			r.log.Debug("[dial/ssdp] read error:", err)
			continue
		}
		r.handleDatagram(buf[:n], src)
	}
}

// handleDatagram parses one datagram and, if it is an M-SEARCH for a ST we
// advertise, replies unicast to `src`.
func (r *ssdpResponder) handleDatagram(pkt []byte, src *net.UDPAddr) {
	method, headers, ok := parseSSDPRequest(pkt)
	if !ok {
		return
	}
	if !strings.EqualFold(method, "M-SEARCH") {
		// We don't act on NOTIFY traffic — we are a server, not a control
		// point.
		return
	}
	man := strings.Trim(strings.TrimSpace(headers.Get("Man")), "\"")
	if man != "" && !strings.EqualFold(man, "ssdp:discover") {
		// Spec requires MAN: "ssdp:discover" for M-SEARCH. Some buggy
		// senders omit it entirely; tolerate that, but reject obviously
		// wrong values.
		return
	}
	st := strings.TrimSpace(headers.Get("St"))
	if st == "" {
		return
	}

	matches := r.targetsMatching(st)
	if len(matches) == 0 {
		return
	}
	for _, t := range matches {
		// For ssdp:all we echo each target's NT in the ST header so the
		// sender can correlate. For specific ST searches we echo the
		// requested ST verbatim — that is what UPnP DA1.1 §1.3.3 mandates.
		respondST := st
		if strings.EqualFold(st, "ssdp:all") {
			respondST = t.NT
		}
		r.sendResponse(src, respondST, t.USN)
	}
}

// targetsMatching returns the subset of advertisement targets that should
// reply to an M-SEARCH with the given ST.
func (r *ssdpResponder) targetsMatching(st string) []ssdpTarget {
	switch {
	case strings.EqualFold(st, "ssdp:all"):
		out := make([]ssdpTarget, len(r.targets))
		copy(out, r.targets)
		return out
	default:
		for _, t := range r.targets {
			if strings.EqualFold(t.NT, st) {
				return []ssdpTarget{t}
			}
		}
		return nil
	}
}

// sendResponse writes one HTTP/1.1 200 OK SSDP search response to `dst`.
// Maps to peer-dial's `peer.reply({...})` inside the search handler.
func (r *ssdpResponder) sendResponse(dst *net.UDPAddr, st, usn string) {
	pkt := buildSearchResponse(st, usn, r.cfg.Location)
	if _, err := r.conn.WriteToUDP(pkt, dst); err != nil {
		r.log.Debug("[dial/ssdp] write response failed:", err)
	}
}

// advertiseLoop ticks NOTIFY ssdp:alive at AdvertisePeriod intervals.
// Maps to peer-dial's periodic `peer.alive(...)` inside peer-ssdp.
func (r *ssdpResponder) advertiseLoop(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.cfg.AdvertisePeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.sendAlive()
		}
	}
}

// sendAlive multicasts NOTIFY ssdp:alive once per advertisement target.
func (r *ssdpResponder) sendAlive() {
	dst, err := net.ResolveUDPAddr("udp4", ssdpMulticastAddrV4)
	if err != nil {
		r.log.Debug("[dial/ssdp] resolve multicast for alive:", err)
		return
	}
	for _, t := range r.targets {
		pkt := buildNotify("ssdp:alive", t.NT, t.USN, r.cfg.Location)
		if _, err := r.conn.WriteToUDP(pkt, dst); err != nil {
			r.log.Debug("[dial/ssdp] write alive failed:", err)
			return
		}
	}
}

// sendByebye multicasts NOTIFY ssdp:byebye once per advertisement target so
// senders can prune our entry from their device caches.
func (r *ssdpResponder) sendByebye() {
	if r.conn == nil {
		return
	}
	dst, err := net.ResolveUDPAddr("udp4", ssdpMulticastAddrV4)
	if err != nil {
		return
	}
	for _, t := range r.targets {
		pkt := buildNotify("ssdp:byebye", t.NT, t.USN, "")
		_, _ = r.conn.WriteToUDP(pkt, dst)
	}
}

// buildNotify renders a NOTIFY datagram. `nts` is "ssdp:alive" or
// "ssdp:byebye". For byebye, LOCATION / CACHE-CONTROL are omitted per the
// UPnP spec — only NT / NTS / USN / HOST are required.
func buildNotify(nts, nt, usn, location string) []byte {
	var b bytes.Buffer
	b.Grow(256)
	b.WriteString("NOTIFY * HTTP/1.1\r\n")
	fmt.Fprintf(&b, "HOST: %s\r\n", ssdpMulticastAddrV4)
	fmt.Fprintf(&b, "NT: %s\r\n", nt)
	fmt.Fprintf(&b, "NTS: %s\r\n", nts)
	fmt.Fprintf(&b, "USN: %s\r\n", usn)
	if nts == "ssdp:alive" {
		fmt.Fprintf(&b, "CACHE-CONTROL: max-age=%d\r\n", cacheMaxAgeSeconds)
		fmt.Fprintf(&b, "LOCATION: %s\r\n", location)
		fmt.Fprintf(&b, "SERVER: %s\r\n", serverProductToken())
		fmt.Fprintf(&b, "BOOTID.UPNP.ORG: %d\r\n", 1)
		fmt.Fprintf(&b, "CONFIGID.UPNP.ORG: %d\r\n", 1)
	}
	b.WriteString("\r\n")
	return b.Bytes()
}

// buildSearchResponse renders the unicast SSDP search response. Headers
// match peer-dial's `peer.reply` plus the standard CACHE-CONTROL / EXT /
// DATE that peer-ssdp adds.
func buildSearchResponse(st, usn, location string) []byte {
	var b bytes.Buffer
	b.Grow(256)
	b.WriteString("HTTP/1.1 200 OK\r\n")
	fmt.Fprintf(&b, "CACHE-CONTROL: max-age=%d\r\n", cacheMaxAgeSeconds)
	fmt.Fprintf(&b, "DATE: %s\r\n", time.Now().UTC().Format(time.RFC1123))
	b.WriteString("EXT: \r\n")
	fmt.Fprintf(&b, "LOCATION: %s\r\n", location)
	fmt.Fprintf(&b, "SERVER: %s\r\n", serverProductToken())
	fmt.Fprintf(&b, "ST: %s\r\n", st)
	fmt.Fprintf(&b, "USN: %s\r\n", usn)
	fmt.Fprintf(&b, "BOOTID.UPNP.ORG: %d\r\n", 1)
	fmt.Fprintf(&b, "CONFIGID.UPNP.ORG: %d\r\n", 1)
	b.WriteString("\r\n")
	return b.Bytes()
}

// parseSSDPRequest splits a UDP datagram into method + headers using
// net/textproto. Returns ok=false on malformed input.
func parseSSDPRequest(pkt []byte) (method string, headers textproto.MIMEHeader, ok bool) {
	tp := textproto.NewReader(bufio.NewReader(bytes.NewReader(pkt)))
	line, err := tp.ReadLine()
	if err != nil {
		return "", nil, false
	}
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 1 {
		return "", nil, false
	}
	hdr, err := tp.ReadMIMEHeader()
	if err != nil {
		// textproto requires a trailing CRLF before EOF; SSDP packets
		// satisfy that. If we end up here the datagram is malformed.
		return "", nil, false
	}
	return parts[0], hdr, true
}

// resolveAdvertiseIP picks the IPv4 address SSDP responses should embed in
// LOCATION. If `iface` is set, use the first IPv4 unicast address on that
// interface; otherwise dial the SSDP multicast address (no packet leaves
// the host) and grab the kernel's chosen outbound IP.
//
// This deliberately uses a connected UDP socket as a portable cross-platform
// "what would I send from?" oracle — see net.Dial("udp", ...) docs.
func resolveAdvertiseIP(iface string) (net.IP, error) {
	if iface != "" {
		ifc, err := net.InterfaceByName(iface)
		if err != nil {
			return nil, fmt.Errorf("interface %q: %w", iface, err)
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			return nil, fmt.Errorf("interface %q addrs: %w", iface, err)
		}
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				if v4 := ipn.IP.To4(); v4 != nil && !v4.IsLoopback() {
					return v4, nil
				}
			}
		}
		return nil, fmt.Errorf("interface %q: no usable IPv4 address", iface)
	}

	c, err := net.Dial("udp4", ssdpMulticastAddrV4)
	if err != nil {
		return nil, fmt.Errorf("dial multicast for IP discovery: %w", err)
	}
	defer c.Close()
	if la, ok := c.LocalAddr().(*net.UDPAddr); ok && la.IP != nil {
		if v4 := la.IP.To4(); v4 != nil {
			return v4, nil
		}
	}
	return nil, errors.New("could not determine outbound IPv4 address")
}
