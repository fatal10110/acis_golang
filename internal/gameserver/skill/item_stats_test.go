package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
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
			Weapon: &item.WeaponDetail{
				Type:          item.WeaponSword,
				Enchant4Skill: &item.SkillRef{ID: 301, Level: 1},
			},
			Modifiers: []item.StatModifier{
				{Op: item.FuncAdd, Stat: "pAtk", Value: 20},
			},
			AttachedSkills: []item.SkillRef{{ID: 300, Level: 1}},
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
		{ID: 301, Level: 1, Activation: modelskill.ActivationPassive, Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAdd, Stat: "pAtk", Value: 40},
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

	if _, _, err := p.EquipItemStats(ch, inst, tmpl); err != nil {
		t.Fatalf("EquipItemStats() error: %v", err)
	}
	if got, want := ch.MAtk(), baseMAtk+5+8; got != want {
		t.Fatalf("MAtk() after equip = %v, want %v (modifier +5, attached passive +8)", got, want)
	}

	p.UnequipItemStats(ch, inv, inst, tmpl)
	if got := ch.MAtk(); got != baseMAtk {
		t.Fatalf("MAtk() after unequip = %v, want unchanged %v", got, baseMAtk)
	}
}

func TestEquipItemStatsWithholdsWeaponPassiveBelowExpertise(t *testing.T) {
	templates := itemStatsTestTemplates()
	inv := itemcontainer.NewPlayerInventory(1, templates)
	p := NewPersistence(nil, itemStatsTestSkills())
	ch := &player.Character{ID: 1}
	baseMAtk, basePAtk := ch.MAtk(), ch.PAtk()
	tmpl, _ := templates.Get(swordTemplateID)
	inst := inv.AddNew(swordTemplateID, 1, 100)
	inv.SetEnchantLevel(inst, 4)
	inv.EquipItem(inst, tmpl)

	if _, _, err := p.EquipItemStats(ch, inst, tmpl); err != nil {
		t.Fatalf("EquipItemStats() error: %v", err)
	}
	if got, want := ch.MAtk(), baseMAtk; got != want {
		t.Fatalf("MAtk() below Expertise = %v, want %v (weapon passive withheld)", got, want)
	}
	if got, want := ch.PAtk(), basePAtk+20; got != want {
		t.Fatalf("PAtk() below Expertise = %v, want %v (weapon modifier remains)", got, want)
	}

	ch.SetSkillLevel(239, int(item.CrystalD))
	if _, _, err := p.RefreshEquippedItemStats(ch, inv); err != nil {
		t.Fatalf("RefreshEquippedItemStats() error: %v", err)
	}
	if got, want := ch.MAtk(), baseMAtk+8; got != want {
		t.Fatalf("MAtk() at Expertise grade = %v, want %v (weapon passive restored)", got, want)
	}
	if got, want := ch.PAtk(), basePAtk+20+40; got != want {
		t.Fatalf("PAtk() at +4 and Expertise grade = %v, want %v (weapon +4 passive restored)", got, want)
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
	if _, _, err := p.EquipItemStats(ch, ring1, tmpl); err != nil {
		t.Fatalf("EquipItemStats(ring1) error: %v", err)
	}

	ring2 := inv.AddNew(ringTemplateID, 1, 101)
	inv.EquipItem(ring2, tmpl)
	if _, _, err := p.EquipItemStats(ch, ring2, tmpl); err != nil {
		t.Fatalf("EquipItemStats(ring2) error: %v", err)
	}
	afterBoth := ch.MAtk()
	if want := baseMAtk + 2*(5+8); afterBoth != want {
		t.Fatalf("MAtk() with both rings equipped = %v, want %v", afterBoth, want)
	}

	inv.UnequipSlot(inv.ItemByObjectID(ring1.ObjectID).Snapshot().LocationData)
	p.UnequipItemStats(ch, inv, ring1, tmpl)

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
	if _, _, err := p.EquipItemStats(ch, inst, tmpl); err != nil {
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

func TestEquipItemStatsEnchant4SkillReadsLiveEnchantLevel(t *testing.T) {
	templates := itemStatsTestTemplates()
	inv := itemcontainer.NewPlayerInventory(1, templates)
	p := NewPersistence(nil, itemStatsTestSkills())
	ch := &player.Character{ID: 1}
	ch.SetSkillLevel(239, int(item.CrystalD))
	basePAtk := ch.PAtk()
	tmpl, _ := templates.Get(swordTemplateID)

	inst := inv.AddNew(swordTemplateID, 1, 200)
	inv.SetEnchantLevel(inst, 3)
	inv.EquipItem(inst, tmpl)
	if _, _, err := p.EquipItemStats(ch, inst, tmpl); err != nil {
		t.Fatalf("EquipItemStats() error: %v", err)
	}
	if got, want := ch.PAtk(), basePAtk+20; got != want {
		t.Fatalf("PAtk() at +3 = %v, want %v without the +4 skill", got, want)
	}

	inv.SetEnchantLevel(inst, 4)
	if got, want := ch.PAtk(), basePAtk+20+40; got != want {
		t.Fatalf("PAtk() at +4 = %v, want %v with the +4 skill", got, want)
	}

	inv.SetEnchantLevel(inst, 3)
	if got, want := ch.PAtk(), basePAtk+20; got != want {
		t.Fatalf("PAtk() after dropping below +4 = %v, want %v without the +4 skill", got, want)
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

// TestEquipItemStatsGrantsNonPassiveAttachedSkillsAndArmsEquipDelay pins
// Gap 1 and Gap 2 of issue 1398 against the real shipped carriers named in
// the issue: 9184 (Teddy Bear Hat, grants ACTIVE skill 3264 "Blessed
// Escape" with a 30s equip delay), 9140 (Salvation Bow, grants ACTIVE skill
// 3261 "Forgiveness" with a 30s equip delay), and 6841 (The Lord's Crown,
// grants ACTIVE skill 3632 "Clan Gate" with no equip delay). Before this
// fix these skills never reached the player's known-skill set at all.
func TestEquipItemStatsGrantsNonPassiveAttachedSkillsAndArmsEquipDelay(t *testing.T) {
	cases := []struct {
		name       string
		itemID     int32
		skillID    int
		equipDelay int
		weapon     bool
	}{
		{"9184 Teddy Bear Hat / 3264 Blessed Escape", 9184, 3264, 30000, false},
		// 9140 Salvation Bow is a Weapon in the shipped datapack
		// (aCis_datapack/data/xml/items/9100-9199.xml:379, no crystal_type
		// set, so its grade defaults to CrystalNone and the Expertise gate
		// in EquipItemStats never withholds it); model it as a weapon here
		// so this case actually exercises the weapon branch of
		// EquipItemStats's gate, not just the armor branch.
		{"9140 Salvation Bow / 3261 Forgiveness", 9140, 3261, 30000, true},
		{"6841 The Lord's Crown / 3632 Clan Gate", 6841, 3632, 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpl := &item.Template{
				ID:             tc.itemID,
				Slot:           item.SlotChest,
				AttachedSkills: []item.SkillRef{{ID: int32(tc.skillID), Level: 1}},
			}
			if tc.weapon {
				tmpl.Kind = item.KindWeapon
				tmpl.Slot = item.SlotRHand
				tmpl.Weapon = &item.WeaponDetail{Type: item.WeaponBow}
			} else {
				tmpl.Kind = item.KindArmor
				tmpl.Armor = &item.ArmorDetail{Type: item.ArmorLight}
			}
			templates := item.NewTable([]*item.Template{tmpl})
			skills := modelskill.NewTable([]modelskill.Definition{
				{ID: modelskill.ID(tc.skillID), Level: 1, Activation: modelskill.ActivationActive, EquipDelay: tc.equipDelay},
			})
			inv := itemcontainer.NewPlayerInventory(1, templates)
			p := NewPersistence(nil, skills)
			ch := &player.Character{ID: 1}
			inst := inv.AddNew(tc.itemID, 1, 100)
			inv.EquipItem(inst, tmpl)

			skillsChanged, timersChanged, err := p.EquipItemStats(ch, inst, tmpl)
			if err != nil {
				t.Fatalf("EquipItemStats() error: %v", err)
			}
			if !skillsChanged {
				t.Fatal("skillsChanged = false, want true (SkillList must be resent)")
			}
			if !timersChanged {
				t.Fatal("timersChanged = false, want true (an ACTIVE item skill was granted)")
			}
			if got := ch.SkillLevel(tc.skillID); got != 1 {
				t.Fatalf("SkillLevel(%d) after equip = %d, want 1", tc.skillID, got)
			}

			def, _ := skills.Get(modelskill.ID(tc.skillID), 1)
			key := cast.ReuseKey(def)
			if got, want := ch.HasSkillReuse(key), tc.equipDelay > 0; got != want {
				t.Fatalf("HasSkillReuse() after equip = %v, want %v (equipDelay=%d)", got, want, tc.equipDelay)
			}

			inv.UnequipSlot(inv.ItemByObjectID(inst.ObjectID).Snapshot().LocationData)
			skillsChanged = p.UnequipItemStats(ch, inv, inst, tmpl)
			if !skillsChanged {
				t.Fatal("UnequipItemStats() skillsChanged = false, want true")
			}
			if got := ch.SkillLevel(tc.skillID); got != 0 {
				t.Fatalf("SkillLevel(%d) after unequip = %d, want 0 (revoked)", tc.skillID, got)
			}
			// The reuse timer armed at equip time is never cleared by
			// unequip in the reference.
			if !ch.HasSkillReuse(key) != (tc.equipDelay == 0) {
				t.Fatalf("HasSkillReuse() after unequip changed unexpectedly for equipDelay=%d", tc.equipDelay)
			}
		})
	}
}

// TestEquipItemStatsWithholdsNonPassiveAttachedSkillBelowWeaponExpertise pins
// the same Expertise gate for a granted (non-passive) AttachedSkills entry
// that TestEquipItemStatsWithholdsWeaponPassiveBelowExpertise already pins
// for a weapon's passive stat funcs: a weapon whose crystal grade exceeds
// the character's Expertise skips the whole item_skill loop in the
// reference (ItemPassiveSkillsListener.onEquip's grade-penalty early
// return), so the granted skill must not appear until Expertise catches up.
func TestEquipItemStatsWithholdsNonPassiveAttachedSkillBelowWeaponExpertise(t *testing.T) {
	const weaponTemplateID int32 = 50
	const grantedSkillID = 950
	tmpl := &item.Template{
		ID: weaponTemplateID, Kind: item.KindWeapon, Slot: item.SlotRHand,
		Crystal:        item.CrystalD,
		Weapon:         &item.WeaponDetail{Type: item.WeaponBow},
		AttachedSkills: []item.SkillRef{{ID: grantedSkillID, Level: 1}},
	}
	templates := item.NewTable([]*item.Template{tmpl})
	skills := modelskill.NewTable([]modelskill.Definition{
		{ID: grantedSkillID, Level: 1, Activation: modelskill.ActivationActive, EquipDelay: 30000},
	})
	inv := itemcontainer.NewPlayerInventory(1, templates)
	p := NewPersistence(nil, skills)
	ch := &player.Character{ID: 1}
	inst := inv.AddNew(weaponTemplateID, 1, 100)
	inv.EquipItem(inst, tmpl)

	skillsChanged, timersChanged, err := p.EquipItemStats(ch, inst, tmpl)
	if err != nil {
		t.Fatalf("EquipItemStats() error: %v", err)
	}
	if skillsChanged || timersChanged {
		t.Fatalf("skillsChanged=%v timersChanged=%v below Expertise, want both false (grant withheld)", skillsChanged, timersChanged)
	}
	if got := ch.SkillLevel(grantedSkillID); got != 0 {
		t.Fatalf("SkillLevel(%d) below Expertise = %d, want 0 (withheld)", grantedSkillID, got)
	}

	ch.SetSkillLevel(239, int(item.CrystalD))
	skillsChanged, timersChanged, err = p.RefreshEquippedItemStats(ch, inv)
	if err != nil {
		t.Fatalf("RefreshEquippedItemStats() error: %v", err)
	}
	if !skillsChanged || !timersChanged {
		t.Fatalf("skillsChanged=%v timersChanged=%v at Expertise grade, want both true (grant restored)", skillsChanged, timersChanged)
	}
	if got := ch.SkillLevel(grantedSkillID); got != 1 {
		t.Fatalf("SkillLevel(%d) at Expertise grade = %d, want 1 (restored)", grantedSkillID, got)
	}
}

// TestRefreshEquippedItemStatsRevokesNonPassiveAttachedSkillWhenGateIsLost
// pins the reverse direction of the Expertise gate: RefreshEquippedItemStats
// calls UnequipItemStats while inst is still in the paperdoll (unlike the
// real equip/unequip flow, which removes it first), so the shared-template
// scan must exclude inst itself — otherwise it always matches "itself" and
// never reaches the revoke loop, leaving a skill granted after the gate that
// granted it is lost.
func TestRefreshEquippedItemStatsRevokesNonPassiveAttachedSkillWhenGateIsLost(t *testing.T) {
	const weaponTemplateID int32 = 51
	const grantedSkillID = 951
	tmpl := &item.Template{
		ID: weaponTemplateID, Kind: item.KindWeapon, Slot: item.SlotRHand,
		Crystal:        item.CrystalD,
		Weapon:         &item.WeaponDetail{Type: item.WeaponBow},
		AttachedSkills: []item.SkillRef{{ID: grantedSkillID, Level: 1}},
	}
	templates := item.NewTable([]*item.Template{tmpl})
	skills := modelskill.NewTable([]modelskill.Definition{
		{ID: grantedSkillID, Level: 1, Activation: modelskill.ActivationActive, EquipDelay: 30000},
	})
	inv := itemcontainer.NewPlayerInventory(1, templates)
	p := NewPersistence(nil, skills)
	ch := &player.Character{ID: 1}
	ch.SetSkillLevel(239, int(item.CrystalD))
	inst := inv.AddNew(weaponTemplateID, 1, 100)
	inv.EquipItem(inst, tmpl)

	if _, _, err := p.EquipItemStats(ch, inst, tmpl); err != nil {
		t.Fatalf("EquipItemStats() error: %v", err)
	}
	if got := ch.SkillLevel(grantedSkillID); got != 1 {
		t.Fatalf("SkillLevel(%d) at Expertise grade = %d, want 1 (granted)", grantedSkillID, got)
	}

	// The character loses the Expertise level that allowed this weapon's
	// grant (e.g. a grade-penalty state change), so the refresh must revoke.
	ch.SetSkillLevel(239, 0)
	skillsChanged, _, err := p.RefreshEquippedItemStats(ch, inv)
	if err != nil {
		t.Fatalf("RefreshEquippedItemStats() error: %v", err)
	}
	if !skillsChanged {
		t.Fatal("RefreshEquippedItemStats() skillsChanged = false, want true (grant must be revoked)")
	}
	if got := ch.SkillLevel(grantedSkillID); got != 0 {
		t.Fatalf("SkillLevel(%d) after losing Expertise = %d, want 0 (revoked)", grantedSkillID, got)
	}
}

// TestUnequipItemStatsKeepsGrantWhileAnotherEquippedItemSharesTemplateID
// pins onUnequip's item-id-scoped sharing check (Java
// ItemPassiveSkillsListener.java:126-128): unequipping one instance of a
// dual-wielded item must not revoke the skill while a second instance of
// the same template is still equipped.
func TestUnequipItemStatsKeepsGrantWhileAnotherEquippedItemSharesTemplateID(t *testing.T) {
	const earringTemplateID int32 = 42
	tmpl := &item.Template{
		ID:             earringTemplateID,
		Kind:           item.KindArmor,
		Slot:           item.SlotLREar,
		Armor:          &item.ArmorDetail{Type: item.ArmorLight},
		AttachedSkills: []item.SkillRef{{ID: 900, Level: 1}},
	}
	templates := item.NewTable([]*item.Template{tmpl})
	skills := modelskill.NewTable([]modelskill.Definition{
		{ID: 900, Level: 1, Activation: modelskill.ActivationActive},
	})
	inv := itemcontainer.NewPlayerInventory(1, templates)
	p := NewPersistence(nil, skills)
	ch := &player.Character{ID: 1}

	earring1 := inv.AddNew(earringTemplateID, 1, 100)
	inv.EquipItem(earring1, tmpl)
	if _, _, err := p.EquipItemStats(ch, earring1, tmpl); err != nil {
		t.Fatalf("EquipItemStats(earring1) error: %v", err)
	}
	earring2 := inv.AddNew(earringTemplateID, 1, 101)
	inv.EquipItem(earring2, tmpl)
	if _, _, err := p.EquipItemStats(ch, earring2, tmpl); err != nil {
		t.Fatalf("EquipItemStats(earring2) error: %v", err)
	}
	if got := ch.SkillLevel(900); got != 1 {
		t.Fatalf("SkillLevel(900) with both earrings equipped = %d, want 1", got)
	}

	inv.UnequipSlot(inv.ItemByObjectID(earring1.ObjectID).Snapshot().LocationData)
	if changed := p.UnequipItemStats(ch, inv, earring1, tmpl); changed {
		t.Fatal("UnequipItemStats(earring1) skillsChanged = true, want false (earring2 still grants the skill)")
	}
	if got := ch.SkillLevel(900); got != 1 {
		t.Fatalf("SkillLevel(900) after unequipping earring1 only = %d, want 1 (earring2 still equipped)", got)
	}

	inv.UnequipSlot(inv.ItemByObjectID(earring2.ObjectID).Snapshot().LocationData)
	if changed := p.UnequipItemStats(ch, inv, earring2, tmpl); !changed {
		t.Fatal("UnequipItemStats(earring2) skillsChanged = false, want true (no more earring grants the skill)")
	}
	if got := ch.SkillLevel(900); got != 0 {
		t.Fatalf("SkillLevel(900) after unequipping both earrings = %d, want 0", got)
	}
}
