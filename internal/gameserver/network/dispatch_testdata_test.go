package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func testTemplates(t *testing.T) *player.TemplateTable {
	t.Helper()
	tmpl := &player.Template{
		ID:        0,
		BaseLevel: 1,
		HPTable:   []float64{80},
		MPTable:   []float64{30},
		CPTable:   []float64{32},
		Spawns:    []location.Location{{X: 10, Y: 20, Z: 30}},
		RunSpeed:  120,
		WalkSpeed: 60,
		SwimSpeed: 50,
		Skills:    []player.SkillGrant{{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50}},
	}
	table, err := player.NewTemplateTable(map[int]*player.Template{0: tmpl})
	if err != nil {
		t.Fatalf("build template table: %v", err)
	}
	return table
}

func testItemTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{
			ID:          item.AdenaID,
			Name:        "Adena",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Stackable:   true,
			Dropable:    true,
			Tradable:    true,
			Destroyable: true,
			Depositable: true,
			EtcItem:     &item.EtcItemDetail{},
		},
		{
			ID:          20,
			Name:        "Potion",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Stackable:   true,
			Dropable:    true,
			Tradable:    true,
			Destroyable: true,
			Depositable: true,
			EtcItem:     &item.EtcItemDetail{Type: item.EtcItemPotion},
		},
		{
			ID:             1463,
			Name:           "Soulshot: No Grade",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Destroyable:    true,
			Crystal:        item.CrystalD,
			DefaultAction:  item.ActionSoulshot,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemShot, Handler: "SoulShots"},
			AttachedSkills: []item.SkillRef{{ID: 2154, Level: 1}},
		},
		{
			ID:             2509,
			Name:           "Spiritshot: No Grade",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Destroyable:    true,
			Crystal:        item.CrystalD,
			DefaultAction:  item.ActionSpiritshot,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemShot, Handler: "SpiritShots"},
			AttachedSkills: []item.SkillRef{{ID: 2155, Level: 1}},
		},
		{
			ID:             1464,
			Name:           "Soulshot: C Grade",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Destroyable:    true,
			Crystal:        item.CrystalC,
			DefaultAction:  item.ActionSoulshot,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemShot, Handler: "SoulShots"},
			AttachedSkills: []item.SkillRef{{ID: 2156, Level: 1}},
		},
		{
			ID:             6645,
			Name:           "Beast Soulshot",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Destroyable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemShot, Handler: "BeastSoulShots"},
			AttachedSkills: []item.SkillRef{{ID: 2033, Level: 1}},
		},
		{
			ID:             6646,
			Name:           "Beast Spiritshot",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Destroyable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemShot, Handler: "BeastSpiritShots"},
			AttachedSkills: []item.SkillRef{{ID: 2008, Level: 1}},
		},
		{
			ID:           30,
			Name:         "Sword",
			Kind:         item.KindWeapon,
			Slot:         item.SlotRHand,
			Duration:     -1,
			Crystal:      item.CrystalD,
			CrystalCount: 10,
			Dropable:     true,
			Tradable:     true,
			Destroyable:  true,
			Depositable:  true,
			Weapon:       &item.WeaponDetail{Type: item.WeaponSword, SoulshotCount: 1, SpiritshotCount: 1},
		},
		{
			ID:        item.CrystalD.ItemID(),
			Name:      "D-grade Crystal",
			Kind:      item.KindEtcItem,
			Duration:  -1,
			Stackable: true,
			EtcItem:   &item.EtcItemDetail{},
		},
		{
			ID:        955,
			Name:      "Scroll: Enchant Weapon (D)",
			Kind:      item.KindEtcItem,
			Duration:  -1,
			Stackable: true,
			EtcItem:   &item.EtcItemDetail{Type: item.EtcItemScrollEnchantWeapon, Handler: "EnchantScrolls"},
		},
		{
			ID:             1060,
			Name:           "Lesser Healing Potion",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Dropable:       true,
			Tradable:       true,
			Destroyable:    true,
			Depositable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemPotion, Handler: "ItemSkills", ReuseDelay: 10000, SharedReuseGroup: 8},
			AttachedSkills: []item.SkillRef{{ID: 2031, Level: 1}},
			UseConditions: []item.UseCondition{{
				Root:      item.Condition{Kind: "player", Attrs: map[string]string{"flying": "False"}},
				MessageID: int32(serverpackets.SystemMessageS1CannotBeUsed),
				AddName:   true,
			}},
		},
		{
			ID:             728,
			Name:           "Mana Potion",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Dropable:       true,
			Tradable:       true,
			Destroyable:    true,
			Depositable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemPotion, Handler: "ItemSkills", ReuseDelay: 2000, SharedReuseGroup: -1},
			AttachedSkills: []item.SkillRef{{ID: 2279, Level: 2}},
		},
		{
			ID:             736,
			Name:           "Scroll: Escape",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Dropable:       true,
			Tradable:       true,
			Destroyable:    true,
			Depositable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemScroll, Handler: "ItemSkills", SharedReuseGroup: -1},
			AttachedSkills: []item.SkillRef{{ID: 2013, Level: 1}},
		},
		{
			ID:             737,
			Name:           "Scroll: Escape (Shared Group)",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Dropable:       true,
			Tradable:       true,
			Destroyable:    true,
			Depositable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemScroll, Handler: "ItemSkills", SharedReuseGroup: 5, ReuseDelay: 9000},
			AttachedSkills: []item.SkillRef{{ID: 2013, Level: 1}},
		},
		{
			ID:          9001,
			Name:        "Quest Token",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Destroyable: true,
			EtcItem:     &item.EtcItemDetail{Type: item.EtcItemQuest},
		},
	})
}
