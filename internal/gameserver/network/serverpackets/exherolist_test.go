package serverpackets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// TestFrameCharInfoHeroFlag pins CharInfo's hero byte, right after the
// noble byte: 1 for a hero, 0 otherwise.
func TestFrameCharInfoHeroFlag(t *testing.T) {
	c := &player.Character{}
	c.SetName("Observed")
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
