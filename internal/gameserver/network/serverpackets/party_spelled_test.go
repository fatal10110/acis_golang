package serverpackets

import (
	"bytes"
	"testing"
)

// TestFramePartySpelled pins PartySpelled against its reference layout
// (PartySpelled.writeImpl, SendablePacket.writeEffect(effect, false)):
// C 0xee, D type, D object id, D count, then per effect D skill id, H
// level, D duration / 1000 — Java integer division, so a permanent -1 ms
// reads 0 and a toggle gets no -1.
func TestFramePartySpelled(t *testing.T) {
	got := framePayload(t, FramePartySpelled(PartySpelledPet, 0x10203040, []AbnormalStatusEffect{
		{SkillID: 1040, Level: 3, DurationMillis: 15_999},
		{SkillID: 1068, Level: 2, DurationMillis: -1},
		{SkillID: 1001, Level: 4, DurationMillis: 30_000, Toggle: true},
	}))

	want := []byte{0xee, 1, 0, 0, 0, 0x40, 0x30, 0x20, 0x10, 3, 0, 0, 0}
	want = append(want, 0x10, 0x04, 0, 0, 3, 0, 15, 0, 0, 0)
	want = append(want, 0x2c, 0x04, 0, 0, 2, 0, 0, 0, 0, 0)
	want = append(want, 0xe9, 0x03, 0, 0, 4, 0, 30, 0, 0, 0)
	if !bytes.Equal(got, want) {
		t.Fatalf("FramePartySpelled() = %x, want %x", got, want)
	}

	empty := framePayload(t, FramePartySpelled(PartySpelledServitor, 7, nil))
	if want := []byte{0xee, 2, 0, 0, 0, 7, 0, 0, 0, 0, 0, 0, 0}; !bytes.Equal(empty, want) {
		t.Fatalf("empty FramePartySpelled() = %x, want %x", empty, want)
	}
}
