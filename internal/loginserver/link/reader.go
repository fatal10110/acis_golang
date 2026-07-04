package link

import (
	"encoding/binary"
	"fmt"
	"unicode/utf16"
)

// reader decodes the little-endian primitives GS-LS link packets use,
// starting after the leading opcode byte every packet carries. Once a read
// runs past the end of the payload, the reader records the first error and
// every subsequent read becomes a no-op returning the zero value, so a
// decoder can perform all its reads unconditionally and check err once at
// the end.
type reader struct {
	buf []byte
	pos int
	err error
}

func newReader(payload []byte) *reader {
	return &reader{buf: payload, pos: 1}
}

// remaining reports how many unread bytes are left in the payload.
func (r *reader) remaining() int {
	return len(r.buf) - r.pos
}

func (r *reader) need(n int) bool {
	if r.err != nil {
		return false
	}
	if n < 0 || r.remaining() < n {
		r.err = fmt.Errorf("need %d bytes, got %d", n, r.remaining())
		return false
	}
	return true
}

func (r *reader) readByte() byte {
	if !r.need(1) {
		return 0
	}
	b := r.buf[r.pos]
	r.pos++
	return b
}

func (r *reader) readUint16() uint16 {
	if !r.need(2) {
		return 0
	}
	v := binary.LittleEndian.Uint16(r.buf[r.pos:])
	r.pos += 2
	return v
}

func (r *reader) readInt32() int32 {
	if !r.need(4) {
		return 0
	}
	v := int32(binary.LittleEndian.Uint32(r.buf[r.pos:]))
	r.pos += 4
	return v
}

func (r *reader) readBytes(n int) []byte {
	if !r.need(n) {
		return nil
	}
	b := make([]byte, n)
	copy(b, r.buf[r.pos:r.pos+n])
	r.pos += n
	return b
}

// readString reads a UTF-16LE string terminated by a 2-byte 0x0000.
func (r *reader) readString() string {
	if r.err != nil {
		return ""
	}
	start := r.pos
	end := start
	for end+1 < len(r.buf) && (r.buf[end] != 0 || r.buf[end+1] != 0) {
		end += 2
	}
	if end+1 >= len(r.buf) {
		r.err = fmt.Errorf("unterminated string at offset %d", start)
		return ""
	}
	r.pos = end + 2
	units := make([]uint16, (end-start)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(r.buf[start+i*2:])
	}
	return string(utf16.Decode(units))
}
