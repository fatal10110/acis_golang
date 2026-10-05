package serverpackets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// ---- from writer_test.go ----
func TestFrameWriterPoolability(t *testing.T) {
	normal := wire.NewFrameWriter(packetWriterCapacity)
	if !poolable(normal) {
		t.Fatal("normal writer should be retained")
	}

	oversized := wire.NewFrameWriter(packetWriterCapacity)
	oversized.WriteBytes(make([]byte, maxPacketWriterCapacity))
	if got := oversized.Cap(); got <= maxPacketWriterCapacity {
		t.Fatalf("oversized writer capacity = %d, want > %d", got, maxPacketWriterCapacity)
	}
	if poolable(oversized) {
		t.Fatal("oversized writer should be dropped")
	}
}

func TestCopyFrameShortSourceReportsFailure(t *testing.T) {
	frame, err := CopyFrame(wire.BorrowedFrame([]byte{1}))
	defer frame.Release()
	if err == nil {
		t.Fatalf("CopyFrame short source = %x, nil; want empty frame and an error", frame.Bytes())
	}
}
