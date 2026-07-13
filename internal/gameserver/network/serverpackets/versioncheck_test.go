package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestFrameVersionCheck(t *testing.T) {
	key := bytes.Repeat([]byte{0xcc}, 16)

	frame := FrameVersionCheck(key)
	defer frame.Release()

	var want []byte
	want = binary.LittleEndian.AppendUint16(want, uint16(2+1+1+versionCheckKeySize+4+4))
	want = append(want, OpcodeVersionCheck)
	want = append(want, 0x01)
	want = append(want, key[:versionCheckKeySize]...)
	want = binary.LittleEndian.AppendUint32(want, 1)
	want = binary.LittleEndian.AppendUint32(want, 1)

	if !bytes.Equal(frame.Bytes(), want) {
		t.Errorf("FrameVersionCheck() = %x, want %x", frame.Bytes(), want)
	}
}
