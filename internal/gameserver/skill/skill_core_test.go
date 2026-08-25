package skill

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// ---- from enchant_test.go ----
const enchantTestSkillID = 3

func TestEnchantEligible(t *testing.T) {
	tests := []struct {
		name      string
		classID   int
		charLevel int
		want      bool
	}{
		{"third class at 76", 88, 76, true},
		{"third class below 76", 88, 75, false},
		{"second class at 76", 2, 76, false},
		{"unknown class", 999, 80, false},
	}
	for _, tt := range tests {
		if got := EnchantEligible(tt.classID, tt.charLevel); got != tt.want {
			t.Errorf("%s: EnchantEligible(%d, %d) = %v, want %v", tt.name, tt.classID, tt.charLevel, got, tt.want)
		}
	}
}

// enchantTestTable returns a synthetic 81-level table whose level N
// requires N*1000 experience, simple enough to reason about in assertions.
func enchantTestTable(t *testing.T) *player.LevelTable {
	t.Helper()
	levels := make(map[int]player.Level, 81)
	for i := 1; i <= 81; i++ {
		levels[i] = player.Level{RequiredExpToLevelUp: int64(i) * 1000}
	}
	table, err := player.NewLevelTable(levels)
	if err != nil {
		t.Fatalf("NewLevelTable() error: %v", err)
	}
	return table
}

// enchantTestChar returns a third-class, level-76 character who already
// knows enchantTestSkillID at level 20 (the current max normal level) and
// has plenty of SP/exp to enchant it to level 101.
func enchantTestChar() *player.Character {
	ch := &player.Character{ID: 1, ClassID: 88, CharLevel: 76, SP: 1000, Exp: 500000}
	ch.SetSkillLevel(enchantTestSkillID, 20)
	return ch
}

func enchantTestTree() *modelskill.Trees {
	return &modelskill.Trees{Enchant: []modelskill.EnchantSkill{
		{ID: enchantTestSkillID, Level: 101, Exp: 300, SP: 50, Rate76: 60, Rate77: 60, Rate78: 60, Rate79: 60, Rate80: 60},
	}}
}

func enchantTestPersistence() *Persistence {
	return NewPersistence(nil, modelskill.NewTable([]modelskill.Definition{
		{ID: enchantTestSkillID, Level: 20},
		{ID: enchantTestSkillID, Level: 101},
	}))
}

func TestEnchantOfferForGates(t *testing.T) {
	trees := enchantTestTree()
	skills := enchantTestPersistence()

	if _, ok := EnchantOfferFor(nil, trees, skills, enchantTestSkillID, 101); ok {
		t.Fatal("nil character returned ok=true")
	}

	notEligible := enchantTestChar()
	notEligible.CharLevel = 70
	if _, ok := EnchantOfferFor(notEligible, trees, skills, enchantTestSkillID, 101); ok {
		t.Fatal("under-leveled character returned ok=true")
	}

	ch := enchantTestChar()
	offer, ok := EnchantOfferFor(ch, trees, skills, enchantTestSkillID, 101)
	if !ok {
		t.Fatal("EnchantOfferFor() returned ok=false")
	}
	if offer.Skill.Level != 101 || offer.Rate != 60 {
		t.Fatalf("offer = %+v, want level 101 rate 60", offer)
	}

	ch2 := enchantTestChar()
	ch2.SetSkillLevel(enchantTestSkillID, 101)
	if _, ok := EnchantOfferFor(ch2, trees, skills, enchantTestSkillID, 101); ok {
		t.Fatal("already-enchanted character returned ok=true")
	}
}

func TestEnchantSucceeds(t *testing.T) {
	ch := enchantTestChar()
	table := enchantTestTable(t)
	trees := enchantTestTree()
	skills := enchantTestPersistence()
	roll := func() int { return 0 }

	result, status, err := Enchant(context.Background(), ch, table, nil, trees, skills, false, roll, enchantTestSkillID, 101)
	if err != nil {
		t.Fatalf("Enchant() error: %v", err)
	}
	if status != EnchantSucceeded {
		t.Fatalf("status = %v, want EnchantSucceeded", status)
	}
	if result.AppliedLevel != 101 {
		t.Fatalf("AppliedLevel = %d, want 101", result.AppliedLevel)
	}
	if ch.SkillLevel(enchantTestSkillID) != 101 {
		t.Fatalf("skill level = %d, want 101", ch.SkillLevel(enchantTestSkillID))
	}
	if ch.SP != 950 {
		t.Fatalf("SP = %d, want 950", ch.SP)
	}
	if ch.Exp != 500000-300 {
		t.Fatalf("Exp = %d, want %d", ch.Exp, 500000-300)
	}
}

func TestEnchantFailsResetsToMaxNormalLevel(t *testing.T) {
	ch := enchantTestChar()
	table := enchantTestTable(t)
	trees := enchantTestTree()
	skills := enchantTestPersistence()
	roll := func() int { return 99 }

	result, status, err := Enchant(context.Background(), ch, table, nil, trees, skills, false, roll, enchantTestSkillID, 101)
	if err != nil {
		t.Fatalf("Enchant() error: %v", err)
	}
	if status != EnchantFailed {
		t.Fatalf("status = %v, want EnchantFailed", status)
	}
	if result.AppliedLevel != 20 {
		t.Fatalf("AppliedLevel = %d, want 20 (max normal level)", result.AppliedLevel)
	}
	if ch.SkillLevel(enchantTestSkillID) != 20 {
		t.Fatalf("skill level = %d, want 20", ch.SkillLevel(enchantTestSkillID))
	}
	// Exp/sp are still spent on a failed attempt.
	if ch.SP != 950 {
		t.Fatalf("SP = %d, want 950", ch.SP)
	}
}

func TestEnchantNeedsSP(t *testing.T) {
	ch := enchantTestChar()
	ch.SP = 10
	table := enchantTestTable(t)
	trees := enchantTestTree()
	skills := enchantTestPersistence()

	result, status, err := Enchant(context.Background(), ch, table, nil, trees, skills, false, func() int { return 0 }, enchantTestSkillID, 101)
	if err != nil {
		t.Fatalf("Enchant() error: %v", err)
	}
	if status != EnchantNeedsSP {
		t.Fatalf("status = %v, want EnchantNeedsSP", status)
	}
	if ch.SP != 10 {
		t.Fatalf("SP changed to %d, want unchanged 10", ch.SP)
	}
	if result.SP != 50 {
		t.Fatalf("result.SP = %d, want 50", result.SP)
	}
}

func TestEnchantNeedsExp(t *testing.T) {
	ch := enchantTestChar()
	table := enchantTestTable(t)
	// Just enough exp to remain at the level-76 floor after subtracting the
	// enchant's cost, minus one — the check must fail.
	ch.Exp = table.RequiredExpForLevel(76) + 299
	trees := enchantTestTree()
	skills := enchantTestPersistence()

	_, status, err := Enchant(context.Background(), ch, table, nil, trees, skills, false, func() int { return 0 }, enchantTestSkillID, 101)
	if err != nil {
		t.Fatalf("Enchant() error: %v", err)
	}
	if status != EnchantNeedsExp {
		t.Fatalf("status = %v, want EnchantNeedsExp", status)
	}
	if ch.SkillLevel(enchantTestSkillID) != 20 {
		t.Fatalf("skill level changed to %d, want unchanged 20", ch.SkillLevel(enchantTestSkillID))
	}
}

func TestEnchantMissingItemWhenSPBookNeeded(t *testing.T) {
	ch := enchantTestChar()
	table := enchantTestTable(t)
	trees := &modelskill.Trees{Enchant: []modelskill.EnchantSkill{
		{ID: enchantTestSkillID, Level: 101, Exp: 300, SP: 50, Rate76: 60, Rate77: 60, Rate78: 60, Rate79: 60, Rate80: 60, ItemID: 6622, ItemCount: 1},
	}}
	skills := enchantTestPersistence()

	result, status, err := Enchant(context.Background(), ch, table, nil, trees, skills, true, func() int { return 0 }, enchantTestSkillID, 101)
	if err != nil {
		t.Fatalf("Enchant() error: %v", err)
	}
	if status != EnchantMissingItem {
		t.Fatalf("status = %v, want EnchantMissingItem", status)
	}
	if ch.SP != 1000 {
		t.Fatalf("SP changed to %d, want unchanged 1000", ch.SP)
	}
	if result.SP != 50 {
		t.Fatalf("result.SP = %d, want 50", result.SP)
	}
}

func TestEnchantSkipsItemCheckWhenConfigDisabled(t *testing.T) {
	ch := enchantTestChar()
	ch.AttachRuntime(&player.Template{}, testInventory(ch.ID, item.AdenaID, 0))
	table := enchantTestTable(t)
	trees := &modelskill.Trees{Enchant: []modelskill.EnchantSkill{
		{ID: enchantTestSkillID, Level: 101, Exp: 300, SP: 50, Rate76: 60, Rate77: 60, Rate78: 60, Rate79: 60, Rate80: 60, ItemID: 6622, ItemCount: 1},
	}}
	skills := enchantTestPersistence()

	_, status, err := Enchant(context.Background(), ch, table, nil, trees, skills, false, func() int { return 0 }, enchantTestSkillID, 101)
	if err != nil {
		t.Fatalf("Enchant() error: %v", err)
	}
	if status != EnchantSucceeded {
		t.Fatalf("status = %v, want EnchantSucceeded (item check should be skipped)", status)
	}
}

// ---- from item_stats_test.go ----
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

// ---- from leveling_test.go ----
// recordingSkillLevelStore records every character_skills write and delete
// GiveSkills makes, so a test can tell a free level entitlement (memory
// only) from a correction to something the character really learned
// (persisted).
type recordingSkillLevelStore struct {
	known   player.SkillLevels
	written []writtenSkill
	deleted []int
}

type writtenSkill struct {
	skillID int
	level   int
}

func (s *recordingSkillLevelStore) ListKnownSkills(context.Context, int32, int32) (player.SkillLevels, error) {
	return s.known, nil
}

func (s *recordingSkillLevelStore) SetKnownSkill(_ context.Context, _ int32, _ int32, skillID, level int) error {
	s.written = append(s.written, writtenSkill{skillID: skillID, level: level})
	return nil
}

func (s *recordingSkillLevelStore) DeleteKnownSkill(_ context.Context, _ int32, _ int32, skillID int) error {
	s.deleted = append(s.deleted, skillID)
	return nil
}

func newLevelingPersistence(store *recordingSkillLevelStore) *Persistence {
	table := modelskill.NewTable([]modelskill.Definition{
		{ID: 3, Level: 1}, {ID: 3, Level: 2}, {ID: 3, Level: 3},
		{ID: 194, Level: 1},
		{ID: 239, Level: 1},
		{ID: 249, Level: 1}, {ID: 249, Level: 2},
	})
	return NewPersistence(nil, table, store)
}

// TestGiveSkillsGrantsFreeSkillsWithoutPersisting pins the free half of the
// refresh: the level's zero-cost grants land on the character, but leave no
// character_skills row behind, because the next level change re-derives them.
func TestGiveSkillsGrantsFreeSkillsWithoutPersisting(t *testing.T) {
	store := &recordingSkillLevelStore{}
	p := newLevelingPersistence(store)
	c := &player.Character{ID: 1, CharLevel: 10}
	tmpl := &player.Template{Skills: []player.SkillGrant{
		{SkillID: 249, Level: 1, MinLevel: 5, Cost: 0},
		{SkillID: 249, Level: 2, MinLevel: 10, Cost: 0},
		{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50},
	}}

	if err := p.GiveSkills(context.Background(), c, tmpl); err != nil {
		t.Fatalf("GiveSkills() error: %v", err)
	}
	if got := c.SkillLevel(249); got != 2 {
		t.Errorf("SkillLevel(249) = %d, want 2 (highest free grant the level unlocks)", got)
	}
	if got := c.SkillLevel(3); got != 0 {
		t.Errorf("SkillLevel(3) = %d, want 0 (a bought skill is never handed over)", got)
	}
	if len(store.written) != 0 || len(store.deleted) != 0 {
		t.Errorf("persisted writes = %v, deletes = %v; want none for free grants", store.written, store.deleted)
	}
}

func TestRewardSkillsGrantsAllAvailableSkillsWithSelectivePersistence(t *testing.T) {
	store := &recordingSkillLevelStore{}
	p := newLevelingPersistence(store)
	c := &player.Character{ID: 1, CharLevel: 10}
	tmpl := &player.Template{Skills: []player.SkillGrant{
		{SkillID: 249, Level: 1, MinLevel: 5, Cost: 0},
		{SkillID: 249, Level: 2, MinLevel: 10, Cost: 0},
		{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50},
	}}

	if err := p.RewardSkills(context.Background(), c, tmpl); err != nil {
		t.Fatalf("RewardSkills() error: %v", err)
	}
	if got := c.SkillLevel(249); got != 2 {
		t.Errorf("SkillLevel(249) = %d, want 2", got)
	}
	if got := c.SkillLevel(3); got != 1 {
		t.Errorf("SkillLevel(3) = %d, want 1", got)
	}
	if len(store.written) != 1 || store.written[0] != (writtenSkill{skillID: 3, level: 1}) {
		t.Errorf("persisted writes = %v, want [{3 1}]", store.written)
	}
}

// TestGiveSkillsDropsLuckyAtMaxLevel pins the newbie Lucky skill going away
// at level 10, without a persisted delete: it was never persisted either.
func TestGiveSkillsDropsLuckyAtMaxLevel(t *testing.T) {
	tmpl := &player.Template{Skills: []player.SkillGrant{
		{SkillID: 194, Level: 1, MinLevel: 1, Cost: 0},
	}}

	t.Run("below the cutoff it is granted", func(t *testing.T) {
		store := &recordingSkillLevelStore{}
		p := newLevelingPersistence(store)
		c := &player.Character{ID: 1, CharLevel: 9}
		if err := p.GiveSkills(context.Background(), c, tmpl); err != nil {
			t.Fatalf("GiveSkills() error: %v", err)
		}
		if got := c.SkillLevel(194); got != 1 {
			t.Errorf("SkillLevel(194) at level 9 = %d, want 1", got)
		}
	})

	t.Run("at the cutoff it is taken away", func(t *testing.T) {
		store := &recordingSkillLevelStore{}
		p := newLevelingPersistence(store)
		c := &player.Character{ID: 1, CharLevel: 10}
		c.SetSkillLevel(194, 1)
		if err := p.GiveSkills(context.Background(), c, tmpl); err != nil {
			t.Fatalf("GiveSkills() error: %v", err)
		}
		if got := c.SkillLevel(194); got != 0 {
			t.Errorf("SkillLevel(194) at level 10 = %d, want 0", got)
		}
		if len(store.deleted) != 0 {
			t.Errorf("persisted deletes = %v, want none", store.deleted)
		}
	})
}

// TestGiveSkillsCorrectsSkillsTheLevelNoLongerSupports pins the half of the
// refresh a level loss depends on, and that those corrections do reach
// character_skills.
func TestGiveSkillsCorrectsSkillsTheLevelNoLongerSupports(t *testing.T) {
	tmpl := &player.Template{Skills: []player.SkillGrant{
		{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50},
		{SkillID: 3, Level: 2, MinLevel: 20, Cost: 50},
		{SkillID: 3, Level: 3, MinLevel: 40, Cost: 50},
	}}

	t.Run("downgrades a skill held above the level's grant", func(t *testing.T) {
		store := &recordingSkillLevelStore{}
		p := newLevelingPersistence(store)
		c := &player.Character{ID: 1, CharLevel: 11}
		c.SetSkillLevel(3, 3)
		if err := p.GiveSkills(context.Background(), c, tmpl); err != nil {
			t.Fatalf("GiveSkills() error: %v", err)
		}
		// Level 11 reaches the MinLevel-20 grant through the nine-level
		// slack, but not the level-3 one at MinLevel 40.
		if got := c.SkillLevel(3); got != 2 {
			t.Errorf("SkillLevel(3) = %d, want 2", got)
		}
		if len(store.written) != 1 || store.written[0] != (writtenSkill{skillID: 3, level: 2}) {
			t.Errorf("persisted writes = %v, want [{3 2}]", store.written)
		}
	})

	t.Run("drops a skill the level cannot hold at all", func(t *testing.T) {
		store := &recordingSkillLevelStore{}
		p := newLevelingPersistence(store)
		c := &player.Character{ID: 1, CharLevel: 1}
		c.SetSkillLevel(3, 1)
		// Every grant for skill 3 here sits beyond level 1's nine-level
		// slack, so the character may not hold it at any level.
		highOnly := &player.Template{Skills: []player.SkillGrant{
			{SkillID: 3, Level: 2, MinLevel: 20, Cost: 50},
			{SkillID: 3, Level: 3, MinLevel: 40, Cost: 50},
		}}
		if err := p.GiveSkills(context.Background(), c, highOnly); err != nil {
			t.Fatalf("GiveSkills() error: %v", err)
		}
		if got := c.SkillLevel(3); got != 0 {
			t.Errorf("SkillLevel(3) = %d, want 0", got)
		}
		if len(store.deleted) != 1 || store.deleted[0] != 3 {
			t.Errorf("persisted deletes = %v, want [3]", store.deleted)
		}
	})

	t.Run("leaves skills the profession never grants alone", func(t *testing.T) {
		store := &recordingSkillLevelStore{}
		p := newLevelingPersistence(store)
		c := &player.Character{ID: 1, CharLevel: 1}
		// 4267 comes from equipment state, not the profession line.
		c.SetSkillLevel(4267, 1)
		if err := p.GiveSkills(context.Background(), c, tmpl); err != nil {
			t.Fatalf("GiveSkills() error: %v", err)
		}
		if got := c.SkillLevel(4267); got != 1 {
			t.Errorf("SkillLevel(4267) = %d, want 1 (untouched)", got)
		}
		if len(store.written) != 0 || len(store.deleted) != 0 {
			t.Errorf("persisted writes = %v, deletes = %v; want none", store.written, store.deleted)
		}
	})
}

// TestGiveSkillsKeepsEnchantOnlyAtHighLevel pins the enchanted-skill branch:
// a skill levelled past the top of its table keeps that enchant only from
// level 76 up, and only while the profession already grants it at full table
// level.
func TestGiveSkillsKeepsEnchantOnlyAtHighLevel(t *testing.T) {
	// Skill 3's table tops out at level 3; 101 is an enchant route level.
	tmpl := &player.Template{Skills: []player.SkillGrant{
		{SkillID: 3, Level: 3, MinLevel: 40, Cost: 50},
	}}

	t.Run("kept at 76 and above", func(t *testing.T) {
		store := &recordingSkillLevelStore{}
		p := newLevelingPersistence(store)
		c := &player.Character{ID: 1, CharLevel: 76}
		c.SetSkillLevel(3, 101)
		if err := p.GiveSkills(context.Background(), c, tmpl); err != nil {
			t.Fatalf("GiveSkills() error: %v", err)
		}
		if got := c.SkillLevel(3); got != 101 {
			t.Errorf("SkillLevel(3) at level 76 = %d, want 101", got)
		}
	})

	t.Run("pulled back below 76", func(t *testing.T) {
		store := &recordingSkillLevelStore{}
		p := newLevelingPersistence(store)
		c := &player.Character{ID: 1, CharLevel: 75}
		c.SetSkillLevel(3, 101)
		if err := p.GiveSkills(context.Background(), c, tmpl); err != nil {
			t.Fatalf("GiveSkills() error: %v", err)
		}
		if got := c.SkillLevel(3); got != 3 {
			t.Errorf("SkillLevel(3) at level 75 = %d, want 3", got)
		}
		if len(store.written) != 1 || store.written[0] != (writtenSkill{skillID: 3, level: 3}) {
			t.Errorf("persisted writes = %v, want [{3 3}]", store.written)
		}
	})
}

// ---- from test_fixtures_test.go ----
func testInventory(ownerID, itemID int32, count int) *itemcontainer.Inventory {
	templates := item.NewTable([]*item.Template{
		{ID: itemID, Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
	inv := itemcontainer.NewPlayerInventory(ownerID, templates)
	inv.AddNew(itemID, count, 100)
	return inv
}
