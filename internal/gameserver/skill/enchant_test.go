package skill

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

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
