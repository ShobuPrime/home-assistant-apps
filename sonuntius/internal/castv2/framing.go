// Maps to: chromium/openscreen cast/common/channel framing — every CASTV2
//          message on the wire is a 4-byte big-endian length prefix
//          followed by exactly that many bytes of CastMessage protobuf.
//          See openscreen `CastSocket::Write` / `CastSocket::ReadBody`.
//
// We deliberately keep the framing layer separate from the codec so future
// work can re-use either piece in isolation (e.g. tests that hand-build a
// frame byte-by-byte, or a tcpdump-style debug tool that just decodes
// CastMessage payloads from a saved buffer).
package castv2

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// maxFrameBytes caps how much one CastMessage can grow to. Openscreen
// enforces 64 KiB; senders never approach that in practice, but a
// pathological/malicious connection could try to wedge us by claiming a
// huge length. We mirror the upstream cap.
const maxFrameBytes = 64 * 1024

// ErrFrameTooLarge is returned by ReadFrame when the length prefix exceeds
// maxFrameBytes. The caller should close the connection.
var ErrFrameTooLarge = errors.New("castv2: frame exceeds 64 KiB cap")

// WriteFrame serialises msg and writes the length-prefixed frame to w.
// The frame is built in a single contiguous slice so net.Conn writes it as
// one TCP segment whenever possible — Cast senders rely on the frame
// arriving atomically.
func WriteFrame(w io.Writer, msg *CastMessage) error {
	body, err := msg.Marshal()
	if err != nil {
		return fmt.Errorf("castv2: marshal: %w", err)
	}
	if len(body) > maxFrameBytes {
		return ErrFrameTooLarge
	}
	frame := make([]byte, 4+len(body))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(body)))
	copy(frame[4:], body)
	if _, err := w.Write(frame); err != nil {
		return fmt.Errorf("castv2: write frame: %w", err)
	}
	return nil
}

// ReadFrame reads one length-prefixed frame from r and decodes it into a
// fresh CastMessage. Returns io.EOF when the peer closes cleanly between
// frames; returns io.ErrUnexpectedEOF if the peer closes mid-frame.
func ReadFrame(r io.Reader) (*CastMessage, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > maxFrameBytes {
		return nil, ErrFrameTooLarge
	}
	body := make([]byte, int(n))
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	msg := &CastMessage{}
	if err := msg.Unmarshal(body); err != nil {
		return nil, fmt.Errorf("castv2: decode frame: %w", err)
	}
	return msg, nil
}
