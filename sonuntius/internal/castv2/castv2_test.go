// Maps to: N/A — Go-only tests for the castv2 wire codec and framing.
//
// These tests pin down the on-the-wire shape that real Cast senders see.
// If any of them break and you did not intend a wire format change, the
// hand-rolled protobuf codec has drifted and senders will reject our
// messages without an obvious clue in the logs.
package castv2

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

// TestCastMessageRoundTrip covers all seven fields with both payload types.
// Encoding then decoding must yield the same struct.
func TestCastMessageRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   CastMessage
	}{
		{
			name: "string payload all fields",
			in: CastMessage{
				ProtocolVersion: ProtocolVersionCastV21,
				SourceID:        "sender-0",
				DestinationID:   "receiver-0",
				Namespace:       NamespaceReceiver,
				PayloadType:     PayloadTypeString,
				PayloadUTF8:     `{"type":"GET_STATUS","requestId":42}`,
			},
		},
		{
			name: "binary payload",
			in: CastMessage{
				ProtocolVersion: ProtocolVersionCastV21,
				SourceID:        "sender-0",
				DestinationID:   "receiver-0",
				Namespace:       NamespaceDeviceAuth,
				PayloadType:     PayloadTypeBinary,
				PayloadBinary:   []byte{0x00, 0x01, 0x02, 0xff, 0xfe},
			},
		},
		{
			name: "empty string payload still emits field 6",
			in: CastMessage{
				ProtocolVersion: ProtocolVersionCastV21,
				SourceID:        "s",
				DestinationID:   "d",
				Namespace:       "ns",
				PayloadType:     PayloadTypeString,
				PayloadUTF8:     "",
			},
		},
		{
			name: "large json payload",
			in: CastMessage{
				ProtocolVersion: ProtocolVersionCastV21,
				SourceID:        "client-99",
				DestinationID:   "receiver-0",
				Namespace:       NamespaceMedia,
				PayloadType:     PayloadTypeString,
				PayloadUTF8: `{"type":"LOAD","requestId":1,"media":{"contentId":"abc",` +
					`"contentType":"audio/mpeg","customData":{"tidal":{"trackId":"12345"}}}}`,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := tc.in.Marshal()
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var got CastMessage
			if err := got.Unmarshal(b); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if got.ProtocolVersion != tc.in.ProtocolVersion {
				t.Errorf("ProtocolVersion = %v want %v", got.ProtocolVersion, tc.in.ProtocolVersion)
			}
			if got.SourceID != tc.in.SourceID {
				t.Errorf("SourceID = %q want %q", got.SourceID, tc.in.SourceID)
			}
			if got.DestinationID != tc.in.DestinationID {
				t.Errorf("DestinationID = %q want %q", got.DestinationID, tc.in.DestinationID)
			}
			if got.Namespace != tc.in.Namespace {
				t.Errorf("Namespace = %q want %q", got.Namespace, tc.in.Namespace)
			}
			if got.PayloadType != tc.in.PayloadType {
				t.Errorf("PayloadType = %v want %v", got.PayloadType, tc.in.PayloadType)
			}
			if got.PayloadUTF8 != tc.in.PayloadUTF8 {
				t.Errorf("PayloadUTF8 = %q want %q", got.PayloadUTF8, tc.in.PayloadUTF8)
			}
			if !bytes.Equal(got.PayloadBinary, tc.in.PayloadBinary) {
				t.Errorf("PayloadBinary = %x want %x", got.PayloadBinary, tc.in.PayloadBinary)
			}
		})
	}
}

// TestCastMessageUnknownField verifies forward compatibility: an unknown
// field encoded with each supported wire type is skipped without error.
func TestCastMessageUnknownField(t *testing.T) {
	// Build a message with our standard fields then append an unknown
	// tag-99 varint, length-delimited, 32-bit, and 64-bit field.
	msg := &CastMessage{
		ProtocolVersion: ProtocolVersionCastV21,
		SourceID:        "s",
		DestinationID:   "d",
		Namespace:       "ns",
		PayloadType:     PayloadTypeString,
		PayloadUTF8:     "{}",
	}
	b, err := msg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	// tag 99 wire type 0 (varint) — key = 99<<3|0 = 792
	b = append(b, encodeVarintForTest(792)...)
	b = append(b, encodeVarintForTest(12345)...)
	// tag 100 wire type 2 (length-delim) — key = 100<<3|2 = 802
	b = append(b, encodeVarintForTest(802)...)
	b = append(b, encodeVarintForTest(3)...)
	b = append(b, []byte{0xde, 0xad, 0xbe}...)
	// tag 101 wire type 5 (32-bit) — key = 101<<3|5 = 813
	b = append(b, encodeVarintForTest(813)...)
	b = append(b, []byte{0x01, 0x02, 0x03, 0x04}...)
	// tag 102 wire type 1 (64-bit) — key = 102<<3|1 = 817
	b = append(b, encodeVarintForTest(817)...)
	b = append(b, []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}...)

	var got CastMessage
	if err := got.Unmarshal(b); err != nil {
		t.Fatalf("Unmarshal with unknown fields: %v", err)
	}
	if got.SourceID != "s" || got.PayloadUTF8 != "{}" {
		t.Fatalf("known fields corrupted: %+v", got)
	}
}

func encodeVarintForTest(v uint64) []byte {
	var out []byte
	for v >= 0x80 {
		out = append(out, byte(v)|0x80)
		v >>= 7
	}
	return append(out, byte(v))
}

// TestFramingRoundTrip writes three messages through WriteFrame and reads
// them back via ReadFrame from the same buffer.
func TestFramingRoundTrip(t *testing.T) {
	msgs := []*CastMessage{
		{
			ProtocolVersion: ProtocolVersionCastV21,
			SourceID:        "a",
			DestinationID:   "b",
			Namespace:       "ns1",
			PayloadType:     PayloadTypeString,
			PayloadUTF8:     `{"x":1}`,
		},
		{
			ProtocolVersion: ProtocolVersionCastV21,
			SourceID:        "a",
			DestinationID:   "b",
			Namespace:       "ns2",
			PayloadType:     PayloadTypeBinary,
			PayloadBinary:   []byte{0x10, 0x20, 0x30},
		},
		{
			ProtocolVersion: ProtocolVersionCastV21,
			SourceID:        "a",
			DestinationID:   "b",
			Namespace:       "ns3",
			PayloadType:     PayloadTypeString,
			PayloadUTF8:     `{"a":[1,2,3,4,5]}`,
		},
	}

	var buf bytes.Buffer
	for _, m := range msgs {
		if err := WriteFrame(&buf, m); err != nil {
			t.Fatalf("WriteFrame: %v", err)
		}
	}

	// Sanity check: each frame should start with a big-endian length.
	hdr := buf.Bytes()[:4]
	want := uint32(0)
	{
		b, _ := msgs[0].Marshal()
		want = uint32(len(b))
	}
	if got := binary.BigEndian.Uint32(hdr); got != want {
		t.Errorf("first frame length-prefix = %d want %d", got, want)
	}

	for i, want := range msgs {
		got, err := ReadFrame(&buf)
		if err != nil {
			t.Fatalf("ReadFrame[%d]: %v", i, err)
		}
		if got.Namespace != want.Namespace {
			t.Errorf("frame %d namespace = %q want %q", i, got.Namespace, want.Namespace)
		}
		if got.PayloadUTF8 != want.PayloadUTF8 {
			t.Errorf("frame %d utf8 = %q want %q", i, got.PayloadUTF8, want.PayloadUTF8)
		}
		if !bytes.Equal(got.PayloadBinary, want.PayloadBinary) {
			t.Errorf("frame %d binary = %x want %x", i, got.PayloadBinary, want.PayloadBinary)
		}
	}

	// After draining all frames, ReadFrame should return io.EOF.
	if _, err := ReadFrame(&buf); err != io.EOF {
		t.Errorf("post-drain ReadFrame err = %v want io.EOF", err)
	}
}

// TestFramingFrameTooLarge confirms WriteFrame rejects oversize messages.
func TestFramingFrameTooLarge(t *testing.T) {
	// Build a payload that, after protobuf overhead, exceeds 64 KiB.
	big := make([]byte, maxFrameBytes+1)
	msg := &CastMessage{
		ProtocolVersion: ProtocolVersionCastV21,
		SourceID:        "s",
		DestinationID:   "d",
		Namespace:       "n",
		PayloadType:     PayloadTypeBinary,
		PayloadBinary:   big,
	}
	if err := WriteFrame(&bytes.Buffer{}, msg); err != ErrFrameTooLarge {
		t.Errorf("WriteFrame err = %v want ErrFrameTooLarge", err)
	}
}

// TestFromCastMessage validates the high-level Message projection.
func TestFromCastMessage(t *testing.T) {
	cm := &CastMessage{
		ProtocolVersion: ProtocolVersionCastV21,
		SourceID:        "sender-0",
		DestinationID:   "receiver-0",
		Namespace:       NamespaceReceiver,
		PayloadType:     PayloadTypeString,
		PayloadUTF8:     `{"type":"GET_STATUS"}`,
	}
	m := FromCastMessage(cm)
	if m.SourceID != "sender-0" {
		t.Errorf("SourceID = %q", m.SourceID)
	}
	if string(m.PayloadJSON) != `{"type":"GET_STATUS"}` {
		t.Errorf("PayloadJSON = %q", m.PayloadJSON)
	}
	if m.IsBinary() {
		t.Error("IsBinary = true want false")
	}

	cm.PayloadType = PayloadTypeBinary
	cm.PayloadUTF8 = ""
	cm.PayloadBinary = []byte{0x01, 0x02}
	m = FromCastMessage(cm)
	if !m.IsBinary() {
		t.Error("IsBinary = false want true")
	}
}
