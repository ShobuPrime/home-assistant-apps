// Maps to: chromium/openscreen cast/common/channel/proto/cast_channel.proto
//          (the CastMessage message and its inner enums) — hand-rolled
//          protobuf codec because the project is stdlib-only.
//
// Upstream wire schema:
//
//	message CastMessage {
//	    enum ProtocolVersion { CASTV2_1_0 = 0; }
//	    enum PayloadType     { STRING = 0; BINARY = 1; }
//	    required ProtocolVersion protocol_version = 1; // varint
//	    required string          source_id        = 2; // length-delimited
//	    required string          destination_id   = 3; // length-delimited
//	    required string          namespace        = 4; // length-delimited
//	    required PayloadType     payload_type     = 5; // varint
//	    optional string          payload_utf8     = 6; // length-delimited
//	    optional bytes           payload_binary   = 7; // length-delimited
//	}
//
// Each field is encoded as a varint tag header (field_number << 3 | wire_type)
// followed by the payload. Wire types we care about:
//
//	0 = varint        (used for enum fields 1 and 5)
//	2 = length-delim  (used for string/bytes fields 2, 3, 4, 6, 7)
//
// No third-party protobuf library; the codec is small enough to live in one
// file (encode + decode + a round-trip test).
package castv2

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// ProtocolVersion mirrors the upstream proto enum. Only one value is defined.
type ProtocolVersion int32

const (
	// ProtocolVersionCastV21 is the only value the CASTV2 wire understands.
	ProtocolVersionCastV21 ProtocolVersion = 0
)

// PayloadType mirrors the upstream proto enum. CASTV2 payloads are either
// UTF-8 JSON (STRING) or opaque bytes (BINARY); the auth namespace is the
// only one in regular use that ships BINARY payloads.
type PayloadType int32

const (
	// PayloadTypeString denotes a UTF-8 payload in payload_utf8 (field 6).
	PayloadTypeString PayloadType = 0
	// PayloadTypeBinary denotes an opaque payload in payload_binary (field 7).
	PayloadTypeBinary PayloadType = 1
)

// CastMessage is the decoded form of the CastMessage protobuf. Field names
// mirror the proto names for one-to-one upstream comparability.
type CastMessage struct {
	ProtocolVersion ProtocolVersion
	SourceID        string
	DestinationID   string
	Namespace       string
	PayloadType     PayloadType
	PayloadUTF8     string
	PayloadBinary   []byte
}

// errTruncated and friends surface decoder failures with a small amount of
// context so callers (Server.handleConn) can log them at debug.
var (
	errTruncated      = errors.New("castmessage: truncated input")
	errVarintOverflow = errors.New("castmessage: varint overflow")
	errUnknownWire    = errors.New("castmessage: unsupported wire type")
)

// Marshal encodes m to its protobuf wire form. Fields are emitted in tag
// order (1..7) and optional fields (6, 7) are only emitted when non-empty.
// We follow proto2 "required" semantics by always emitting tags 1..5 even
// when their values are the zero value — senders rely on that.
func (m *CastMessage) Marshal() ([]byte, error) {
	// Pre-size the buffer with a rough upper bound; the encoder appends and
	// never reads, so under-allocation is harmless.
	out := make([]byte, 0, 64+len(m.SourceID)+len(m.DestinationID)+
		len(m.Namespace)+len(m.PayloadUTF8)+len(m.PayloadBinary))

	// Tag 1: protocol_version (varint).
	out = appendVarintField(out, 1, uint64(m.ProtocolVersion))
	// Tag 2: source_id (length-delimited string).
	out = appendBytesField(out, 2, []byte(m.SourceID))
	// Tag 3: destination_id (length-delimited string).
	out = appendBytesField(out, 3, []byte(m.DestinationID))
	// Tag 4: namespace (length-delimited string).
	out = appendBytesField(out, 4, []byte(m.Namespace))
	// Tag 5: payload_type (varint).
	out = appendVarintField(out, 5, uint64(m.PayloadType))
	// Tag 6: payload_utf8 (optional length-delimited string). Per proto2
	// optional semantics we omit when empty unless PayloadType says STRING.
	// Omitting an empty optional matches what openscreen / Chrome senders do.
	if m.PayloadType == PayloadTypeString || len(m.PayloadUTF8) > 0 {
		out = appendBytesField(out, 6, []byte(m.PayloadUTF8))
	}
	// Tag 7: payload_binary (optional length-delimited bytes).
	if m.PayloadType == PayloadTypeBinary || len(m.PayloadBinary) > 0 {
		out = appendBytesField(out, 7, m.PayloadBinary)
	}
	return out, nil
}

// Unmarshal parses a CastMessage from b into the receiver. Unknown fields
// are skipped per proto2 forward-compatibility, which is important because
// real senders sometimes include extension fields we don't model here.
func (m *CastMessage) Unmarshal(b []byte) error {
	*m = CastMessage{}
	i := 0
	for i < len(b) {
		key, n, err := readVarint(b[i:])
		if err != nil {
			return err
		}
		i += n
		fieldNum := int(key >> 3)
		wireType := int(key & 0x07)

		switch fieldNum {
		case 1:
			v, n, err := readVarint(b[i:])
			if err != nil {
				return err
			}
			m.ProtocolVersion = ProtocolVersion(int32(v))
			i += n
		case 2:
			s, n, err := readLengthDelim(b[i:])
			if err != nil {
				return err
			}
			m.SourceID = string(s)
			i += n
		case 3:
			s, n, err := readLengthDelim(b[i:])
			if err != nil {
				return err
			}
			m.DestinationID = string(s)
			i += n
		case 4:
			s, n, err := readLengthDelim(b[i:])
			if err != nil {
				return err
			}
			m.Namespace = string(s)
			i += n
		case 5:
			v, n, err := readVarint(b[i:])
			if err != nil {
				return err
			}
			m.PayloadType = PayloadType(int32(v))
			i += n
		case 6:
			s, n, err := readLengthDelim(b[i:])
			if err != nil {
				return err
			}
			m.PayloadUTF8 = string(s)
			i += n
		case 7:
			s, n, err := readLengthDelim(b[i:])
			if err != nil {
				return err
			}
			// Copy out so callers can't mutate our backing buffer.
			m.PayloadBinary = append([]byte(nil), s...)
			i += n
		default:
			// Unknown field — skip according to wire type.
			n, err := skipUnknown(b[i:], wireType)
			if err != nil {
				return fmt.Errorf("castmessage: skip unknown field %d: %w", fieldNum, err)
			}
			i += n
		}
	}
	return nil
}

// appendVarintField emits a (tag, varint) pair into out.
func appendVarintField(out []byte, fieldNum int, value uint64) []byte {
	key := uint64(fieldNum)<<3 | 0 // wire type 0 (varint)
	out = appendVarint(out, key)
	out = appendVarint(out, value)
	return out
}

// appendBytesField emits a (tag, length, bytes) triple into out.
func appendBytesField(out []byte, fieldNum int, value []byte) []byte {
	key := uint64(fieldNum)<<3 | 2 // wire type 2 (length-delimited)
	out = appendVarint(out, key)
	out = appendVarint(out, uint64(len(value)))
	out = append(out, value...)
	return out
}

// appendVarint encodes v as a base-128 varint per the protobuf spec.
func appendVarint(out []byte, v uint64) []byte {
	for v >= 0x80 {
		out = append(out, byte(v)|0x80)
		v >>= 7
	}
	return append(out, byte(v))
}

// readVarint decodes a base-128 varint from b. Returns the value and the
// number of bytes consumed. Errors on overflow or truncation.
func readVarint(b []byte) (uint64, int, error) {
	var v uint64
	var shift uint
	for i, by := range b {
		if i >= 10 {
			return 0, 0, errVarintOverflow
		}
		v |= uint64(by&0x7F) << shift
		if by&0x80 == 0 {
			return v, i + 1, nil
		}
		shift += 7
	}
	return 0, 0, errTruncated
}

// readLengthDelim reads a varint length followed by `length` bytes. Returns
// the slice (aliasing b) and the total number of bytes consumed.
func readLengthDelim(b []byte) ([]byte, int, error) {
	n, used, err := readVarint(b)
	if err != nil {
		return nil, 0, err
	}
	end := used + int(n)
	if end > len(b) || int(n) < 0 {
		return nil, 0, errTruncated
	}
	return b[used:end], end, nil
}

// skipUnknown advances past an unknown field of the given wire type. The
// proto wire format defines six wire types; we handle 0/1/2/5 and reject
// the deprecated start/end-group types (3/4).
func skipUnknown(b []byte, wireType int) (int, error) {
	switch wireType {
	case 0: // varint
		_, n, err := readVarint(b)
		return n, err
	case 1: // 64-bit
		if len(b) < 8 {
			return 0, errTruncated
		}
		return 8, nil
	case 2: // length-delimited
		_, n, err := readLengthDelim(b)
		return n, err
	case 5: // 32-bit
		if len(b) < 4 {
			return 0, errTruncated
		}
		return 4, nil
	default:
		return 0, errUnknownWire
	}
}

// be32 is a tiny helper used by tests to construct length-prefixed frames
// without importing encoding/binary into the test file. Not part of the
// public API.
//
//nolint:unused // kept for tests
func be32(v uint32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return b[:]
}
