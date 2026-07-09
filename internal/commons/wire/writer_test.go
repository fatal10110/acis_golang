package wire

import (
	"bytes"
	"testing"
)

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
