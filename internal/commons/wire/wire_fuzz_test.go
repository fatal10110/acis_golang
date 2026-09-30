package wire

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/decodefuzz"
)

// FuzzFrameReader feeds a listener's frame reader an arbitrary inbound byte
// stream. Every frame it returns must be exactly as long as its header
// says, and it must stop with an error, never a panic, at the first
// malformed header or truncated frame.
func FuzzFrameReader(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x01, 0x00})                         // length below the header size
	f.Add([]byte{0x02, 0x00})                         // empty payload
	f.Add([]byte{0x07, 0x00, 0x00, 0xea, 0x02, 0x00}) // ProtocolVersion 746, one byte short
	f.Add([]byte{0x07, 0x00, 0x00, 0xea, 0x02, 0x00, 0x00, 0x03, 0x00, 0x03})
	f.Add([]byte{0xff, 0xff, 0x00}) // maximum length, truncated
	f.Fuzz(func(t *testing.T, stream []byte) {
		frames := NewFrameReader(bytes.NewReader(stream))
		consumed := 0
		for {
			payload, err := frames.ReadFrame()
			if err != nil {
				return
			}
			header := int(binary.LittleEndian.Uint16(stream[consumed:]))
			if header != FrameHeaderSize+len(payload) {
				t.Fatalf("frame header %d returned a %d-byte payload", header, len(payload))
			}
			consumed += header
			if consumed > len(stream) {
				t.Fatalf("frames consumed %d bytes of a %d-byte stream", consumed, len(stream))
			}
		}
	})
}

// Reader operations a FuzzReader program can invoke, one per program byte.
const (
	opUint8 = iota
	opUint16
	opInt32
	opInt64
	opFloat64
	opBytes // the next program byte is the length
	opString
	opCount
)

// FuzzReader runs an arbitrary sequence of reads over an arbitrary payload,
// the way a decoder walks an attacker-supplied packet. No read may panic or
// read past the payload; once a read fails the error must stick and every
// later read must return the zero value; and no read may allocate more than
// the payload justifies.
func FuzzReader(f *testing.F) {
	f.Add([]byte{opString, opInt32, opInt32, opInt32, opInt32}, []byte{0x08, 'a', 0, 'b', 0, 0, 0, 1, 0, 0, 0, 2, 0, 0, 0, 3, 0, 0, 0, 4, 0, 0, 0})
	f.Add([]byte{opUint16, opBytes, 4, opString}, []byte{0xd0, 0x2f, 0x00, 1, 2, 3, 4, 'x', 0})
	f.Add([]byte{opString}, []byte{0x21, 'n', 0, 'p', 0, 'c'})
	f.Add([]byte{opInt64, opFloat64, opUint8}, []byte{})
	f.Fuzz(func(t *testing.T, program, payload []byte) {
		decodefuzz.Bounded(t, "Reader", payload, func() { runReaderProgram(t, program, payload) })
	})
}

func runReaderProgram(t *testing.T, program, payload []byte) {
	r := NewReader(payload)
	remaining := r.Remaining()
	for pc := 0; pc < len(program); pc++ {
		failed := r.Err() != nil
		var zero bool
		switch program[pc] % opCount {
		case opUint8:
			zero = r.ReadUint8() == 0
		case opUint16:
			zero = r.ReadUint16() == 0
		case opInt32:
			zero = r.ReadInt32() == 0
		case opInt64:
			zero = r.ReadInt64() == 0
		case opFloat64:
			zero = r.ReadFloat64() == 0
		case opBytes:
			n := 0
			if pc+1 < len(program) {
				pc++
				n = int(program[pc])
			}
			b := r.ReadBytes(n)
			zero = b == nil
			if r.Err() == nil && len(b) != n {
				t.Fatalf("ReadBytes(%d) returned %d bytes", n, len(b))
			}
		case opString:
			s := r.ReadString()
			zero = s == ""
			if r.Err() == nil && len(s) > 3*(remaining-r.Remaining())/2 {
				t.Fatalf("ReadString decoded %d bytes from %d consumed", len(s), remaining-r.Remaining())
			}
		}
		if failed && (!zero || r.Err() == nil) {
			t.Fatalf("read after error %v returned a value or cleared the error", r.Err())
		}
		if err := r.Err(); err != nil && !errors.Is(err, ErrShortPacket) && !errors.Is(err, ErrStringTooLong) {
			t.Fatalf("unexpected reader error %v", err)
		}
		if r.Remaining() < 0 || r.Remaining() > remaining {
			t.Fatalf("Remaining went from %d to %d", remaining, r.Remaining())
		}
		remaining = r.Remaining()
	}
}
