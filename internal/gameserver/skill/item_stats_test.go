package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"testing"
)

const (
	ringTemplateID  int32 = 1
	swordTemplateID int32 = 2
	enchantArmorID  int32 = 3
)

func itemStatsTestTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{
			ID:   ringTemplateID,
			Kind: item.KindArmor,
			Slot: item.SlotLRFinger,
			Armor: &item.ArmorDetail{
				Type: item.ArmorLight,
			},
			Modifiers: []item.StatModifier{
				{Op: item.FuncAdd, Stat: "mAtk", Value: 5},
			},
			AttachedSkills: []item.SkillRef{{ID: 300, Level: 1}},
		},
		{
			ID:      swordTemplateID,
			Kind:    item.KindWeapon,
			Slot:    item.SlotRHand,
			Crystal: item.CrystalD,
			Weapon:  &item.WeaponDetail{Type: item.WeaponSword},
			Modifiers: []item.StatModifier{
				{Op: item.FuncAdd, Stat: "pAtk", Value: 20},
			},
		},
		{
			ID:   enchantArmorID,
			Kind: item.KindArmor,
			Slot: item.SlotChest,
			Armor: &item.ArmorDetail{
				Type: item.ArmorLight,
			},
			Modifiers: []item.StatModifier{
				{Op: item.FuncAdd, Stat: "pDef", Value: 10},
				{Op: item.FuncEnchant, Stat: "pDef", Value: 0},
			},
		},
	})
}

func itemStatsTestSkills() *modelskill.Table {
	return modelskill.NewTable([]modelskill.Definition{
		{ID: 300, Level: 1, Activation: modelskill.ActivationPassive, Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAdd, Stat: "mAtk", Value: 8},
		}},
	})
}

func TestEquipItemStatsAttachesModifiersAndAttachedSkillPassives(t *testing.T) {
	templates := itemStatsTestTemplates()
	inv := itemcontainer.NewPlayerInventory(1, templates)
	p := NewPersistence(nil, itemStatsTestSkills())
	ch := &player.Character{ID: 1}
	baseMAtk := ch.MAtk()

	inst := inv.AddNew(ringTemplateID, 1, 100)
	tmpl, _ := templates.Get(ringTemplateID)
	inv.EquipItem(inst, tmpl)

	if err := p.EquipItemStats(ch, inst, tmpl); err != nil {
		t.Fatalf("EquipItemStats() error: %v", err)
	}
	if got, want := ch.MAtk(), baseMAtk+5+8; got != want {
		t.Fatalf("MAtk() after equip = %v, want %v (modifier +5, attached passive +8)", got, want)
	}

	p.UnequipItemStats(ch, inst, tmpl)
	if got := ch.MAtk(); got != baseMAtk {
		t.Fatalf("MAtk() after unequip = %v, want unchanged %v", got, baseMAtk)
	}
}

func TestUnequipItemStatsOnlyRemovesTheUnequippedInstance(t *testing.T) {
	templates := itemStatsTestTemplates()
	inv := itemcontainer.NewPlayerInventory(1, templates)
	p := NewPersistence(nil, itemStatsTestSkills())
	ch := &player.Character{ID: 1}
	baseMAtk := ch.MAtk()
	tmpl, _ := templates.Get(ringTemplateID)

	ring1 := inv.AddNew(ringTemplateID, 1, 100)
	inv.EquipItem(ring1, tmpl)
	if err := p.EquipItemStats(ch, ring1, tmpl); err != nil {
		t.Fatalf("EquipItemStats(ring1) error: %v", err)
	}

	ring2 := inv.AddNew(ringTemplateID, 1, 101)
	inv.EquipItem(ring2, tmpl)
	if err := p.EquipItemStats(ch, ring2, tmpl); err != nil {
		t.Fatalf("EquipItemStats(ring2) error: %v", err)
	}
	afterBoth := ch.MAtk()
	if want := baseMAtk + 2*(5+8); afterBoth != want {
		t.Fatalf("MAtk() with both rings equipped = %v, want %v", afterBoth, want)
	}

	inv.UnequipSlot(inv.ItemByObjectID(ring1.ObjectID).Snapshot().LocationData)
	p.UnequipItemStats(ch, ring1, tmpl)

	if got, want := ch.MAtk(), baseMAtk+5+8; got != want {
		t.Fatalf("MAtk() after unequipping ring1 only = %v, want %v (ring2's own funcs must survive)", got, want)
	}
}

func TestEquipItemStatsEnchantFuncReadsLiveEnchantLevel(t *testing.T) {
	templates := itemStatsTestTemplates()
	inv := itemcontainer.NewPlayerInventory(1, templates)
	p := NewPersistence(nil, itemStatsTestSkills())
	ch := &player.Character{ID: 1}
	tmpl, _ := templates.Get(enchantArmorID)

	inst := inv.AddNew(enchantArmorID, 1, 200)
	inv.EquipItem(inst, tmpl)
	if err := p.EquipItemStats(ch, inst, tmpl); err != nil {
		t.Fatalf("EquipItemStats() error: %v", err)
	}
	unenchanted := ch.PDef()

	inv.SetEnchantLevel(inst, 2)
	afterEnchant := ch.PDef()
	if afterEnchant <= unenchanted {
		t.Fatalf("PDef() at +2 enchant = %v, want more than the unenchanted %v (the func must read the instance's live enchant level, not a value captured at attach time)", afterEnchant, unenchanted)
	}

	inv.SetEnchantLevel(inst, 5)
	if got := ch.PDef(); got <= afterEnchant {
		t.Fatalf("PDef() at +5 enchant = %v, want more than the +2 result %v", got, afterEnchant)
	}
}

func TestRestoreEquippedItemStatsReattachesOnRelogin(t *testing.T) {
	templates := itemStatsTestTemplates()
	inv := itemcontainer.NewPlayerInventory(1, templates)
	p := NewPersistence(nil, itemStatsTestSkills())
	ch := &player.Character{ID: 1}
	baseMAtk := ch.MAtk()
	tmpl, _ := templates.Get(ringTemplateID)

	inst := inv.AddNew(ringTemplateID, 1, 100)
	inv.EquipItem(inst, tmpl)

	if err := p.RestoreEquippedItemStats(ch, inv); err != nil {
		t.Fatalf("RestoreEquippedItemStats() error: %v", err)
	}
	if got, want := ch.MAtk(), baseMAtk+5+8; got != want {
		t.Fatalf("MAtk() after restore = %v, want %v", got, want)
	}
}
