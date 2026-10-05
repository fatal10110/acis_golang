package wire

import (
	"bytes"
	"errors"
	"testing"
)

// TestCopyFrameReportsWhyItRefuses pins that CopyFrame keeps a refused
// source's cause: an invalid frame's own Err stays reachable through the
// returned error, and a short frame names its length, so whoever reports
// the refusal does not have to infer the cause from a recipient.
func TestCopyFrameReportsWhyItRefuses(t *testing.T) {
	t.Run("invalid", func(t *testing.T) {
		w := NewFrameWriter(4)
		w.WriteBytes(make([]byte, MaxFrameLength))
		released := 0
		source := OwnedFrame(w.Frame(), w, func(*Writer) { released++ })
		cause := source.Err()
		if cause == nil {
			t.Fatal("oversized OwnedFrame.Err() = nil, want length error")
		}

		frame, err := CopyFrame(source)
		if !errors.Is(err, cause) {
			t.Fatalf("CopyFrame(invalid) error = %v, want it to wrap %v", err, cause)
		}
		if frame.Bytes() != nil || frame.Err() != nil {
			t.Fatalf("CopyFrame(invalid) frame = %x / %v, want the zero frame", frame.Bytes(), frame.Err())
		}
		source.Release()
		if released != 1 {
			t.Fatalf("source release hook ran %d times, want 1: the caller still owns the source", released)
		}
	})
	t.Run("short", func(t *testing.T) {
		_, err := CopyFrame(BorrowedFrame([]byte{1}))
		if err == nil || err.Error() != ShortFrameError(1).Error() {
			t.Fatalf("CopyFrame(1 byte) error = %v, want %v", err, ShortFrameError(1))
		}
	})
	t.Run("valid", func(t *testing.T) {
		source := BorrowedFrame([]byte{5, 0, 1, 2, 3})
		frame, err := CopyFrame(source)
		if err != nil {
			t.Fatalf("CopyFrame(valid) error = %v", err)
		}
		defer frame.Release()
		if !bytes.Equal(frame.Bytes(), source.Bytes()) {
			t.Fatalf("CopyFrame(valid) = %x, want %x", frame.Bytes(), source.Bytes())
		}
		if &frame.Bytes()[0] == &source.Bytes()[0] {
			t.Fatal("CopyFrame(valid) shares the source's storage")
		}
	})
}
