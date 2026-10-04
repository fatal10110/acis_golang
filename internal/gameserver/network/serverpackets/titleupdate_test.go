package serverpackets

import (
	"bytes"
	"testing"
)

// Golden frame: a 2-byte little-endian length that counts itself, then
// TitleUpdate's body as the reference writes it: 0xcc, the object id (D),
// the title (S).
func TestTitleUpdateGolden(t *testing.T) {
	body := append([]byte{0xcc}, appendD(nil, 0x10000001)...)
	body = append(body, encodeUTF16Z("Hero")...)
	want := appendH(nil, uint16(len(body)+2))
	want = append(want, body...)
	if got := frameBytes(t, FrameTitleUpdate(0x10000001, "Hero")); !bytes.Equal(got, want) {
		t.Fatalf("TitleUpdate frame = % x, want % x", got, want)
	}
}
