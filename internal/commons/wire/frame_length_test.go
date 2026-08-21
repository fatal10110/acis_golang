package wire

import (
	"bytes"
	"testing"
)

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
