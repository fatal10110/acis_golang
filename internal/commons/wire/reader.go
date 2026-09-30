package wire

import (
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// ErrShortPacket is returned when a read would go past the end of the
// payload — a malformed or truncated inbound packet.
var ErrShortPacket = errors.New("wire: short read")

// ErrStringTooLong is returned when a string field runs past MaxStringUnits
// code units without its terminator.
var ErrStringTooLong = errors.New("wire: string exceeds maximum length")

// MaxStringUnits is the most UTF-16 code units ReadString accepts before the
// terminator: the longest string one frame's payload can carry. It keeps a
// Reader over an arbitrary buffer from scanning or decoding past what any
// single packet could legitimately hold.
const MaxStringUnits = (MaxFrameLength - FrameHeaderSize) / 2

// Reader decodes little-endian primitives from a packet payload, in the
// order they were written.
type Reader struct {
	buf []byte
	pos int
	err error
}

// NewReader wraps payload for sequential decoding. payload is not copied;
// the caller must not mutate it while the reader is in use.
func NewReader(payload []byte) *Reader {
	return &Reader{buf: payload}
}

// NewPacketReader wraps payload for decoding, discarding the leading opcode
// byte every inbound packet carries. A payload shorter than one byte leaves
// the reader's Err() set rather than panicking.
func NewPacketReader(payload []byte) *Reader {
	r := NewReader(payload)
	r.ReadUint8() // opcode
	return r
}

// Err reports the first decode error encountered (ErrShortPacket or
// ErrStringTooLong), if any. Once set, every subsequent read returns the
// type's zero value instead of panicking or reading out of bounds, so a
// decoder can perform a run of reads and check Err once at the end rather
// than after every call.
func (r *Reader) Err() error {
	return r.err
}

// Remaining reports how many unread bytes are left in the payload.
func (r *Reader) Remaining() int {
	return len(r.buf) - r.pos
}

func (r *Reader) take(n int) []byte {
	if r.err != nil || n < 0 || n > r.Remaining() {
		r.err = ErrShortPacket
		return nil
	}
	b := r.buf[r.pos : r.pos+n]
	r.pos += n
	return b
}

// ReadUint8 reads a single byte.
func (r *Reader) ReadUint8() byte {
	b := r.take(1)
	if b == nil {
		return 0
	}
	return b[0]
}

// ReadUint16 reads a little-endian 16-bit integer.
func (r *Reader) ReadUint16() uint16 {
	b := r.take(2)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint16(b)
}

// ReadInt32 reads a little-endian 32-bit integer.
func (r *Reader) ReadInt32() int32 {
	b := r.take(4)
	if b == nil {
		return 0
	}
	return int32(binary.LittleEndian.Uint32(b))
}

// ReadInt64 reads a little-endian 64-bit integer.
func (r *Reader) ReadInt64() int64 {
	b := r.take(8)
	if b == nil {
		return 0
	}
	return int64(binary.LittleEndian.Uint64(b))
}

// ReadFloat64 reads a little-endian IEEE-754 float64 value.
func (r *Reader) ReadFloat64() float64 {
	return math.Float64frombits(uint64(r.ReadInt64()))
}

// ReadBytes reads and copies the next n raw bytes.
func (r *Reader) ReadBytes(n int) []byte {
	b := r.take(n)
	if b == nil {
		return nil
	}
	out := make([]byte, n)
	copy(out, b)
	return out
}

// ReadString reads a null-terminated UTF-16LE string: 16-bit code units up
// to but excluding the trailing 0x0000 unit, which is consumed.
//
// The terminator is located before anything is allocated, so a field with
// no terminator (ErrShortPacket) or with more than MaxStringUnits code units
// before it (ErrStringTooLong) is rejected without allocating and returns "".
// An accepted string is decoded into a single exactly sized allocation.
func (r *Reader) ReadString() string {
	if r.err != nil {
		return ""
	}
	rest := r.buf[r.pos:]
	units := 0
	for {
		off := 2 * units
		if off+2 > len(rest) {
			r.err = ErrShortPacket
			return ""
		}
		if rest[off] == 0 && rest[off+1] == 0 {
			break
		}
		if units == MaxStringUnits {
			r.err = ErrStringTooLong
			return ""
		}
		units++
	}
	r.pos += 2*units + 2
	return decodeUTF16LE(rest[:2*units])
}

// decodeUTF16LE converts an even-length UTF-16LE byte run to a string,
// replacing unpaired surrogates with U+FFFD as utf16.Decode does. It sizes
// the result exactly first so the string is built in one allocation.
func decodeUTF16LE(b []byte) string {
	size := 0
	for i := 0; i < len(b); {
		c, w := utf16LERune(b[i:])
		size += utf8.RuneLen(c)
		i += w
	}
	var sb strings.Builder
	sb.Grow(size)
	for i := 0; i < len(b); {
		c, w := utf16LERune(b[i:])
		sb.WriteRune(c)
		i += w
	}
	return sb.String()
}

// utf16LERune decodes the code point starting at b and reports how many
// bytes it spans: 4 for a valid surrogate pair, otherwise 2.
func utf16LERune(b []byte) (rune, int) {
	c := rune(binary.LittleEndian.Uint16(b))
	if !utf16.IsSurrogate(c) {
		return c, 2
	}
	if len(b) >= 4 {
		if pair := utf16.DecodeRune(c, rune(binary.LittleEndian.Uint16(b[2:]))); pair != utf8.RuneError {
			return pair, 4
		}
	}
	return utf8.RuneError, 2
}
