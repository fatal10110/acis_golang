package serverpackets

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// TestFrameExHeroList pins ExHeroList's bytes: FE 23 00, the count, then
// per hero its name, class, clan name and crest, alliance name and crest,
// and election count.
func TestFrameExHeroList(t *testing.T) {
	got := framePayload(t, FrameExHeroList([]HeroListEntry{
		{Name: "Ab", ClassID: 88, ClanName: "C", ClanCrest: 7, AllyName: "", AllyCrest: 0, Count: 3},
	}))
	var want []byte
	str := func(s string) {
		for _, u := range utf16.Encode([]rune(s)) {
			want = binary.LittleEndian.AppendUint16(want, u)
		}
		want = append(want, 0, 0)
	}
	num := func(n int32) { want = binary.LittleEndian.AppendUint32(want, uint32(n)) }
	want = append(want, OpcodeExtended, 0x23, 0x00)
	num(1)
	str("Ab")
	num(88)
	str("C")
	num(7)
	str("")
	num(0)
	num(3)
	if string(got) != string(want) {
		t.Fatalf("ExHeroList = % x, want % x", got, want)
	}
	if empty := framePayload(t, FrameExHeroList(nil)); string(empty) != string([]byte{OpcodeExtended, 0x23, 0, 0, 0, 0, 0}) {
		t.Fatalf("empty ExHeroList = % x", empty)
	}
}

// TestFrameCharInfoHeroFlag pins CharInfo's hero byte, right after the
// noble byte: 1 for a hero, 0 otherwise.
func TestFrameCharInfoHeroFlag(t *testing.T) {
	c := &player.Character{Name: "Observed"}
	hero := func() byte {
		got := framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: c, Template: &player.Template{}}))
		return got[len(got)-38]
	}
	if got := hero(); got != 0 {
		t.Fatalf("hero byte = %d, want 0", got)
	}
	c.SetHero(true)
	if got := hero(); got != 1 {
		t.Fatalf("hero byte of a hero = %d, want 1", got)
	}
}
