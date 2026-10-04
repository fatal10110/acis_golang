package serverpackets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// TestFrameCharInfoHeroAura pins CharInfo's hero byte for a game master
// shown with GMHeroAura: 1 though it is no hero.
func TestFrameCharInfoHeroAura(t *testing.T) {
	c := &player.Character{Name: "Master"}
	got := framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: c, Template: &player.Template{}, HeroAura: true}))
	if got[len(got)-38] != 1 {
		t.Fatalf("hero byte with the hero aura = %d, want 1", got[len(got)-38])
	}
}
