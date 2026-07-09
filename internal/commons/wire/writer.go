package wire

import (
	"encoding/binary"
	"math"
	"sync"
	"unicode/utf16"
)

const defaultFrameCapacity = 256

var frameWriterPool = sync.Pool{
	New: func() any { return &Writer{} },
}

// Writer assembles little-endian primitives into a packet payload. The zero
// value is ready to use.
type Writer struct {
	buf       []byte
	bodyStart int
	pooled    bool
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
	return w.buf[w.bodyStart:]
}

// NewPacketWriter starts an outbound packet with its leading opcode byte.
func NewPacketWriter(opcode byte) *Writer {
	w := &Writer{}
	w.WriteUint8(opcode)
	return w
}

// NewFramePacketWriter starts an outbound packet with room reserved for the
// frame header.
func NewFramePacketWriter(opcode byte) *Writer {
	w := frameWriterPool.Get().(*Writer)
	w.pooled = true
	w.bodyStart = FrameHeaderSize
	if cap(w.buf) < defaultFrameCapacity {
		w.buf = make([]byte, FrameHeaderSize, defaultFrameCapacity)
	} else {
		w.buf = w.buf[:FrameHeaderSize]
	}
	w.WriteUint8(opcode)
	return w
}

// Frame returns the complete length-prefixed frame.
func (w *Writer) Frame() []byte {
	if w.bodyStart != FrameHeaderSize {
		return FrameBytes(w.Bytes())
	}
	binary.LittleEndian.PutUint16(w.buf[:FrameHeaderSize], uint16(len(w.buf)))
	return w.buf
}

// Release returns a pooled frame writer to the pool. Callers must not use w
// or any bytes returned by it after Release.
func (w *Writer) Release() {
	if w == nil || !w.pooled {
		return
	}
	w.buf = w.buf[:0]
	w.bodyStart = 0
	w.pooled = false
	frameWriterPool.Put(w)
}
