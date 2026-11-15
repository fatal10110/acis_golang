package npcs

import (
	"strconv"
	"testing"
)

// TestHeroWeaponEventRefusedToANonHero: a weapon pick sent by a player who
// is no hero gives nothing, even when a page offered the pick.
func TestHeroWeaponEventRefusedToANonHero(t *testing.T) {
	t.Parallel()
	w, monument := bootMonument(t, 76, heroFeatureOptions(t)...)

	w.openAnyNpcPage(t)
	wantPage(t, w.bypass(t, npcCommand(monument, "Quest HeroWeapon")), monument, scriptPage(t, heroWeaponDir+"no_hero.htm", monument), 2)
	w.openAnyNpcPage(t)
	if frames := w.bypass(t, "Quest HeroWeapon "+strconv.Itoa(infinityBlade)); len(frames) != 0 {
		t.Fatalf("pick by a non-hero answered %x, want nothing", opcodes(frames))
	}
	if n := w.srv.PlayerItemCount(t, w.player, int32(infinityBlade)); n != 0 {
		t.Fatalf("weapons held = %d, want 0", n)
	}
}
