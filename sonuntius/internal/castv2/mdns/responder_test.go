// Maps to: N/A — Go-only tests for the mDNS responder.
//
// We exercise three things:
//
//  1. Constructor validation (missing required fields → error).
//  2. The wire packet built by buildAnswerPacket round-trips through a
//     small DNS parser: header flags are response+AA, four answers, and
//     each answer carries the expected name/type/rdata.
//  3. A synthetic PTR query for _googlecast._tcp.local. is recognised by
//     matchesOurService.
//
// We deliberately do NOT bind a multicast socket here. Tests that need
// actual UDP traffic are gated behind t.Skip("requires multicast") so CI
// works inside restricted containers.
package mdns

import (
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestNewResponderValidation(t *testing.T) {
	cases := []struct {
		name string
		opts Options
		ok   bool
	}{
		{"missing instance", Options{ServiceType: "_googlecast._tcp", Port: 8009}, false},
		{"missing service", Options{InstanceName: "x", Port: 8009}, false},
		{"missing port", Options{InstanceName: "x", ServiceType: "_googlecast._tcp"}, false},
		{"valid", Options{InstanceName: "x", ServiceType: "_googlecast._tcp", Port: 8009}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.opts.Logger = testLogger()
			_, err := NewResponder(tc.opts)
			if (err == nil) != tc.ok {
				t.Errorf("err = %v (expected ok=%v)", err, tc.ok)
			}
		})
	}
}

func TestBuildAnswerPacketShape(t *testing.T) {
	r := mustNewResponder(t)
	r.ipv4 = net.IPv4(192, 0, 2, 5)

	pkt := r.buildAnswerPacket()

	// Header sanity: response bit + AA bit set, ancount=4.
	if len(pkt) < 12 {
		t.Fatalf("packet too short: %d", len(pkt))
	}
	flags := binary.BigEndian.Uint16(pkt[2:4])
	if flags&0x8000 == 0 {
		t.Error("QR bit not set")
	}
	if flags&0x0400 == 0 {
		t.Error("AA bit not set")
	}
	an := binary.BigEndian.Uint16(pkt[6:8])
	if an != 4 {
		t.Errorf("ancount = %d want 4", an)
	}

	answers := decodePacketAnswers(t, pkt, int(an))
	if len(answers) != 4 {
		t.Fatalf("answers decoded = %d want 4", len(answers))
	}

	// Expected: PTR, SRV, TXT, A in this order.
	wantNames := []string{
		"_googlecast._tcp.local.",
		"Sonuntius (Tidal)._googlecast._tcp.local.",
		"Sonuntius (Tidal)._googlecast._tcp.local.",
	}
	wantTypes := []uint16{typePTR, typeSRV, typeTXT, typeA}

	for i, a := range answers {
		if a.rtype != wantTypes[i] {
			t.Errorf("answer[%d] type = %d want %d", i, a.rtype, wantTypes[i])
		}
		if i < 3 && !strings.EqualFold(strings.TrimSuffix(a.name, "."),
			strings.TrimSuffix(wantNames[i], ".")) {
			t.Errorf("answer[%d] name = %q want %q", i, a.name, wantNames[i])
		}
	}

	// PTR rdata must point to the instance FQDN.
	ptrTarget := decodeNameFromRDATA(t, pkt, answers[0].rdataStart)
	if !strings.EqualFold(strings.TrimSuffix(ptrTarget, "."),
		strings.TrimSuffix(r.instanceFQDN, ".")) {
		t.Errorf("PTR target = %q want %q", ptrTarget, r.instanceFQDN)
	}

	// SRV rdata: priority(2) weight(2) port(2) target.
	srv := answers[1].rdata
	if len(srv) < 6 {
		t.Fatalf("SRV rdata too short: %d", len(srv))
	}
	port := binary.BigEndian.Uint16(srv[4:6])
	if port != 8009 {
		t.Errorf("SRV port = %d want 8009", port)
	}

	// TXT rdata: must contain "id=test-uuid" and "fn=Sonuntius (Tidal)".
	txt := string(answers[2].rdata)
	if !strings.Contains(txt, "id=test-uuid") {
		t.Errorf("TXT missing id= record: %q", txt)
	}
	if !strings.Contains(txt, "fn=Sonuntius (Tidal)") {
		t.Errorf("TXT missing fn= record: %q", txt)
	}

	// A rdata: 4 bytes.
	if len(answers[3].rdata) != 4 {
		t.Errorf("A rdata len = %d want 4", len(answers[3].rdata))
	}
	if !net.IP(answers[3].rdata).Equal(net.IPv4(192, 0, 2, 5)) {
		t.Errorf("A rdata = %v want 192.0.2.5", net.IP(answers[3].rdata))
	}
}

func TestParseSyntheticPTRQuery(t *testing.T) {
	// Build a minimal query for _googlecast._tcp.local. type PTR (12).
	pkt := buildQueryPacket("_googlecast._tcp.local.", typePTR)
	msg, err := parseMessage(pkt)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	if msg.qr != 0 {
		t.Error("qr != 0 for query")
	}
	if len(msg.questions) != 1 {
		t.Fatalf("questions = %d", len(msg.questions))
	}
	q := msg.questions[0]
	if !strings.EqualFold(q.name, "_googlecast._tcp.local.") {
		t.Errorf("question name = %q", q.name)
	}
	if q.qtype != typePTR {
		t.Errorf("question type = %d", q.qtype)
	}

	// matchesOurService must accept this question.
	r := mustNewResponder(t)
	if !r.matchesOurService(q) {
		t.Error("matchesOurService = false for PTR _googlecast._tcp.local.")
	}
}

func TestGoodbyePacketTTLZero(t *testing.T) {
	r := mustNewResponder(t)
	r.ipv4 = net.IPv4(192, 0, 2, 1)
	pkt := r.buildAnswerPacketTTL(0)
	answers := decodePacketAnswers(t, pkt, 4)
	for i, a := range answers {
		if a.ttl != 0 {
			t.Errorf("answer[%d] ttl = %d want 0", i, a.ttl)
		}
	}
}

// TestEncodeNameRoundTrip ensures encodeName/decodeName are inverses for a
// representative set of names.
func TestEncodeNameRoundTrip(t *testing.T) {
	names := []string{
		"_googlecast._tcp.local.",
		"Sonuntius (Tidal)._googlecast._tcp.local.",
		"host.local.",
		".",
	}
	for _, n := range names {
		enc := encodeName(n)
		dec, _, err := decodeName(enc, 0)
		if err != nil {
			t.Errorf("decode %q: %v", n, err)
			continue
		}
		if !strings.EqualFold(dec, n) && !(dec == "." && n == ".") {
			t.Errorf("round trip %q -> %q", n, dec)
		}
	}
}

// ---------- helpers ----------

func mustNewResponder(t *testing.T) *Responder {
	t.Helper()
	r, err := NewResponder(Options{
		InstanceName: "Sonuntius (Tidal)",
		ServiceType:  "_googlecast._tcp",
		Port:         8009,
		UUID:         "test-uuid",
		HostName:     "sonuntius",
		TXTRecords:   map[string]string{"fn": "Sonuntius (Tidal)", "md": "Chromecast"},
		Logger:       testLogger(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

type decodedRR struct {
	name       string
	rtype      uint16
	rclass     uint16
	ttl        uint32
	rdata      []byte
	rdataStart int
}

// decodePacketAnswers walks the answer section. Assumes header qd=0 (our
// responses set qd=0).
func decodePacketAnswers(t *testing.T, pkt []byte, ancount int) []decodedRR {
	t.Helper()
	off := 12
	out := make([]decodedRR, 0, ancount)
	for i := 0; i < ancount; i++ {
		name, n, err := decodeName(pkt, off)
		if err != nil {
			t.Fatalf("answer[%d] name: %v", i, err)
		}
		off = n
		if off+10 > len(pkt) {
			t.Fatalf("answer[%d] truncated header", i)
		}
		rtype := binary.BigEndian.Uint16(pkt[off : off+2])
		rclass := binary.BigEndian.Uint16(pkt[off+2 : off+4])
		ttl := binary.BigEndian.Uint32(pkt[off+4 : off+8])
		rdlen := int(binary.BigEndian.Uint16(pkt[off+8 : off+10]))
		off += 10
		if off+rdlen > len(pkt) {
			t.Fatalf("answer[%d] rdata truncated", i)
		}
		out = append(out, decodedRR{
			name:       name,
			rtype:      rtype,
			rclass:     rclass,
			ttl:        ttl,
			rdata:      append([]byte(nil), pkt[off:off+rdlen]...),
			rdataStart: off,
		})
		off += rdlen
	}
	return out
}

func decodeNameFromRDATA(t *testing.T, pkt []byte, off int) string {
	t.Helper()
	s, _, err := decodeName(pkt, off)
	if err != nil {
		t.Fatalf("decode rdata name: %v", err)
	}
	return s
}

func buildQueryPacket(name string, qtype uint16) []byte {
	var hdr [12]byte
	binary.BigEndian.PutUint16(hdr[0:2], 1)    // id
	binary.BigEndian.PutUint16(hdr[2:4], 0)    // flags: query
	binary.BigEndian.PutUint16(hdr[4:6], 1)    // qdcount
	pkt := append([]byte{}, hdr[:]...)
	pkt = append(pkt, encodeName(name)...)
	var tail [4]byte
	binary.BigEndian.PutUint16(tail[0:2], qtype)
	binary.BigEndian.PutUint16(tail[2:4], classIN)
	pkt = append(pkt, tail[:]...)
	return pkt
}

func TestMulticastSkipped(t *testing.T) {
	// Real network test placeholder. Multicast in CI containers is
	// unreliable, so we skip — local devs can flip this if they want to
	// exercise the live socket.
	t.Skip("requires multicast")
}
