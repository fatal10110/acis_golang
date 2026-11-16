package summon

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestSummonRandomDamageFollowsTheHeldWeapon pins the random damage
// multiplier of a summon's hit, 1 + Rnd.get(-r, r)/100: r is the random
// damage of the weapon a pet holds, and 5+sqrt(level) for a servitor or a pet
// holding none. The npc template's random damage takes no part. Every roll
// here is its lowest, so the multiplier is 1 - r/100.
func TestSummonRandomDamageFollowsTheHeldWeapon(t *testing.T) {
	const weaponID = 9001
	templates := item.NewTable([]*item.Template{{
		ID: weaponID, Name: "Wolf Weapon", Kind: item.KindWeapon, Slot: item.SlotWolf,
		Weapon: &item.WeaponDetail{Type: item.WeaponPet, RandomDamage: 10},
	}})
	armedInv := itemcontainer.NewPetInventory(10, templates)
	inst := armedInv.AddNew(weaponID, 1, 11)
	tmpl, _ := templates.Get(weaponID)
	armedInv.SetPaperdollItem(itemcontainer.RHand, inst, tmpl)

	for _, tc := range []struct {
		name   string
		actor  *Actor
		spread int
		mul    float64
	}{
		{"servitor", mustServitor(t, ServitorConfig{ObjectID: 1, Level: 60, Roll: zeroSummonRoll}), -1, 0.88},
		{"pet holding none", mustPet(t, PetConfig{ObjectID: 2, Level: 25, Inventory: itemcontainer.NewPetInventory(20, templates), Roll: zeroSummonRoll}), -1, 0.90},
		{"pet holding a weapon", mustPet(t, PetConfig{ObjectID: 3, Level: 60, Inventory: armedInv, Roll: zeroSummonRoll}), 10, 0.90},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.actor.RandomDamageSpread(); got != tc.spread {
				t.Fatalf("RandomDamageSpread() = %d, want %d", got, tc.spread)
			}
			if got := creature.RandomDamageMultiplier(tc.actor, modelskill.Definition{}); !closeSummonFloat(got, tc.mul) {
				t.Fatalf("RandomDamageMultiplier() = %v, want %v", got, tc.mul)
			}
		})
	}
}

// TestSummonPhysicalAttackRangeIsFortyForEverySummon pins a summon's
// physical attack range at its attack type's 40, servitor or pet alike.
func TestSummonPhysicalAttackRangeIsFortyForEverySummon(t *testing.T) {
	for name, a := range map[string]*Actor{
		"servitor": mustServitor(t, ServitorConfig{ObjectID: 1}),
		"pet":      mustPet(t, PetConfig{ObjectID: 2}),
	} {
		if got := a.PhysicalAttackRange(); got != 40 {
			t.Errorf("%s PhysicalAttackRange() = %d, want 40", name, got)
		}
	}
}
