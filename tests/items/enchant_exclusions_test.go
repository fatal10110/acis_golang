package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// excludedWeapon is a crystal-D weapon template of the given id and type, so
// the enchant scroll's grade check passes and only the item's own exclusion
// can refuse it.
func excludedWeapon(id int32, name string, weaponType item.WeaponType, duration int32) *item.Template {
	return &item.Template{
		ID:           id,
		Name:         name,
		Kind:         item.KindWeapon,
		Slot:         item.SlotRHand,
		Duration:     duration,
		Crystal:      item.CrystalD,
		CrystalCount: 10,
		Dropable:     true,
		Tradable:     true,
		Destroyable:  true,
		Depositable:  true,
		Weapon:       &item.WeaponDetail{Type: weaponType, SoulshotCount: 1, SpiritshotCount: 1},
	}
}

// TestEnchantRefusesExcludedWeapons pins the enchantability exclusions: hero
// weapons (ids 6611-6621, 6842), shadow weapons (a finite duration), fishing
// rods and the traveler weapons (ids 7822-7831) cannot be enchanted. Each
// refusal answers InappropriateEnchantCondition then EnchantResult(CANCELLED),
// keeps the scroll and leaves the item's enchant level untouched.
func TestEnchantRefusesExcludedWeapons(t *testing.T) {
	t.Parallel()
	cases := []*item.Template{
		excludedWeapon(6611, "Infinity Blade", item.WeaponSword, -1),
		excludedWeapon(6842, "Hero item 6842", item.WeaponSword, -1),
		excludedWeapon(8821, "Shadow Item: Two Handed Sword", item.WeaponSword, 100),
		excludedWeapon(6529, "Baby Duck Rod", item.WeaponFishingRod, -1),
		excludedWeapon(7822, "Traveler's Mace", item.WeaponBlunt, -1),
		excludedWeapon(7831, "Traveler's Pike", item.WeaponPole, -1),
	}
	for _, tmpl := range cases {
		t.Run(tmpl.Name, func(t *testing.T) {
			t.Parallel()
			templates := item.NewTable(append(gameservertest.ItemTemplates().All(), tmpl))
			var excluded int32
			srv, objID, _, scroll := bootEnchanter(t, func() float64 { return 0.0 }, 0, false, func(t *testing.T, srv *gameservertest.Server, objID int32) {
				excluded = srv.GiveItem(t, objID, tmpl.ID, 1)
			}, gameservertest.WithItemTemplates(templates))
			c := srv.Client

			openEnchantSelection(t, c, scroll, 955)
			c.Send(encodeRequestEnchantItem(excluded))

			assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageInappropriateEnchantCondition)
			assertEnchantResult(t, c.Read(), serverpackets.EnchantResultCancelled)
			barrier(t, c)

			if inst := mustFindItem(t, srv, objID, scroll); inst.Count != 1 {
				t.Fatalf("scroll count after refused enchant = %d, want 1", inst.Count)
			}
			if inst := mustFindItem(t, srv, objID, excluded); inst.EnchantLevel != 0 {
				t.Fatalf("enchant level of template %d after refusal = %d, want 0", tmpl.ID, inst.EnchantLevel)
			}
		})
	}
}
