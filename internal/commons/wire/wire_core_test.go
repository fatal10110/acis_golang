package wire

import (
	"bytes"
	"errors"
	"io"
	"math"
	"net"
	"testing"
)

// ---- from frame_length_test.go ----
func oversizedPayload() []byte {
	return make([]byte, MaxFrameLength-FrameHeaderSize+1)
}

func TestFrameBytesRejectsOversizedPayload(t *testing.T) {
	if frame, err := FrameBytes(make([]byte, MaxFrameLength-FrameHeaderSize)); err != nil {
		t.Fatalf("FrameBytes(max) error = %v", err)
	} else if len(frame) != MaxFrameLength {
		t.Fatalf("FrameBytes(max) length = %d, want %d", len(frame), MaxFrameLength)
	}
	if _, err := FrameBytes(oversizedPayload()); err == nil {
		t.Fatal("FrameBytes(max + 1) error = nil, want length error")
	}
}

func TestWriteFrameRejectsOversizedPayloadWithoutWriting(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, oversizedPayload()); err == nil {
		t.Fatal("WriteFrame(max + 1) error = nil, want length error")
	}
	if buf.Len() != 0 {
		t.Fatalf("WriteFrame wrote %d bytes for a rejected frame, want 0", buf.Len())
	}
}

func TestBorrowedFrameRejectsOversizedFrame(t *testing.T) {
	frame := BorrowedFrame(make([]byte, MaxFrameLength+1))
	if frame.Err() == nil {
		t.Fatal("BorrowedFrame(max + 1).Err() = nil, want length error")
	}
	if frame.Bytes() != nil {
		t.Fatal("rejected frame carries bytes, want none")
	}
}

func TestOwnedFrameRejectsOversizedFrameAndStillReleases(t *testing.T) {
	w := NewFrameWriter(8)
	released := false
	frame := OwnedFrame(make([]byte, MaxFrameLength+1), w, func(*Writer) { released = true })
	if frame.Err() == nil {
		t.Fatal("OwnedFrame(max + 1).Err() = nil, want length error")
	}
	if frame.Bytes() != nil {
		t.Fatal("rejected frame carries bytes, want none")
	}
	frame.Release()
	if !released {
		t.Fatal("rejected frame did not release its writer")
	}
}

func TestWriterFrameOverLimitIsRejectedByOwnedFrame(t *testing.T) {
	w := NewFrameWriter(8)
	w.WriteBytes(oversizedPayload())
	if frame := OwnedFrame(w.Frame(), w, func(*Writer) {}); frame.Err() == nil {
		t.Fatal("oversized writer frame accepted, want length error")
	}
}

// ---- from frame_test.go ----
func TestWriteReadFrameRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{"empty payload", nil},
		{"short payload", []byte{0x01, 0x02, 0x03}},
		{"payload near 16-bit boundary", bytes.Repeat([]byte{0xaa}, 1<<16-FrameHeaderSize-1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteFrame(&buf, tt.payload); err != nil {
				t.Fatalf("WriteFrame() error = %v", err)
			}

			got, err := ReadFrame(&buf)
			if err != nil {
				t.Fatalf("ReadFrame() error = %v", err)
			}
			if !bytes.Equal(got, tt.payload) {
				t.Fatalf("ReadFrame() = %x, want %x", got, tt.payload)
			}
		})
	}
}

func TestWriteFrameHeader(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, []byte{0x01, 0x02, 0x03}); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}

	want := []byte{0x05, 0x00, 0x01, 0x02, 0x03} // header (2+3=5, little-endian) + payload
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("frame bytes = %x, want %x", buf.Bytes(), want)
	}
}

func TestReadFrameRejectsShortHeader(t *testing.T) {
	// Header claims a length shorter than the header itself.
	r := bytes.NewReader([]byte{0x01, 0x00})
	if _, err := ReadFrame(r); err == nil {
		t.Fatal("ReadFrame() expected error for length < header size, got nil")
	}
}

func TestReadFrameRejectsTruncatedPayload(t *testing.T) {
	// Header claims 6 bytes total (4 payload bytes) but only 2 are present.
	r := bytes.NewReader([]byte{0x06, 0x00, 0x01, 0x02})
	if _, err := ReadFrame(r); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("ReadFrame() error = %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestReadFrameEOFOnEmptyStream(t *testing.T) {
	r := bytes.NewReader(nil)
	if _, err := ReadFrame(r); !errors.Is(err, io.EOF) {
		t.Fatalf("ReadFrame() error = %v, want io.EOF", err)
	}
}

// ---- from framereader_test.go ----
type countingConn struct {
	net.Conn
	r     *bytes.Reader
	reads int
}

func (c *countingConn) Read(p []byte) (int, error) {
	c.reads++
	return c.r.Read(p)
}

func TestFrameReaderReadsSequentialFrames(t *testing.T) {
	var stream []byte
	stream = append(stream, mustFrameBytes([]byte{1, 2, 3})...)
	stream = append(stream, mustFrameBytes([]byte{4, 5})...)

	fr := NewFrameReader(bytes.NewReader(stream))

	first, err := fr.ReadFrame()
	if err != nil {
		t.Fatalf("first ReadFrame: %v", err)
	}
	if !bytes.Equal(first, []byte{1, 2, 3}) {
		t.Fatalf("first payload = % X, want 01 02 03", first)
	}

	second, err := fr.ReadFrame()
	if err != nil {
		t.Fatalf("second ReadFrame: %v", err)
	}
	if !bytes.Equal(second, []byte{4, 5}) {
		t.Fatalf("second payload = % X, want 04 05", second)
	}

	if _, err := fr.ReadFrame(); err != io.EOF {
		t.Fatalf("ReadFrame at end = %v, want io.EOF", err)
	}
}

func TestFrameReaderBuffersConsecutiveFrames(t *testing.T) {
	stream := append(mustFrameBytes([]byte{1, 2, 3}), mustFrameBytes([]byte{4, 5})...)
	conn := &countingConn{r: bytes.NewReader(stream)}
	fr := NewFrameReader(conn)

	for _, want := range [][]byte{{1, 2, 3}, {4, 5}} {
		got, err := fr.ReadFrame()
		if err != nil {
			t.Fatalf("ReadFrame: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("payload = % X, want % X", got, want)
		}
	}
	if conn.reads != 1 {
		t.Fatalf("underlying reads = %d, want 1", conn.reads)
	}
}

func TestFrameReaderRejectsHeaderShorterThanItself(t *testing.T) {
	fr := NewFrameReader(bytes.NewReader([]byte{0x01, 0x00}))
	if _, err := fr.ReadFrame(); err == nil {
		t.Fatal("ReadFrame() err = nil, want an error for a length shorter than the header")
	}
}

func TestFrameReaderReusesItsBuffer(t *testing.T) {
	var stream []byte
	stream = append(stream, mustFrameBytes([]byte{1, 2, 3})...)
	stream = append(stream, mustFrameBytes([]byte{9, 9, 9})...)

	r := bytes.NewReader(stream)
	fr := NewFrameReader(r)

	// Warm the buffer past both frames, then measure a steady-state pass.
	for {
		if _, err := fr.ReadFrame(); err != nil {
			break
		}
	}

	allocs := testing.AllocsPerRun(100, func() {
		if _, err := r.Seek(0, io.SeekStart); err != nil {
			t.Fatalf("seek: %v", err)
		}
		for {
			if _, err := fr.ReadFrame(); err != nil {
				return
			}
		}
	})
	if allocs != 0 {
		t.Errorf("steady-state allocations per pass = %v, want 0", allocs)
	}
}

func BenchmarkReadFrame(b *testing.B) {
	stream := mustFrameBytes(make([]byte, 64))
	r := bytes.NewReader(stream)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := r.Seek(0, io.SeekStart); err != nil {
			b.Fatalf("seek: %v", err)
		}
		if _, err := ReadFrame(r); err != nil {
			b.Fatalf("ReadFrame: %v", err)
		}
	}
}

func BenchmarkFrameReader(b *testing.B) {
	stream := mustFrameBytes(make([]byte, 64))
	r := bytes.NewReader(stream)
	fr := NewFrameReader(r)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := r.Seek(0, io.SeekStart); err != nil {
			b.Fatalf("seek: %v", err)
		}
		if _, err := fr.ReadFrame(); err != nil {
			b.Fatalf("ReadFrame: %v", err)
		}
	}
}

// mustFrameBytes frames a payload short enough that framing cannot fail.
func mustFrameBytes(payload []byte) []byte {
	frame, err := FrameBytes(payload)
	if err != nil {
		panic(err)
	}
	return frame
}

// ---- from reader_test.go ----
func TestReaderPrimitivesRoundTripWriter(t *testing.T) {
	var w Writer
	w.WriteUint8(0x7F)
	w.WriteUint16(0xBEEF)
	w.WriteInt32(-1)
	w.WriteInt64(-2)
	w.WriteBytes([]byte{1, 2, 3})
	w.WriteString("abc")

	r := NewReader(w.Bytes())
	if got := r.ReadUint8(); got != 0x7F {
		t.Fatalf("ReadUint8() = %#x, want 0x7F", got)
	}
	if got := r.ReadUint16(); got != 0xBEEF {
		t.Fatalf("ReadUint16() = %#x, want 0xBEEF", got)
	}
	if got := r.ReadInt32(); got != -1 {
		t.Fatalf("ReadInt32() = %d, want -1", got)
	}
	if got := r.ReadInt64(); got != -2 {
		t.Fatalf("ReadInt64() = %d, want -2", got)
	}
	if got := r.ReadBytes(3); string(got) != "\x01\x02\x03" {
		t.Fatalf("ReadBytes(3) = % X, want 01 02 03", got)
	}
	if got := r.ReadString(); got != "abc" {
		t.Fatalf("ReadString() = %q, want %q", got, "abc")
	}
	if r.Err() != nil {
		t.Fatalf("Err() = %v, want nil", r.Err())
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("Remaining() = %d, want 0", rem)
	}
}

func TestReaderShortPacketSetsErrInsteadOfPanicking(t *testing.T) {
	r := NewReader([]byte{0x01})

	_ = r.ReadUint8()
	if r.Err() != nil {
		t.Fatalf("Err() after in-bounds read = %v, want nil", r.Err())
	}

	if got := r.ReadInt32(); got != 0 {
		t.Fatalf("ReadInt32() past end = %d, want 0", got)
	}
	if r.Err() != ErrShortPacket {
		t.Fatalf("Err() = %v, want %v", r.Err(), ErrShortPacket)
	}

	// Once short, every further read stays zero instead of reading
	// out-of-bounds memory or panicking.
	if got := r.ReadUint8(); got != 0 {
		t.Fatalf("ReadUint8() after short read = %d, want 0", got)
	}
}

func TestReaderNegativeByteCountSetsErrInsteadOfPanicking(t *testing.T) {
	r := NewReader([]byte{0x01, 0x02, 0x03})

	if got := r.ReadBytes(-1); got != nil {
		t.Fatalf("ReadBytes(-1) = % X, want nil", got)
	}
	if r.Err() != ErrShortPacket {
		t.Fatalf("Err() = %v, want %v", r.Err(), ErrShortPacket)
	}
	if rem := r.Remaining(); rem != 3 {
		t.Fatalf("Remaining() = %d, want 3", rem)
	}
}

func TestReaderReadStringWithoutTerminatorIsShort(t *testing.T) {
	r := NewReader([]byte{'a', 0}) // one code unit, no null terminator

	if got := r.ReadString(); got != "a" {
		t.Fatalf("ReadString() = %q, want %q", got, "a")
	}
	if r.Err() != ErrShortPacket {
		t.Fatalf("Err() = %v, want %v", r.Err(), ErrShortPacket)
	}
}

// ---- from writer_test.go ----
func TestUint16Count(t *testing.T) {
	count, err := Uint16Count(math.MaxUint16)
	if err != nil {
		t.Fatalf("Uint16Count(max) error = %v", err)
	}
	if count != math.MaxUint16 {
		t.Fatalf("Uint16Count(max) = %d, want %d", count, math.MaxUint16)
	}

	if _, err := Uint16Count(math.MaxUint16 + 1); err == nil {
		t.Fatal("Uint16Count(max + 1) error = nil, want overflow error")
	}
}

func TestWriterPrimitives(t *testing.T) {
	var w Writer
	w.WriteUint8(0x01)
	w.WriteUint16(0x0203)
	w.WriteInt32(0x04050607)
	w.WriteInt64(0x08090A0B0C0D0E0F)
	w.WriteBytes([]byte{0xAA, 0xBB})

	want := []byte{
		0x01,
		0x03, 0x02,
		0x07, 0x06, 0x05, 0x04,
		0x0F, 0x0E, 0x0D, 0x0C, 0x0B, 0x0A, 0x09, 0x08,
		0xAA, 0xBB,
	}
	if got := w.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("Bytes() = % X, want % X", got, want)
	}
}

func TestWriterFloat64RoundTrips(t *testing.T) {
	var w Writer
	w.WriteFloat64(3.5)

	r := NewReader(w.Bytes())
	if got := r.ReadFloat64(); got != 3.5 {
		t.Fatalf("ReadFloat64() = %v, want 3.5", got)
	}
}

func TestBoolWireValues(t *testing.T) {
	if BoolByte(true) != 1 || BoolByte(false) != 0 {
		t.Fatalf("BoolByte returned unexpected values")
	}
	if BoolInt32(true) != 1 || BoolInt32(false) != 0 {
		t.Fatalf("BoolInt32 returned unexpected values")
	}
}

func TestWriterStringIsNullTerminatedUTF16LE(t *testing.T) {
	var w Writer
	w.WriteString("Hi")

	want := []byte{'H', 0, 'i', 0, 0, 0}
	if got := w.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("Bytes() = % X, want % X", got, want)
	}
}

func TestWriterStringEncodesSurrogatePairs(t *testing.T) {
	var w Writer
	w.WriteString("\U0001F600") // outside the BMP, needs a UTF-16 surrogate pair

	r := NewReader(w.Bytes())
	if got := r.ReadString(); got != "\U0001F600" {
		t.Fatalf("round-trip = %q, want %q", got, "\U0001F600")
	}
}

func TestWriterStringReplacesInvalidUTF8(t *testing.T) {
	var w Writer
	w.WriteString("\xff")

	want := []byte{0xFD, 0xFF, 0, 0}
	if got := w.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("Bytes() = % X, want % X", got, want)
	}
}

func TestWriterStringDoesNotAllocateWithReusedWriter(t *testing.T) {
	w := NewFrameWriter(64)
	allocs := testing.AllocsPerRun(1000, func() {
		w.ResetFrame(64)
		w.WriteString("PlayerName\U0001F600")
	})
	if allocs != 0 {
		t.Fatalf("WriteString allocations = %v, want 0", allocs)
	}
}

func BenchmarkWriteString(b *testing.B) {
	w := NewFrameWriter(64)
	b.ReportAllocs()
	for b.Loop() {
		w.ResetFrame(64)
		w.WriteString("PlayerName")
	}
}

func TestFrameWriterBackfillsHeaderWithoutChangingBytes(t *testing.T) {
	w := NewFrameWriter(16)
	w.WriteUint8(0x14)
	w.WriteInt32(1)

	wantPayload := []byte{0x14, 0x01, 0x00, 0x00, 0x00}
	if got := w.Bytes(); !bytes.Equal(got, wantPayload) {
		t.Fatalf("Bytes() = % X, want % X", got, wantPayload)
	}

	wantFrame := []byte{0x07, 0x00, 0x14, 0x01, 0x00, 0x00, 0x00}
	if got := w.Frame(); !bytes.Equal(got, wantFrame) {
		t.Fatalf("Frame() = % X, want % X", got, wantFrame)
	}
}
