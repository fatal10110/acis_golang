package wire

import (
	"encoding/binary"
	"fmt"
	"math"
	"unicode/utf16"
	"unicode/utf8"
)

// Uint16Count validates a count encoded in an unsigned 16-bit packet field.
func Uint16Count(n int) (uint16, error) {
	if n < 0 || n > math.MaxUint16 {
		return 0, fmt.Errorf("wire: uint16 count out of range: %d", n)
	}
	return uint16(n), nil
}

// Writer assembles little-endian primitives into a packet payload. The zero
// value is ready to use.
type Writer struct {
	buf           []byte
	payloadOffset int
}

// NewFrameWriter returns a Writer with space reserved for a frame length
// header before the packet payload.
func NewFrameWriter(capacity int) *Writer {
	w := &Writer{}
	w.ResetFrame(capacity)
	return w
}

// ResetFrame clears w and reserves a frame length header before the payload.
func (w *Writer) ResetFrame(capacity int) {
	if capacity < FrameHeaderSize {
		capacity = FrameHeaderSize
	}
	if cap(w.buf) < capacity {
		w.buf = make([]byte, FrameHeaderSize, capacity)
	} else {
		w.buf = w.buf[:FrameHeaderSize]
		w.buf[0], w.buf[1] = 0, 0
	}
	w.payloadOffset = FrameHeaderSize
}

// WriteUint8 appends a single byte.
func (w *Writer) WriteUint8(b byte) {
	w.buf = append(w.buf, b)
}

// WriteUint16 appends a little-endian 16-bit integer.
func (w *Writer) WriteUint16(v uint16) {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], v)
	w.buf = append(w.buf, b[:]...)
}

// WriteInt32 appends a little-endian 32-bit integer.
func (w *Writer) WriteInt32(v int32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(v))
	w.buf = append(w.buf, b[:]...)
}

// WriteInt64 appends a little-endian 64-bit integer.
func (w *Writer) WriteInt64(v int64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(v))
	w.buf = append(w.buf, b[:]...)
}

// WriteFloat64 appends a little-endian IEEE-754 float64 value.
func (w *Writer) WriteFloat64(v float64) {
	w.WriteInt64(int64(math.Float64bits(v)))
}

// WriteFloat32 appends a little-endian IEEE-754 single-precision float.
func (w *Writer) WriteFloat32(v float32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], math.Float32bits(v))
	w.buf = append(w.buf, b[:]...)
}

// WriteBytes appends raw bytes verbatim.
func (w *Writer) WriteBytes(b []byte) {
	w.buf = append(w.buf, b...)
}

// WriteString appends s as null-terminated UTF-16LE: each rune as one or two
// 16-bit code units, followed by a trailing 0x0000 unit.
//
// A surrogate code unit spelled in its three-byte generalized UTF-8 form
// (ED A0..BF 80..BF, as CESU-8 and WTF-8 write it) is emitted as that single
// unit, so a string carrying an unpaired surrogate keeps it on the wire. Any
// other invalid byte is emitted as U+FFFD.
func (w *Writer) WriteString(s string) {
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			if unit, ok := encodedSurrogate(s[i:]); ok {
				w.WriteUint16(unit)
				n = 3
			} else {
				w.WriteUint16(uint16(utf8.RuneError))
			}
		case r < 0x10000:
			w.WriteUint16(uint16(r))
		default:
			high, low := utf16.EncodeRune(r)
			w.WriteUint16(uint16(high))
			w.WriteUint16(uint16(low))
		}
		i += n
	}
	w.WriteUint16(0)
}

// encodedSurrogate reports the surrogate code unit whose generalized UTF-8
// form begins s.
func encodedSurrogate(s string) (uint16, bool) {
	if len(s) < 3 || s[0] != 0xED || s[1]&0xE0 != 0xA0 || s[2]&0xC0 != 0x80 {
		return 0, false
	}
	return 0xD000 | uint16(s[1]&0x3F)<<6 | uint16(s[2]&0x3F), true
}

// BoolByte returns the packet byte value for b.
func BoolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

// BoolInt32 returns the packet int32 value for b.
func BoolInt32(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

// Len returns the number of bytes written to w so far, including any frame
// header reserved by NewFrameWriter/ResetFrame.
func (w *Writer) Len() int {
	return len(w.buf)
}

// PatchUint16 overwrites the little-endian 16-bit integer at offset (an
// index previously obtained from Len, immediately before the field it now
// patches was written) with v. Used to backfill a count field written
// before its element count was known.
func (w *Writer) PatchUint16(offset int, v uint16) {
	binary.LittleEndian.PutUint16(w.buf[offset:offset+2], v)
}

// PatchInt32 overwrites the little-endian 32-bit integer at offset (an index
// previously obtained from Len, immediately before the field it now patches
// was written) with v. Used to backfill a count field written before its
// element count was known.
func (w *Writer) PatchInt32(offset int, v int32) {
	binary.LittleEndian.PutUint32(w.buf[offset:offset+4], uint32(v))
}

// Bytes returns the assembled payload.
func (w *Writer) Bytes() []byte {
	return w.buf[w.payloadOffset:]
}

// Cap returns the capacity of w's backing buffer.
func (w *Writer) Cap() int {
	return cap(w.buf)
}

// Frame returns the assembled payload behind a little-endian frame length
// header. Writers built with NewFrameWriter backfill that header in place.
// A payload too long for the uint16 header keeps its truncated header here;
// OwnedFrame and BorrowedFrame reject the oversized result before it can be
// queued for send.
func (w *Writer) Frame() []byte {
	if w.payloadOffset != FrameHeaderSize {
		return frameBytes(w.buf)
	}
	binary.LittleEndian.PutUint16(w.buf[:FrameHeaderSize], uint16(len(w.buf)))
	return w.buf
}

// NewPacketWriter starts an outbound packet with its leading opcode byte.
func NewPacketWriter(opcode byte) *Writer {
	w := &Writer{}
	w.WriteUint8(opcode)
	return w
}
