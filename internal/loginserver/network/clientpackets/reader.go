package clientpackets

import "encoding/binary"

// reader decodes the little-endian primitives login client packets use,
// starting after the leading opcode byte every packet carries.
type reader struct {
	buf []byte
	pos int
}

func newReader(payload []byte) *reader {
	return &reader{buf: payload, pos: 1}
}

// remaining reports how many unread bytes are left in the payload.
func (r *reader) remaining() int {
	return len(r.buf) - r.pos
}

// readInt32 reads a little-endian 32-bit integer.
func (r *reader) readInt32() int32 {
	v := int32(binary.LittleEndian.Uint32(r.buf[r.pos:]))
	r.pos += 4
	return v
}

// readByte reads a single byte.
func (r *reader) readByte() byte {
	b := r.buf[r.pos]
	r.pos++
	return b
}

// readBytes reads and copies the next n raw bytes.
func (r *reader) readBytes(n int) []byte {
	b := make([]byte, n)
	copy(b, r.buf[r.pos:r.pos+n])
	r.pos += n
	return b
}
