package serverpackets

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestFrameExRegenMax(t *testing.T) {
	got := framePayload(t, FrameExRegenMax(14, 2, 16))
	want := []byte{OpcodeExtended}
	want = binary.LittleEndian.AppendUint16(want, OpcodeExRegenMax)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 14)
	want = binary.LittleEndian.AppendUint32(want, 2)
	want = binary.LittleEndian.AppendUint64(want, math.Float64bits(16*0.66))
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameExRegenMax() = %x, want %x", got, want)
	}
}
