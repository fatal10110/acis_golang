package link

import (
	"encoding/binary"
	"unicode/utf16"
)

// writer assembles the little-endian primitives GS-LS link packets use,
// starting with the packet's opcode byte.
type writer struct {
	buf []byte
}

func newWriter(opcode byte) *writer {
	return &writer{buf: []byte{opcode}}
}

func (w *writer) writeByte(b byte) {
	w.buf = append(w.buf, b)
}

func (w *writer) writeInt32(v int32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(v))
	w.buf = append(w.buf, b[:]...)
}

func (w *writer) writeBytes(b []byte) {
	w.buf = append(w.buf, b...)
}

// writeString writes s as UTF-16LE followed by a 2-byte 0x0000 terminator.
func (w *writer) writeString(s string) {
	for _, u := range utf16.Encode([]rune(s)) {
		var b [2]byte
		binary.LittleEndian.PutUint16(b[:], u)
		w.buf = append(w.buf, b[:]...)
	}
	w.buf = append(w.buf, 0, 0)
}

func (w *writer) bytes() []byte {
	return w.buf
}

func boolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}
