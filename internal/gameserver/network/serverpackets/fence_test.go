package serverpackets

import (
	"bytes"
	"testing"
)

// TestExColosseumFenceInfoGolden pins ExColosseumFenceInfo.java: 0xfe, sub-opcode
// 0x0009 (H), then object id, type, x, y, z, width and length (D each).
func TestExColosseumFenceInfoGolden(t *testing.T) {
	got := frameBytes(t, FrameExColosseumFenceInfo(FenceInfo{
		ObjectID: 0x01020304, Type: 2, X: -71432, Y: 258832, Z: -3104, SizeX: 300, SizeY: 500,
	}))
	want := []byte{
		0x21, 0x00, 0xfe, 0x09, 0x00,
		0x04, 0x03, 0x02, 0x01, // object id
		0x02, 0x00, 0x00, 0x00, // type
		0xf8, 0xe8, 0xfe, 0xff, // x -71432
		0x10, 0xf3, 0x03, 0x00, // y 258832
		0xe0, 0xf3, 0xff, 0xff, // z -3104
		0x2c, 0x01, 0x00, 0x00, // width 300
		0xf4, 0x01, 0x00, 0x00, // length 500
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("ExColosseumFenceInfo = % x, want % x", got, want)
	}
}
