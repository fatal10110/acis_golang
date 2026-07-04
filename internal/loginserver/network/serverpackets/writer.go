package serverpackets

import "encoding/binary"

// writer assembles the little-endian primitives login server packets use,
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

func (w *writer) writeInt16(v uint16) {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], v)
	w.buf = append(w.buf, b[:]...)
}

func (w *writer) writeInt32(v int32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(v))
	w.buf = append(w.buf, b[:]...)
}

func (w *writer) writeBytes(b []byte) {
	w.buf = append(w.buf, b...)
}

func (w *writer) bytes() []byte {
	return w.buf
}
