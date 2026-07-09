package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestFrameSSQInfo(t *testing.T) {
	got := framePayload(t, FrameSSQInfo())

	want := []byte{OpcodeSSQInfo}
	want = binary.LittleEndian.AppendUint16(want, regularSkyState)

	if !bytes.Equal(got, want) {
		t.Errorf("FrameSSQInfo() = % x, want % x", got, want)
	}
}
