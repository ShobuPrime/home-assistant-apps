package ipc

import (
	"bufio"
	"fmt"
	"net"

	"github.com/shobuprime/sonuntius/internal/events"
)

// Client is the receiver end of the JSON-line UDS protocol. It is the
// counterpart that cast-receiver, yt-cast, and sonuntius-ctl use to
// talk to ma-bridge.
type Client struct {
	conn   *net.UnixConn
	writer *bufio.Writer
	reader *bufio.Scanner
}

// Dial opens the UDS connection at path. Pass "" to use SocketPath().
func Dial(path string) (*Client, error) {
	if path == "" {
		path = SocketPath()
	}
	addr := &net.UnixAddr{Name: path, Net: "unix"}
	conn, err := net.DialUnix("unix", nil, addr)
	if err != nil {
		return nil, fmt.Errorf("ipc: dial %s: %w", path, err)
	}
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	return &Client{
		conn:   conn,
		writer: bufio.NewWriter(conn),
		reader: sc,
	}, nil
}

// Send pushes a single event to the server.
func (c *Client) Send(ev events.Event) error {
	payload, err := events.Marshal(ev)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if _, err := c.writer.Write(payload); err != nil {
		return err
	}
	return c.writer.Flush()
}

// Recv reads the next event, or returns nil + io.EOF on close.
func (c *Client) Recv() (events.Event, error) {
	if !c.reader.Scan() {
		if err := c.reader.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("ipc: connection closed")
	}
	return events.Unmarshal(c.reader.Bytes())
}

// Close terminates the connection.
func (c *Client) Close() error {
	return c.conn.Close()
}
