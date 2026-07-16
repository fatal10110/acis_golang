package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestFramePledgeCrest(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03}
	got := framePayload(t, FramePledgeCrest(101, data))

	want := []byte{OpcodePledgeCrest}
	want = binary.LittleEndian.AppendUint32(want, 101)
	want = binary.LittleEndian.AppendUint32(want, uint32(len(data)))
	want = append(want, data...)

	if !bytes.Equal(got, want) {
		t.Fatalf("FramePledgeCrest() = %x, want %x", got, want)
	}
}

func TestFrameAllyCrest(t *testing.T) {
	data := []byte{0x04, 0x05, 0x06}
	got := framePayload(t, FrameAllyCrest(103, data))

	want := []byte{OpcodeAllyCrest}
	want = binary.LittleEndian.AppendUint32(want, 103)
	want = binary.LittleEndian.AppendUint32(want, uint32(len(data)))
	want = append(want, data...)

	if !bytes.Equal(got, want) {
		t.Fatalf("FrameAllyCrest() = %x, want %x", got, want)
	}
}

func TestFramePledgeCrestMissingData(t *testing.T) {
	got := framePayload(t, FramePledgeCrest(101, nil))

	want := []byte{OpcodePledgeCrest}
	want = binary.LittleEndian.AppendUint32(want, 101)
	want = binary.LittleEndian.AppendUint32(want, 0)

	if !bytes.Equal(got, want) {
		t.Fatalf("FramePledgeCrest(nil) = %x, want %x", got, want)
	}
}
