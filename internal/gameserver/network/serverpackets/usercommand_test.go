package serverpackets

import (
	"bytes"
	"testing"
)

// TestFrameExMultiPartyCommandChannelInfo pins the command channel overview
// byte for byte: 0xfe, sub-opcode 0x30, the leader's name, a zero channel
// loot field, the member count, the party count, then per party its
// leader's name, object id and member count. Strings are UTF-16LE with a
// zero terminator.
func TestFrameExMultiPartyCommandChannelInfo(t *testing.T) {
	got := framePayload(t, FrameExMultiPartyCommandChannelInfo("Al", 5, []ChannelParty{
		{LeaderName: "Al", LeaderID: 0x10000001, Members: 2},
		{LeaderName: "Bo", LeaderID: 0x10000002, Members: 3},
	}))
	want := []byte{
		0xfe, 0x30, 0x00,
		'A', 0, 'l', 0, 0, 0,
		0, 0, 0, 0, // channel loot
		5, 0, 0, 0, // members
		2, 0, 0, 0, // parties
		'A', 0, 'l', 0, 0, 0,
		0x01, 0x00, 0x00, 0x10,
		2, 0, 0, 0,
		'B', 0, 'o', 0, 0, 0,
		0x02, 0x00, 0x00, 0x10,
		3, 0, 0, 0,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("ExMultiPartyCommandChannelInfo = % x, want % x", got, want)
	}
}

// TestSystemMessageLooting pins the loot rule messages: 1031 finders keepers
// to 1035 by turn including spoil, in the client's loot rule order.
func TestSystemMessageLooting(t *testing.T) {
	for rule, want := range []int{1031, 1032, 1033, 1034, 1035} {
		if got := SystemMessageLooting(int32(rule)); got != want {
			t.Errorf("SystemMessageLooting(%d) = %d, want %d", rule, got, want)
		}
	}
}
