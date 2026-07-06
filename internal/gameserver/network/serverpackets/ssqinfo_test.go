package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEncodeSSQInfo(t *testing.T) {
	got := EncodeSSQInfo()

	want := []byte{OpcodeSSQInfo}
	want = binary.LittleEndian.AppendUint16(want, regularSkyState)

	if !bytes.Equal(got, want) {
		t.Errorf("EncodeSSQInfo() = % x, want % x", got, want)
	}
}
