package serverpackets

import (
	"bytes"
	"testing"
)

// TestNpcFrameBuilderNpcSayShout pins the chat type of an NPC line: 0 for a
// plain line, 1 for a shout (Npc.broadcastNpcShout).
func TestNpcFrameBuilderNpcSayShout(t *testing.T) {
	for _, tc := range []struct {
		shout bool
		typ   int32
	}{{false, SayTypeAll}, {true, SayTypeShout}} {
		got := framePayload(t, NpcFrameBuilder{}.NpcSay(500, 12564, "Hello", tc.shout))
		want := []byte{OpcodeNpcSay}
		want = appendD(want, 500)
		want = appendD(want, tc.typ)
		want = appendD(want, 1012564)
		want = append(want, encodeUTF16Z("Hello")...)
		if !bytes.Equal(got, want) {
			t.Errorf("NpcSay(shout=%v) = %x, want %x", tc.shout, got, want)
		}
	}
}
