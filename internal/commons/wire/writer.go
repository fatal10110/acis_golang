package wire

import (
	"encoding/binary"
	"fmt"
	"math"
	"unicode/utf16"
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
func (w *Writer) WriteString(s string) {
	for _, r := range s {
		// Ranging over a string replaces invalid UTF-8, including encoded surrogates, with RuneError.
		if r < 0x10000 {
			w.WriteUint16(uint16(r))
			continue
		}
		high, low := utf16.EncodeRune(r)
		w.WriteUint16(uint16(high))
		w.WriteUint16(uint16(low))
	}
	w.WriteUint16(0)
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
