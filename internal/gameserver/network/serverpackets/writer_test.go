package serverpackets

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

func TestReleaseFrameWriterDropsOversizedBuffer(t *testing.T) {
	originalPool := packetWriterPool
	packetWriterPool = &sync.Pool{New: func() any { return wire.NewFrameWriter(packetWriterCapacity) }}
	t.Cleanup(func() { packetWriterPool = originalPool })

	w := newFrameWriter(0)
	w.WriteBytes(make([]byte, maxPacketWriterCapacity))
	releaseFrameWriter(w)

	if got := cap(newFrameWriter(0).Frame()); got != packetWriterCapacity {
		t.Fatalf("pooled writer capacity = %d, want %d after oversized frame", got, packetWriterCapacity)
	}
}
