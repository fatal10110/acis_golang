package wire

import (
	"encoding/binary"
	"math"
	"unicode/utf16"
)

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

// WriteFloat64 appends a little-endian IEEE-754 double.
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
	for _, unit := range utf16.Encode([]rune(s)) {
		w.WriteUint16(unit)
	}
	w.WriteUint16(0)
}

// Bytes returns the assembled payload.
func (w *Writer) Bytes() []byte {
	return w.buf[w.payloadOffset:]
}

// Frame returns the assembled payload behind a little-endian frame length
// header. Writers built with NewFrameWriter backfill that header in place.
func (w *Writer) Frame() []byte {
	if w.payloadOffset != FrameHeaderSize {
		return FrameBytes(w.buf)
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
