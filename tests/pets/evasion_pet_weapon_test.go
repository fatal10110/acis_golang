package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPetStrikeEvasionNeedsHeldWeapon pins the pet side of the
// physical-skill evasion gate: a pet holding a weapon item lets its target
// roll P_SKILL_EVASION, so a monster that evades every physical skill keeps
// its HP, while the same strike from a pet holding none lands.
func TestPetStrikeEvasionNeedsHeldWeapon(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		armed bool
	}{
		{name: "armed pet is evaded", armed: true},
		{name: "unarmed pet lands", armed: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			catalog := item.NewTable(append(gameservertest.ItemTemplates().All(), petWeaponTemplate()))
			h, wolf, hostile := bootWolfStrikerWith(t, wolfStrike(), gameservertest.WithItemTemplates(catalog))
			if tc.armed {
				runOnPetQueue(t, wolf, func() {
					inv := wolf.PetInventory()
					inst := inv.AddNew(fixturePetWeaponID, 1, 1_900_000)
					tmpl, _ := inv.Templates().Get(fixturePetWeaponID)
					if inst == nil || tmpl == nil {
						t.Error("pet weapon not added to the pet inventory")
						return
					}
					inv.SetPaperdollItem(itemcontainer.RHand, inst, tmpl)
				})
			}
			hostile.AddStatFuncs([]effect.Mod{{Stat: stat.PSkillEvasion, Op: effect.OpSet, Value: 100}})
			full := float64(hostile.MaxHP())

			startWolfStrike(t, h)
			h.srv.Advance(t, 2*time.Second)
			h.srv.Settle(t)
			if hit := hostile.HP() < full; hit == tc.armed {
				t.Fatalf("monster HP after the strike = %v of %v, want evaded %v", hostile.HP(), full, tc.armed)
			}
			drainFrames(t, h.client)
		})
	}
}
