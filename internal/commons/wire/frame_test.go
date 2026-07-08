package wire

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

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
