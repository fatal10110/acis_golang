package serverpackets

import (
	"bytes"
	"testing"
)

// CreatureSay's bytes, written out by hand from the reference writeImpl
// layout for a text line: 0x4a, d speaker object id, d chat channel, S
// name, S text.
func TestCreatureSayBytes(t *testing.T) {
	got := framePayload(t, FrameCreatureSay(0x01020304, 2, "->Bo", "hi"))
	want := []byte{
		0x4a,
		0x04, 0x03, 0x02, 0x01,
		0x02, 0x00, 0x00, 0x00,
		'-', 0x00, '>', 0x00, 'B', 0x00, 'o', 0x00, 0x00, 0x00,
		'h', 0x00, 'i', 0x00, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("CreatureSay = % x, want % x", got, want)
	}
}
