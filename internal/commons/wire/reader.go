package wire

import (
	"encoding/binary"
	"errors"
	"math"
	"unicode/utf16"
)

// ErrShortPacket is returned when a read would go past the end of the
// payload — a malformed or truncated inbound packet.
var ErrShortPacket = errors.New("wire: short read")

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

// Err reports the first short-read error encountered, if any. Once set,
// every subsequent read returns the type's zero value instead of panicking
// or reading out of bounds, so a decoder can perform a run of reads and
// check Err once at the end rather than after every call.
func (r *Reader) Err() error {
	return r.err
}

// Remaining reports how many unread bytes are left in the payload.
func (r *Reader) Remaining() int {
	return len(r.buf) - r.pos
}

func (r *Reader) take(n int) []byte {
	if r.err != nil || n > r.Remaining() {
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

// ReadInt16 reads a little-endian 16-bit integer.
func (r *Reader) ReadInt16() uint16 {
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

// ReadFloat64 reads a little-endian IEEE-754 double.
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
func (r *Reader) ReadString() string {
	var units []uint16
	for {
		u := r.ReadInt16()
		if r.err != nil || u == 0 {
			break
		}
		units = append(units, u)
	}
	return string(utf16.Decode(units))
}
