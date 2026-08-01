package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

func TestRefreshExpertisePenalty(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindWeapon, Slot: item.SlotRHand, Crystal: item.CrystalB, Weapon: &item.WeaponDetail{}},
		{ID: 2, Kind: item.KindArmor, Slot: item.SlotFullArmor, Crystal: item.CrystalC, Armor: &item.ArmorDetail{}},
		{ID: 3, Kind: item.KindArmor, Slot: item.SlotNeck, Crystal: item.CrystalC, Armor: &item.ArmorDetail{}},
		{ID: 4, Kind: item.KindEtcItem, Slot: item.SlotLHand, Crystal: item.CrystalS, EtcItem: &item.EtcItemDetail{Type: item.EtcItemArrow}},
	})
	inv := itemcontainer.NewPlayerInventory(1, templates)
	for _, id := range []int32{1, 2, 3, 4} {
		inst := inv.AddNew(id, 1, 100+id)
		tmpl, _ := templates.Get(id)
		inv.EquipItem(inst, tmpl)
	}

	c := &Character{ID: 1}
	c.AttachRuntime(nil, inv)
	c.SetSkillLevel(expertiseSkillID, 1)
	updates, refreshes := 0, 0
	c.SetGradePenaltyUpdater(func() { updates++ })
	c.SetItemStatsRefresher(func() { refreshes++ })

	c.RefreshExpertisePenalty()
	if got, want := c.ArmorGradePenalty(), 3; !c.WeaponGradePenalty() || got != want {
		t.Fatalf("penalty = weapon %v armor %d, want weapon true armor %d", c.WeaponGradePenalty(), got, want)
	}
	if got := c.SkillLevel(gradePenaltySkillID); got != 1 {
		t.Fatalf("grade penalty skill level = %d, want 1", got)
	}
	if updates != 1 || refreshes != 1 {
		t.Fatalf("hooks = updates %d refreshes %d, want 1 each", updates, refreshes)
	}

	// Unchanged state neither re-sends packets nor reattaches item passives.
	c.RefreshExpertisePenalty()
	if updates != 1 || refreshes != 1 {
		t.Fatalf("unchanged hooks = updates %d refreshes %d, want 1 each", updates, refreshes)
	}

	c.SetSkillLevel(expertiseSkillID, int(item.CrystalS))
	c.RefreshExpertisePenalty()
	if c.WeaponGradePenalty() || c.ArmorGradePenalty() != 0 || c.HasSkill(gradePenaltySkillID) {
		t.Fatalf("cleared penalty = weapon %v armor %d skill %v", c.WeaponGradePenalty(), c.ArmorGradePenalty(), c.HasSkill(gradePenaltySkillID))
	}
	if updates != 2 || refreshes != 2 {
		t.Fatalf("cleared hooks = updates %d refreshes %d, want 2 each", updates, refreshes)
	}
}
