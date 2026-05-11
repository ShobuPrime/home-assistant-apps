// Maps to: N/A — Go-only mDNS / DNS-SD responder (RFC 6762 + RFC 6763).
//
// The lifecycle mirrors internal/ytcast/dial/ssdp.go (stdlib UDP-multicast
// responder loop, periodic re-announcements, goodbye on Stop) but the
// packet format is DNS-wire (RFC 1035 header + question/answer sections)
// rather than HTTPMU. The multicast endpoint is 224.0.0.251:5353.
//
// We advertise one DNS-SD instance under <ServiceType>.local. with the
// four record types Cast senders look for:
//
//   - PTR  : _googlecast._tcp.local.        -> <InstanceName>._googlecast._tcp.local.
//   - SRV  : <InstanceName>._googlecast.... -> <host>.local. port 8009
//   - TXT  : <InstanceName>._googlecast.... -> "key=value" rdata pairs (RFC 6763 §6)
//   - A    : <host>.local.                  -> our outbound IPv4 address
//
// The responder is a one-shot responder: it answers every PTR/SRV/TXT/A
// query for the configured instance, re-announces unsolicited every
// AnnouncePeriod, and emits goodbye records (TTL=0) on Stop so caches
// expire promptly. We do not implement probing/conflict detection — the
// addon runs in `host_network: true` and the InstanceName is expected to
// be unique on the LAN.
package mdns

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// mDNS wire constants.
const (
	mdnsMulticastAddrV4 = "224.0.0.251:5353"

	// DefaultAnnouncePeriod is how often unsolicited re-announcements fire.
	// 60 s matches what python-zeroconf and avahi emit in steady state.
	DefaultAnnouncePeriod = 60 * time.Second

	// recordTTL is what we put in every answer's TTL field. RFC 6762 §10
	// recommends 120 s for "shared" records (PTR) and 120 s for "unique"
	// records (SRV, TXT, A); we keep all four uniform.
	recordTTL = 120
)

// DNS class + cache-flush bit.
const (
	classIN         uint16 = 1
	cacheFlush      uint16 = 0x8000
	classINUnicast  uint16 = classIN | cacheFlush
	classINMulti    uint16 = classIN
	classQUUnicast  uint16 = classIN | 0x8000 // QU bit on questions
	classQMMulticst uint16 = classIN
)

// DNS record types we care about.
const (
	typeA    uint16 = 1
	typePTR  uint16 = 12
	typeTXT  uint16 = 16
	typeSRV  uint16 = 33
	typeANY  uint16 = 255
	typeAAAA uint16 = 28
)

// Options configure a Responder.
type Options struct {
	// InstanceName is the human-readable service instance name. For Cast,
	// senders display this in the picker. Example: "Sonuntius (Tidal)".
	InstanceName string

	// ServiceType is the DNS-SD service type. For Cast: "_googlecast._tcp".
	// The ".local." domain suffix is appended automatically.
	ServiceType string

	// Port is the TCP port the receiver listens on (8009 for CASTV2).
	Port int

	// UUID is the Cast id surfaced in the TXT record as `id=<uuid>`.
	// Senders use it as the stable identity for the receiver — must
	// persist across addon restarts.
	UUID string

	// HostName is the leaf hostname embedded in SRV records and the A
	// record. Defaults to os.Hostname() + ".local.".
	HostName string

	// TXTRecords are additional key/value pairs to include in the TXT
	// record (e.g. fn=<friendly-name>, md=Chromecast, ca=4101, ve=05).
	// The `id` key is overwritten from Options.UUID.
	TXTRecords map[string]string

	// BindInterface optionally pins the interface used for multicast
	// listening + outbound IP resolution. Empty means auto-detect.
	BindInterface string

	// AnnouncePeriod is how often unsolicited announcements fire.
	// Defaults to DefaultAnnouncePeriod.
	AnnouncePeriod time.Duration

	// Logger receives lifecycle and error events.
	Logger *slog.Logger
}

// Responder owns the mDNS UDP multicast socket and answers queries for
// the configured service instance.
type Responder struct {
	opts Options
	log  *slog.Logger

	conn *net.UDPConn

	// Cached pieces of the response (so we don't rebuild on every query).
	host        string // <hostname>.local.
	serviceFQDN string // <type>.local.
	instanceFQDN string // <instance>.<type>.local.
	ipv4         net.IP

	cancel   context.CancelFunc
	wg       sync.WaitGroup
	stopOnce sync.Once
}

// NewResponder constructs a Responder. Network sockets are not opened
// until Start.
func NewResponder(opts Options) (*Responder, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.InstanceName == "" {
		return nil, errors.New("mdns: InstanceName is required")
	}
	if opts.ServiceType == "" {
		return nil, errors.New("mdns: ServiceType is required")
	}
	if opts.Port <= 0 {
		return nil, errors.New("mdns: Port must be > 0")
	}
	if opts.AnnouncePeriod <= 0 {
		opts.AnnouncePeriod = DefaultAnnouncePeriod
	}
	if opts.HostName == "" {
		h, err := os.Hostname()
		if err != nil || h == "" {
			h = "sonuntius"
		}
		opts.HostName = h
	}
	if opts.TXTRecords == nil {
		opts.TXTRecords = map[string]string{}
	}
	if opts.UUID != "" {
		opts.TXTRecords["id"] = opts.UUID
	}

	st := strings.TrimSuffix(opts.ServiceType, ".")
	host := strings.TrimSuffix(opts.HostName, ".")
	r := &Responder{
		opts:         opts,
		log:          opts.Logger,
		host:         host + ".local.",
		serviceFQDN:  st + ".local.",
		instanceFQDN: opts.InstanceName + "." + st + ".local.",
	}
	return r, nil
}

// Start binds the UDP multicast socket and launches the read +
// announcement goroutines. Returns once the socket is listening.
func (r *Responder) Start(parent context.Context) error {
	addr, err := net.ResolveUDPAddr("udp4", mdnsMulticastAddrV4)
	if err != nil {
		return fmt.Errorf("mdns: resolve multicast: %w", err)
	}
	var iface *net.Interface
	if r.opts.BindInterface != "" {
		iface, err = net.InterfaceByName(r.opts.BindInterface)
		if err != nil {
			return fmt.Errorf("mdns: interface %q: %w", r.opts.BindInterface, err)
		}
	}
	conn, err := net.ListenMulticastUDP("udp4", iface, addr)
	if err != nil {
		return fmt.Errorf("mdns: listen multicast %s: %w", mdnsMulticastAddrV4, err)
	}
	_ = conn.SetReadBuffer(1 << 20)
	r.conn = conn

	ip, err := resolveAdvertiseIP(r.opts.BindInterface)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("mdns: resolve advertise IP: %w", err)
	}
	r.ipv4 = ip

	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel

	r.wg.Add(2)
	go r.readLoop(ctx)
	go r.announceLoop(ctx)

	// Initial unsolicited announcement so listeners discover us without
	// waiting AnnouncePeriod.
	r.sendAnnounce()
	r.log.Info("mdns: responder online",
		"instance", r.instanceFQDN,
		"service", r.serviceFQDN,
		"port", r.opts.Port,
		"ip", r.ipv4.String())
	return nil
}

// Stop emits goodbye announcements (TTL=0) for every owned record and
// closes the socket.
func (r *Responder) Stop() error {
	var err error
	r.stopOnce.Do(func() {
		r.sendGoodbye()
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

// readLoop drains the multicast socket, parses incoming queries, and
// answers any whose question matches the records we own.
func (r *Responder) readLoop(ctx context.Context) {
	defer r.wg.Done()
	buf := make([]byte, 2048)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_ = r.conn.SetReadDeadline(time.Now().Add(time.Second))
		n, src, err := r.conn.ReadFromUDP(buf)
		if err != nil {
			var nerr net.Error
			if errors.As(err, &nerr) && nerr.Timeout() {
				continue
			}
			if errors.Is(err, net.ErrClosed) {
				return
			}
			r.log.Debug("mdns: read error", "err", err)
			continue
		}
		r.handleDatagram(buf[:n], src)
	}
}

// announceLoop ticks unsolicited announcements at AnnouncePeriod.
func (r *Responder) announceLoop(ctx context.Context) {
	defer r.wg.Done()
	t := time.NewTicker(r.opts.AnnouncePeriod)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.sendAnnounce()
		}
	}
}

// handleDatagram parses one inbound mDNS message and, if any question
// matches our owned records, sends a unicast or multicast response.
func (r *Responder) handleDatagram(pkt []byte, src *net.UDPAddr) {
	msg, err := parseMessage(pkt)
	if err != nil {
		// Malformed datagrams are common on a noisy LAN; debug, not warn.
		r.log.Debug("mdns: parse error", "err", err)
		return
	}
	if msg.qr != 0 {
		// Response, not a query — ignore.
		return
	}
	for _, q := range msg.questions {
		if r.matchesOurService(q) {
			r.respondToQuery(q, src)
		}
	}
}

// matchesOurService reports whether question q targets a record we own.
func (r *Responder) matchesOurService(q question) bool {
	if equalFoldName(q.name, r.serviceFQDN) {
		// PTR or ANY query for our service type.
		return q.qtype == typePTR || q.qtype == typeANY
	}
	if equalFoldName(q.name, r.instanceFQDN) {
		return q.qtype == typeSRV || q.qtype == typeTXT ||
			q.qtype == typeANY
	}
	if equalFoldName(q.name, r.host) {
		return q.qtype == typeA || q.qtype == typeANY
	}
	return false
}

// respondToQuery builds an answer-only response for q. Per RFC 6762 §6,
// responses go to 224.0.0.251:5353 unless the query had the QU bit set
// (asking for a unicast reply); we emit multicast either way for
// simplicity — that is what python-zeroconf and avahi do under steady
// state.
func (r *Responder) respondToQuery(q question, src *net.UDPAddr) {
	pkt := r.buildAnswerPacket()
	dst, err := net.ResolveUDPAddr("udp4", mdnsMulticastAddrV4)
	if err != nil {
		return
	}
	if _, err := r.conn.WriteToUDP(pkt, dst); err != nil {
		r.log.Debug("mdns: write answer failed", "err", err)
	}
}

// sendAnnounce multicasts an unsolicited announcement carrying all four
// records (PTR, SRV, TXT, A). Sent on Start and at AnnouncePeriod.
func (r *Responder) sendAnnounce() {
	if r.conn == nil {
		return
	}
	pkt := r.buildAnswerPacket()
	dst, err := net.ResolveUDPAddr("udp4", mdnsMulticastAddrV4)
	if err != nil {
		return
	}
	if _, err := r.conn.WriteToUDP(pkt, dst); err != nil {
		r.log.Debug("mdns: write announce failed", "err", err)
	}
}

// sendGoodbye multicasts the same record set with TTL=0 so listeners
// invalidate cached entries immediately.
func (r *Responder) sendGoodbye() {
	if r.conn == nil {
		return
	}
	pkt := r.buildAnswerPacketTTL(0)
	dst, err := net.ResolveUDPAddr("udp4", mdnsMulticastAddrV4)
	if err != nil {
		return
	}
	_, _ = r.conn.WriteToUDP(pkt, dst)
}

// buildAnswerPacket constructs the standard 4-record answer message.
func (r *Responder) buildAnswerPacket() []byte {
	return r.buildAnswerPacketTTL(recordTTL)
}

// buildAnswerPacketTTL is the parameterised form so sendGoodbye can emit
// the same records with TTL=0.
func (r *Responder) buildAnswerPacketTTL(ttl uint32) []byte {
	var b bytes.Buffer
	// Header: id=0, flags=0x8400 (response + authoritative),
	// qdcount=0, ancount=4, nscount=0, arcount=0.
	writeHeader(&b, 0, 0x8400, 0, 4, 0, 0)

	// PTR _googlecast._tcp.local. -> <instance>._googlecast._tcp.local.
	writeRR(&b, r.serviceFQDN, typePTR, classINMulti, ttl, encodeName(r.instanceFQDN))

	// SRV <instance> -> 0 0 <port> <host>.
	srv := encodeSRV(0, 0, uint16(r.opts.Port), r.host)
	writeRR(&b, r.instanceFQDN, typeSRV, classINUnicast, ttl, srv)

	// TXT <instance> -> key=value strings.
	txt := encodeTXT(r.opts.TXTRecords)
	writeRR(&b, r.instanceFQDN, typeTXT, classINUnicast, ttl, txt)

	// A <host> -> ipv4.
	v4 := r.ipv4.To4()
	if v4 == nil {
		v4 = net.IPv4zero.To4()
	}
	writeRR(&b, r.host, typeA, classINUnicast, ttl, []byte(v4))

	return b.Bytes()
}

// ---------- DNS-wire codec ----------

// message is the decoded form of an mDNS packet. We only model the bits
// the responder actually inspects.
type message struct {
	id        uint16
	qr        uint8 // 0=query, 1=response
	questions []question
}

// question is a parsed entry from the question section.
type question struct {
	name  string
	qtype uint16
	qclas uint16
}

// parseMessage parses just enough of a DNS message to extract questions.
// Answer / authority / additional sections are skipped — we never react
// to them.
func parseMessage(pkt []byte) (*message, error) {
	if len(pkt) < 12 {
		return nil, errors.New("packet too short")
	}
	id := binary.BigEndian.Uint16(pkt[0:2])
	flags := binary.BigEndian.Uint16(pkt[2:4])
	qd := int(binary.BigEndian.Uint16(pkt[4:6]))
	msg := &message{id: id, qr: uint8(flags >> 15)}
	off := 12
	for i := 0; i < qd; i++ {
		name, n, err := decodeName(pkt, off)
		if err != nil {
			return nil, fmt.Errorf("question %d: %w", i, err)
		}
		off = n
		if off+4 > len(pkt) {
			return nil, errors.New("question truncated")
		}
		qt := binary.BigEndian.Uint16(pkt[off : off+2])
		qc := binary.BigEndian.Uint16(pkt[off+2 : off+4])
		off += 4
		msg.questions = append(msg.questions, question{
			name:  name,
			qtype: qt,
			qclas: qc,
		})
	}
	return msg, nil
}

// writeHeader emits the 12-byte DNS header.
func writeHeader(b *bytes.Buffer, id, flags, qd, an, ns, ar uint16) {
	var hdr [12]byte
	binary.BigEndian.PutUint16(hdr[0:2], id)
	binary.BigEndian.PutUint16(hdr[2:4], flags)
	binary.BigEndian.PutUint16(hdr[4:6], qd)
	binary.BigEndian.PutUint16(hdr[6:8], an)
	binary.BigEndian.PutUint16(hdr[8:10], ns)
	binary.BigEndian.PutUint16(hdr[10:12], ar)
	b.Write(hdr[:])
}

// writeRR emits one resource record: NAME TYPE CLASS TTL RDLENGTH RDATA.
// We do not use name compression — the packets are small enough that the
// few duplicate-label bytes are not worth the complexity.
func writeRR(b *bytes.Buffer, name string, rtype, rclass uint16, ttl uint32, rdata []byte) {
	b.Write(encodeName(name))
	var hdr [10]byte
	binary.BigEndian.PutUint16(hdr[0:2], rtype)
	binary.BigEndian.PutUint16(hdr[2:4], rclass)
	binary.BigEndian.PutUint32(hdr[4:8], ttl)
	binary.BigEndian.PutUint16(hdr[8:10], uint16(len(rdata)))
	b.Write(hdr[:])
	b.Write(rdata)
}

// encodeName encodes a fully-qualified DNS name in length-prefixed-label
// form, terminated by a 0 byte. The trailing dot is optional in the
// input.
func encodeName(name string) []byte {
	name = strings.TrimSuffix(name, ".")
	if name == "" {
		return []byte{0}
	}
	var b bytes.Buffer
	for _, label := range strings.Split(name, ".") {
		if len(label) > 63 {
			// Truncate ridiculously long labels rather than refusing —
			// callers control InstanceName so this should never happen
			// in practice, but be defensive.
			label = label[:63]
		}
		b.WriteByte(byte(len(label)))
		b.WriteString(label)
	}
	b.WriteByte(0)
	return b.Bytes()
}

// decodeName decodes a possibly-compressed DNS name starting at offset
// `off` in pkt. Returns the decoded name (with trailing dot) and the
// offset *after* the name (which, for compressed names, is the byte
// after the 2-byte pointer, not after the target labels).
func decodeName(pkt []byte, off int) (string, int, error) {
	var labels []string
	jumped := false
	jumpedTo := 0
	for {
		if off >= len(pkt) {
			return "", 0, errors.New("name truncated")
		}
		b := pkt[off]
		if b == 0 {
			off++
			break
		}
		if b&0xC0 == 0xC0 {
			if off+1 >= len(pkt) {
				return "", 0, errors.New("compressed pointer truncated")
			}
			ptr := int(binary.BigEndian.Uint16(pkt[off:off+2]) & 0x3FFF)
			if !jumped {
				jumpedTo = off + 2
				jumped = true
			}
			off = ptr
			continue
		}
		l := int(b)
		off++
		if off+l > len(pkt) {
			return "", 0, errors.New("label truncated")
		}
		labels = append(labels, string(pkt[off:off+l]))
		off += l
	}
	if jumped {
		off = jumpedTo
	}
	if len(labels) == 0 {
		return ".", off, nil
	}
	return strings.Join(labels, ".") + ".", off, nil
}

// encodeSRV builds the rdata for an SRV record: priority, weight, port,
// target.
func encodeSRV(priority, weight, port uint16, target string) []byte {
	var b bytes.Buffer
	var hdr [6]byte
	binary.BigEndian.PutUint16(hdr[0:2], priority)
	binary.BigEndian.PutUint16(hdr[2:4], weight)
	binary.BigEndian.PutUint16(hdr[4:6], port)
	b.Write(hdr[:])
	b.Write(encodeName(target))
	return b.Bytes()
}

// encodeTXT builds the rdata for a TXT record: a sequence of length-
// prefixed strings of the form "key=value".
//
// Order is sorted by key to keep tests deterministic; real senders do not
// care about TXT ordering.
func encodeTXT(kv map[string]string) []byte {
	keys := make([]string, 0, len(kv))
	for k := range kv {
		keys = append(keys, k)
	}
	// Simple insertion sort — keys are tiny and avoiding "sort" keeps the
	// import list lean.
	for i := 1; i < len(keys); i++ {
		j := i
		for j > 0 && keys[j-1] > keys[j] {
			keys[j-1], keys[j] = keys[j], keys[j-1]
			j--
		}
	}
	var b bytes.Buffer
	for _, k := range keys {
		entry := k + "=" + kv[k]
		if len(entry) > 255 {
			entry = entry[:255]
		}
		b.WriteByte(byte(len(entry)))
		b.WriteString(entry)
	}
	// An empty TXT rdata is illegal — emit a single zero-length string.
	if b.Len() == 0 {
		b.WriteByte(0)
	}
	return b.Bytes()
}

// equalFoldName compares two DNS names case-insensitively. Both inputs
// are expected to be in dotted form with a trailing dot.
func equalFoldName(a, b string) bool {
	return strings.EqualFold(strings.TrimSuffix(a, "."), strings.TrimSuffix(b, "."))
}

// resolveAdvertiseIP picks the IPv4 address to embed in the A record.
// If iface is set, returns the first IPv4 unicast address on that
// interface; otherwise dials the mDNS multicast address (no packet
// leaves the host) and asks the kernel which IP it would have used.
//
// Modeled after internal/ytcast/dial/ssdp.go::resolveAdvertiseIP.
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
		return nil, fmt.Errorf("interface %q: no IPv4 address", iface)
	}
	c, err := net.Dial("udp4", mdnsMulticastAddrV4)
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
