// Package mqtt is a minimal MQTT 3.1.1 client written against the
// standard library only — no third-party MQTT dependency. It implements
// just what AegisHA needs: CONNECT (with a Last-Will), QoS-0 PUBLISH (with
// retain), SUBSCRIBE, keepalive PING, and inbound PUBLISH dispatch, plus
// automatic reconnect with backoff. Subscriptions and the OnConnect hook
// are replayed on every (re)connect so HA discovery and retained state
// are always re-announced after a broker blip.
package mqtt

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

// Message is an inbound or outbound MQTT message.
type Message struct {
	Topic   string
	Payload []byte
	Retain  bool
}

// Handler receives inbound messages for a subscription.
type Handler func(Message)

// Options configures a Client.
type Options struct {
	Broker    string // host:port
	ClientID  string
	Username  string
	Password  string
	KeepAlive time.Duration
	TLS       *tls.Config // nil => plaintext TCP
	Will      *Message    // Last-Will-and-Testament (published by the broker on ungraceful disconnect)
	OnConnect func(*Client)
	Logger    *slog.Logger
}

const (
	pktCONNECT    = 0x10
	pktCONNACK    = 0x20
	pktPUBLISH    = 0x30
	pktSUBSCRIBE  = 0x80
	pktSUBACK     = 0x90
	pktPINGREQ    = 0xC0
	pktPINGRESP   = 0xD0
	pktDISCONNECT = 0xE0
)

type subscription struct {
	filter string
	qos    byte
	fn     Handler
}

// Client is a reconnecting MQTT 3.1.1 client.
type Client struct {
	opts Options
	log  *slog.Logger

	mu        sync.Mutex
	conn      net.Conn
	connected bool

	writeMu  sync.Mutex
	packetID uint16

	subMu sync.Mutex
	subs  []subscription
}

// New constructs a Client. Call Run in a goroutine to maintain the
// connection.
func New(opts Options) *Client {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.KeepAlive == 0 {
		opts.KeepAlive = 30 * time.Second
	}
	return &Client{opts: opts, log: opts.Logger}
}

// Connected reports whether a broker session is currently established.
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// Run maintains the connection until ctx is cancelled, reconnecting with
// exponential backoff.
func (c *Client) Run(ctx context.Context) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		err := c.session(ctx)
		if ctx.Err() != nil {
			return
		}
		c.log.Warn("mqtt: session ended", "err", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
}

func (c *Client) session(ctx context.Context) error {
	conn, err := c.dial(ctx)
	if err != nil {
		return err
	}
	c.setConn(conn)
	defer c.clearConn()

	if err := c.sendConnect(); err != nil {
		return err
	}
	br := bufio.NewReader(conn)
	if err := c.readConnack(conn, br); err != nil {
		return err
	}
	c.log.Info("mqtt: connected", "broker", c.opts.Broker, "client_id", c.opts.ClientID)

	// Replay subscriptions, then run the OnConnect hook (discovery + state).
	c.subMu.Lock()
	subs := append([]subscription(nil), c.subs...)
	c.subMu.Unlock()
	for _, s := range subs {
		if err := c.sendSubscribe(s.filter, s.qos); err != nil {
			return err
		}
	}
	if c.opts.OnConnect != nil {
		c.opts.OnConnect(c)
	}

	sessCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go c.keepAlive(sessCtx, conn)
	return c.readLoop(sessCtx, conn, br)
}

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	d := &net.Dialer{Timeout: 10 * time.Second}
	if c.opts.TLS != nil {
		return tls.DialWithDialer(d, "tcp", c.opts.Broker, c.opts.TLS)
	}
	return d.DialContext(ctx, "tcp", c.opts.Broker)
}

func (c *Client) setConn(conn net.Conn) {
	c.mu.Lock()
	c.conn, c.connected = conn, true
	c.mu.Unlock()
}

func (c *Client) clearConn() {
	c.mu.Lock()
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn, c.connected = nil, false
	c.mu.Unlock()
}

// --- outbound packets ---

func (c *Client) sendConnect() error {
	var vh []byte
	vh = appendString(vh, "MQTT")
	vh = append(vh, 4) // protocol level 3.1.1

	var flags byte = 0x02 // clean session
	if c.opts.Username != "" {
		flags |= 0x80
	}
	if c.opts.Password != "" {
		flags |= 0x40
	}
	if c.opts.Will != nil {
		flags |= 0x04 // will flag, will QoS 0
		if c.opts.Will.Retain {
			flags |= 0x20
		}
	}
	vh = append(vh, flags)
	vh = binary.BigEndian.AppendUint16(vh, uint16(c.opts.KeepAlive/time.Second))

	var payload []byte
	payload = appendString(payload, c.opts.ClientID)
	if c.opts.Will != nil {
		payload = appendString(payload, c.opts.Will.Topic)
		payload = appendBytes(payload, c.opts.Will.Payload)
	}
	if c.opts.Username != "" {
		payload = appendString(payload, c.opts.Username)
	}
	if c.opts.Password != "" {
		payload = appendString(payload, c.opts.Password)
	}
	return c.writePacket(pktCONNECT, append(vh, payload...))
}

// Publish sends a QoS-0 PUBLISH.
func (c *Client) Publish(topic string, payload []byte, retain bool) error {
	b1 := byte(pktPUBLISH)
	if retain {
		b1 |= 0x01
	}
	var p []byte
	p = appendString(p, topic)
	p = append(p, payload...)
	return c.writePacket(b1, p)
}

// Subscribe registers a handler for a topic filter and sends a SUBSCRIBE
// if currently connected. The subscription is replayed on reconnect.
func (c *Client) Subscribe(filter string, fn Handler) error {
	c.subMu.Lock()
	c.subs = append(c.subs, subscription{filter: filter, qos: 0, fn: fn})
	c.subMu.Unlock()
	if c.Connected() {
		return c.sendSubscribe(filter, 0)
	}
	return nil
}

func (c *Client) sendSubscribe(filter string, qos byte) error {
	c.writeMu.Lock()
	id := c.nextPacketID()
	c.writeMu.Unlock()
	var p []byte
	p = binary.BigEndian.AppendUint16(p, id)
	p = appendString(p, filter)
	p = append(p, qos)
	return c.writePacket(pktSUBSCRIBE|0x02, p)
}

// Disconnect sends a graceful MQTT DISCONNECT. The broker will NOT then
// publish the Last-Will, so callers should publish their own retained
// "offline" status first if they want one.
func (c *Client) Disconnect() {
	_ = c.writePacket(pktDISCONNECT, nil)
}

func (c *Client) nextPacketID() uint16 {
	c.packetID++
	if c.packetID == 0 {
		c.packetID = 1
	}
	return c.packetID
}

func (c *Client) writePacket(header byte, remaining []byte) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return errors.New("mqtt: not connected")
	}
	var buf []byte
	buf = append(buf, header)
	buf = appendRemainingLength(buf, len(remaining))
	buf = append(buf, remaining...)

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := conn.Write(buf)
	return err
}

func (c *Client) keepAlive(ctx context.Context, conn net.Conn) {
	t := time.NewTicker(c.opts.KeepAlive)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := c.writePacket(pktPINGREQ, nil); err != nil {
				_ = conn.Close() // force the read loop to error and reconnect
				return
			}
		}
	}
}

// --- inbound ---

func (c *Client) readConnack(conn net.Conn, br *bufio.Reader) error {
	_ = conn.SetReadDeadline(time.Now().Add(c.opts.KeepAlive * 2))
	typ, payload, err := readPacket(br)
	if err != nil {
		return err
	}
	if typ&0xF0 != pktCONNACK {
		return fmt.Errorf("mqtt: expected CONNACK, got 0x%02x", typ)
	}
	if len(payload) < 2 {
		return errors.New("mqtt: short CONNACK")
	}
	if payload[1] != 0 {
		return fmt.Errorf("mqtt: connection refused, code %d", payload[1])
	}
	return nil
}

func (c *Client) readLoop(ctx context.Context, conn net.Conn, br *bufio.Reader) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_ = conn.SetReadDeadline(time.Now().Add(c.opts.KeepAlive * 2))
		typ, payload, err := readPacket(br)
		if err != nil {
			return err
		}
		switch typ & 0xF0 {
		case pktPUBLISH:
			c.dispatch(typ, payload)
		case pktPINGRESP, pktSUBACK:
			// nothing to do
		}
	}
}

func (c *Client) dispatch(header byte, payload []byte) {
	retain := header&0x01 != 0
	qos := (header >> 1) & 0x03
	if len(payload) < 2 {
		return
	}
	tlen := int(binary.BigEndian.Uint16(payload[:2]))
	if len(payload) < 2+tlen {
		return
	}
	topic := string(payload[2 : 2+tlen])
	rest := payload[2+tlen:]
	if qos > 0 {
		if len(rest) < 2 { // skip the packet id; we never request QoS>0 though
			return
		}
		rest = rest[2:]
	}
	msg := Message{Topic: topic, Payload: rest, Retain: retain}

	c.subMu.Lock()
	subs := append([]subscription(nil), c.subs...)
	c.subMu.Unlock()
	for _, s := range subs {
		if topicMatch(s.filter, topic) {
			s.fn(msg)
		}
	}
}

// --- wire helpers ---

func appendString(b []byte, s string) []byte {
	return appendBytes(b, []byte(s))
}

func appendBytes(b, v []byte) []byte {
	b = binary.BigEndian.AppendUint16(b, uint16(len(v)))
	return append(b, v...)
}

func appendRemainingLength(b []byte, n int) []byte {
	for {
		digit := byte(n % 128)
		n /= 128
		if n > 0 {
			digit |= 0x80
		}
		b = append(b, digit)
		if n == 0 {
			return b
		}
	}
}

func readPacket(br *bufio.Reader) (byte, []byte, error) {
	first, err := br.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	n, err := readRemainingLength(br)
	if err != nil {
		return 0, nil, err
	}
	buf := make([]byte, n)
	if _, err := readFull(br, buf); err != nil {
		return 0, nil, err
	}
	return first, buf, nil
}

func readRemainingLength(br *bufio.Reader) (int, error) {
	var value, mult int
	for range 4 {
		b, err := br.ReadByte()
		if err != nil {
			return 0, err
		}
		value += int(b&0x7F) << mult
		if b&0x80 == 0 {
			return value, nil
		}
		mult += 7
	}
	return 0, errors.New("mqtt: malformed remaining length")
}

func readFull(br *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := br.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// topicMatch implements MQTT topic-filter matching (+ and # wildcards).
func topicMatch(filter, topic string) bool {
	fs := splitTopic(filter)
	ts := splitTopic(topic)
	for i, f := range fs {
		if f == "#" {
			return true
		}
		if i >= len(ts) {
			return false
		}
		if f != "+" && f != ts[i] {
			return false
		}
	}
	return len(fs) == len(ts)
}

func splitTopic(t string) []string {
	var out []string
	start := 0
	for i := 0; i < len(t); i++ {
		if t[i] == '/' {
			out = append(out, t[start:i])
			start = i + 1
		}
	}
	return append(out, t[start:])
}
