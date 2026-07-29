package serverpackets

import (
	"bytes"
	"testing"
)

func TestFrameNpcSay(t *testing.T) {
	got := framePayload(t, FrameNpcSay(500, 12564, SayTypeAll, "Hello"))
	want := []byte{OpcodeNpcSay}
	want = appendD(want, 500)
	want = appendD(want, 0)
	want = appendD(want, 1012564)
	want = append(want, encodeUTF16Z("Hello")...)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameNpcSay() = %x, want %x", got, want)
	}
}
