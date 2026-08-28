package skill

import (
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// ---- from buffer_test.go ----
func TestNewBufferSkillDefaultsLevelFromTable(t *testing.T) {
	entry, err := NewBufferSkill(1035, "Buffs", nil, 0, "desc", NewTable([]Definition{{ID: 1035, Level: 4}}))
	if err != nil {
		t.Fatalf("NewBufferSkill() error: %v", err)
	}
	if entry.Skill.ID != 1035 || entry.Skill.Level != 4 || entry.Price != 0 {
		t.Fatalf("NewBufferSkill() = %+v", entry)
	}
}

func TestNewBufferSkillExplicitLevel(t *testing.T) {
	level := 7
	entry, err := NewBufferSkill(1035, "Buffs", &level, 100, "desc", nil)
	if err != nil {
		t.Fatalf("NewBufferSkill() error: %v", err)
	}
	if entry.Skill.Level != 7 || entry.Price != 100 {
		t.Fatalf("NewBufferSkill() = %+v", entry)
	}
}

func TestNewBufferSkillMissingSkill(t *testing.T) {
	entry, err := NewBufferSkill(1035, "Buffs", nil, 0, "desc", NewTable(nil))
	if err != nil {
		t.Fatalf("NewBufferSkill() error: %v", err)
	}
	if entry.Skill.Level != 0 {
		t.Fatalf("NewBufferSkill() level = %d, want 0", entry.Skill.Level)
	}
}

func TestNewBufferTablePreservesCategoryOrder(t *testing.T) {
	table, err := NewBufferTable([]BufferSkill{
		{Skill: Ref{ID: 1035, Level: 4}, Category: "Buffs"},
		{Skill: Ref{ID: 271, Level: 1}, Category: "Dances"},
		{Skill: Ref{ID: 264, Level: 1}, Category: "Songs"},
	})
	if err != nil {
		t.Fatalf("NewBufferTable() error: %v", err)
	}

	got := table.Categories()
	want := []string{"Buffs", "Dances", "Songs"}
	if len(got) != len(want) {
		t.Fatalf("Categories() len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Categories()[%d] = %q, want %q (%v)", i, got[i], want[i], got)
		}
	}
}

// ---- from chance_test.go ----
func TestTriggerTypeString(t *testing.T) {
	for _, name := range triggerTypeStrings {
		trigger, ok := triggerTypeNames[name]
		if !ok {
			t.Fatalf("triggerTypeNames missing %q", name)
		}
		if got := trigger.String(); got != name {
			t.Fatalf("String() = %q, want %q", got, name)
		}
	}
}

func TestParseChanceConditionEmptyTypeIsNoCondition(t *testing.T) {
	cond, ok, err := ParseChanceCondition("", -1)
	if err != nil {
		t.Fatalf("ParseChanceCondition() error = %v, want nil", err)
	}
	if ok {
		t.Fatalf("ParseChanceCondition() ok = true, want false for an empty chanceType")
	}
	if cond != (ChanceCondition{}) {
		t.Fatalf("ParseChanceCondition() cond = %+v, want zero value", cond)
	}
}

func TestParseChanceConditionUnknownTypeIsAnError(t *testing.T) {
	if _, ok, err := ParseChanceCondition("BOGUS", 50); err == nil || ok {
		t.Fatalf("ParseChanceCondition(BOGUS) = ok %v err %v, want an error and ok=false", ok, err)
	}
}

func TestParseChanceConditionKnownType(t *testing.T) {
	cond, ok, err := ParseChanceCondition("ON_ATTACKED", 80)
	if err != nil || !ok {
		t.Fatalf("ParseChanceCondition() = %+v, %v, %v, want ok with no error", cond, ok, err)
	}
	if cond.Trigger != TriggerOnAttacked || cond.Chance != 80 {
		t.Fatalf("cond = %+v, want {TriggerOnAttacked 80}", cond)
	}
}

// TestChanceConditionFires reproduces the reference chance-condition roll:
// trigger must match, and a non-negative chance only fires below its own
// value out of a [0,100) roll, while a negative chance always fires once
// the trigger matches.
func TestChanceConditionFires(t *testing.T) {
	tests := []struct {
		name    string
		cond    ChanceCondition
		trigger TriggerType
		roll    int
		want    bool
	}{
		{"trigger mismatch never fires", ChanceCondition{Trigger: TriggerOnCrit, Chance: -1}, TriggerOnHit, 0, false},
		{"negative chance always fires on match", ChanceCondition{Trigger: TriggerOnHit, Chance: -1}, TriggerOnHit, 99, true},
		{"roll under chance fires", ChanceCondition{Trigger: TriggerOnHit, Chance: 50}, TriggerOnHit, 49, true},
		{"roll at chance does not fire", ChanceCondition{Trigger: TriggerOnHit, Chance: 50}, TriggerOnHit, 50, false},
		{"roll over chance does not fire", ChanceCondition{Trigger: TriggerOnHit, Chance: 50}, TriggerOnHit, 51, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cond.Fires(tt.trigger, tt.roll); got != tt.want {
				t.Fatalf("Fires(%v, %d) = %v, want %v", tt.trigger, tt.roll, got, tt.want)
			}
		})
	}
}

// ---- from definition_test.go ----
// minimalAttrs returns the attributes a level carries when its data sets
// only the classification tags every skill has.
func minimalAttrs() DefinitionAttrs {
	return DefinitionAttrs{
		Activation:            ActivationActive,
		Target:                TargetOne,
		SkillType:             "BUFF",
		EffectRange:           -1,
		AbnormalLevel:         -1,
		EffectAbnormalLevel:   -1,
		NegateLevel:           -1,
		Radius:                80,
		EffectNpcID:           -1,
		Element:               ElementNone,
		CubicActivationTime:   8,
		CubicActivationChance: 30,
		SummonTotalLifeTime:   1200000,
		ActivationChance:      -1,
		CanBeReflected:        true,
		CanBeDispelled:        true,
	}
}

func TestNewDefinitionCarriesAttrs(t *testing.T) {
	d := NewDefinition(7, 1, "Test Skill", minimalAttrs())
	if d.ID != 7 || d.Level != 1 || d.Name != "Test Skill" {
		t.Fatalf("NewDefinition() identity = %+v", d)
	}
	if d.Target != TargetOne || d.SkillType != "BUFF" || d.Activation != ActivationActive {
		t.Fatalf("NewDefinition() tags = %+v", d)
	}
	if d.EffectRange != -1 || d.AbnormalLevel != -1 || d.NegateLevel != -1 {
		t.Fatalf("NewDefinition() (-1)-carrying fields = %+v", d)
	}
	if d.Radius != 80 {
		t.Fatalf("NewDefinition() Radius = %d, want 80", d.Radius)
	}
	if d.Element != ElementNone {
		t.Fatalf("NewDefinition() Element = %v, want ElementNone", d.Element)
	}
	if !d.CanBeReflected || !d.CanBeDispelled {
		t.Fatalf("NewDefinition() reflect/dispel = %+v", d)
	}
	// BUFF isn't a classified-offensive type, and target isn't CORPSE_MOB.
	if d.Offensive {
		t.Fatal("NewDefinition() Offensive = true, want false")
	}
	// Not PDAM/BLOW.
	if d.BaseCritRate != -1 {
		t.Fatalf("NewDefinition() BaseCritRate = %d, want -1", d.BaseCritRate)
	}
	if d.Flight != nil {
		t.Fatalf("NewDefinition() Flight = %v, want nil", d.Flight)
	}
	if d.SharedReuse != nil {
		t.Fatalf("NewDefinition() SharedReuse = %v, want nil", d.SharedReuse)
	}
}

func TestNewDefinitionOffensiveAndCritDefaults(t *testing.T) {
	a := minimalAttrs()
	a.SkillType = "PDAM"
	d := NewDefinition(1, 1, "x", a)
	if !d.Offensive {
		t.Fatal("PDAM: Offensive = false, want true")
	}
	if d.BaseCritRate != 0 {
		t.Fatalf("PDAM: BaseCritRate = %d, want 0", d.BaseCritRate)
	}

	// A debuff or a corpse-mob target is offensive whatever its skill type.
	a = minimalAttrs()
	a.Debuff = true
	if !NewDefinition(1, 1, "x", a).Offensive {
		t.Fatal("debuff: Offensive = false, want true")
	}
	a = minimalAttrs()
	a.Target = TargetCorpseMob
	if !NewDefinition(1, 1, "x", a).Offensive {
		t.Fatal("CORPSE_MOB target: Offensive = false, want true")
	}
}

func TestNewDefinitionExplicitOverridesDefault(t *testing.T) {
	a := minimalAttrs()
	a.SkillType = "PDAM"
	offensive, rate := false, 42
	a.Offensive, a.BaseCritRate = &offensive, &rate

	d := NewDefinition(1, 1, "x", a)
	if d.Offensive {
		t.Fatal("explicit offensive=false was overridden by the PDAM default")
	}
	if d.BaseCritRate != 42 {
		t.Fatalf("BaseCritRate = %d, want 42 (explicit)", d.BaseCritRate)
	}
}

func TestNewDefinitionHeroSkill(t *testing.T) {
	if !NewDefinition(395, 1, "Hero Skill", minimalAttrs()).HeroSkill {
		t.Fatal("skill 395: HeroSkill = false, want true")
	}
	if NewDefinition(1, 1, "Not Hero", minimalAttrs()).HeroSkill {
		t.Fatal("skill 1: HeroSkill = true, want false")
	}
}

func TestParseRef(t *testing.T) {
	ref, err := ParseRef("10-2")
	if err != nil {
		t.Fatalf("ParseRef(\"10-2\") error: %v", err)
	}
	if ref.ID != 10 || ref.Level != 2 {
		t.Fatalf("ParseRef(\"10-2\") = %+v, want {10 2}", ref)
	}
	for _, raw := range []string{"", "10", "not-a-pair-of-ints", "10-2-3", "x-2", "10-x", "100--1", "-1-2"} {
		if _, err := ParseRef(raw); err == nil {
			t.Fatalf("ParseRef(%q) = nil error, want an error", raw)
		}
	}
}

func TestParseEnums(t *testing.T) {
	if a, err := ParseActivation("TOGGLE"); err != nil || a != ActivationToggle {
		t.Fatalf("ParseActivation(\"TOGGLE\") = %v, %v", a, err)
	}
	if tgt, err := ParseTarget("CORPSE_MOB"); err != nil || tgt != TargetCorpseMob {
		t.Fatalf("ParseTarget(\"CORPSE_MOB\") = %v, %v", tgt, err)
	}
	if e, err := ParseElement("VALAKAS"); err != nil || e != ElementValakas {
		t.Fatalf("ParseElement(\"VALAKAS\") = %v, %v", e, err)
	}
	if f, err := ParseFlight("CHARGE"); err != nil || f != FlightCharge {
		t.Fatalf("ParseFlight(\"CHARGE\") = %v, %v", f, err)
	}
	if _, err := ParseTarget("NOT_A_TARGET"); err == nil {
		t.Fatal("ParseTarget(\"NOT_A_TARGET\") = nil error, want an error")
	}
}

// ---- from enum_test.go ----
func TestActivationFromStatSet(t *testing.T) {
	set := commons.NewStatSet()
	set.Set("operateType", "TOGGLE")
	got, err := commons.GetEnum[Activation](set, "operateType", activationNames)
	if err != nil || got != ActivationToggle {
		t.Fatalf("Activation = %v, %v, want ActivationToggle", got, err)
	}
	if got.String() != "TOGGLE" {
		t.Fatalf("String() = %q, want TOGGLE", got.String())
	}

	set.Set("operateType", "BOGUS")
	if _, err := commons.GetEnum[Activation](set, "operateType", activationNames); err == nil {
		t.Fatal("expected an error for an unknown operateType tag, got nil")
	}
}

func TestTargetFromStatSet(t *testing.T) {
	for _, name := range targetStrings {
		set := commons.NewStatSet()
		set.Set("target", name)
		got, err := commons.GetEnum[Target](set, "target", targetNames)
		if err != nil {
			t.Fatalf("target %q: %v", name, err)
		}
		if got.String() != name {
			t.Fatalf("target %q round-trip = %q", name, got.String())
		}
	}
}

func TestElementDefault(t *testing.T) {
	set := commons.NewStatSet()
	got, err := commons.GetEnumDefault[Element](set, "element", elementNames, ElementNone)
	if err != nil || got != ElementNone {
		t.Fatalf("Element default = %v, %v, want ElementNone", got, err)
	}

	set.Set("element", "FIRE")
	got, err = commons.GetEnumDefault[Element](set, "element", elementNames, ElementNone)
	if err != nil || got != ElementFire {
		t.Fatalf("Element = %v, %v, want ElementFire", got, err)
	}
}

func TestUnknownEnumValueStringsFallBack(t *testing.T) {
	if got := Activation(99).String(); got != "Activation(99)" {
		t.Fatalf("Activation(99).String() = %q", got)
	}
	if got := Target(99).String(); got != "Target(99)" {
		t.Fatalf("Target(99).String() = %q", got)
	}
	if got := Element(99).String(); got != "Element(99)" {
		t.Fatalf("Element(99).String() = %q", got)
	}
	if got := Flight(99).String(); got != "Flight(99)" {
		t.Fatalf("Flight(99).String() = %q", got)
	}
}

// ---- from extractable_test.go ----
func TestParseExtractableItems(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []ExtractableProduct
	}{
		{"empty", "", nil},
		{
			"single pair",
			"57,10,20.5",
			[]ExtractableProduct{{Items: []ExtractableItem{{ItemID: 57, Quantity: 10}}, Chance: 20.5}},
		},
		{
			"two groups, second with two pairs",
			"57,10,20.5;1234,1,5678,2,79.5",
			[]ExtractableProduct{
				{Items: []ExtractableItem{{ItemID: 57, Quantity: 10}}, Chance: 20.5},
				{Items: []ExtractableItem{{ItemID: 1234, Quantity: 1}, {ItemID: 5678, Quantity: 2}}, Chance: 79.5},
			},
		},
		{
			"malformed group is skipped, well-formed kept",
			"not-a-number,10,20.5;57,10,20.5",
			[]ExtractableProduct{{Items: []ExtractableItem{{ItemID: 57, Quantity: 10}}, Chance: 20.5}},
		},
		{
			"even field count is skipped",
			"57,10;57,10,20.5",
			[]ExtractableProduct{{Items: []ExtractableItem{{ItemID: 57, Quantity: 10}}, Chance: 20.5}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseExtractableItems(tt.raw)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseExtractableItems(%q) = %#v, want %#v", tt.raw, got, tt.want)
			}
		})
	}
}

// ---- from healsps_test.go ----
func TestNewHealSps(t *testing.T) {
	magicLevel := 76
	got, err := NewHealSps(292, 900, nil, nil, &magicLevel)
	if err != nil {
		t.Fatalf("NewHealSps() error: %v", err)
	}
	if got.MagicLevel != 76 || got.Correction != 292 || got.NeededMAtk != 900 {
		t.Fatalf("NewHealSps() = %+v", got)
	}

	if _, err := NewHealSps(17, 0, nil, nil, nil); err == nil {
		t.Fatal("expected an error for missing skillId/magicLevel selectors, got nil")
	}

	skillID := int32(1401)
	if _, err := NewHealSps(17, 0, &skillID, nil, nil); err == nil {
		t.Fatal("expected an error for skillId without skillLevel, got nil")
	}
}

func TestHealSpsTableCalculate(t *testing.T) {
	table, err := NewHealSpsTable([]HealSps{
		{MagicLevel: 74, Correction: 281, NeededMAtk: 850},
		{SkillID: 1401, SkillLevel: 11, Correction: 286, NeededMAtk: 875},
		{MagicLevel: 76, Correction: 292, NeededMAtk: 900},
	})
	if err != nil {
		t.Fatalf("NewHealSpsTable() error: %v", err)
	}

	if got := table.Calculate(1401, 11, 76, 875); got != 286 {
		t.Fatalf("Calculate(skill match) = %v, want 286", got)
	}
	if got := table.Calculate(2000, 1, 76, 890); got != 287 {
		t.Fatalf("Calculate(magic level fallback) = %v, want 287", got)
	}
	if got := table.Calculate(2000, 1, 1, 1); got != 0 {
		t.Fatalf("Calculate(no match) = %v, want 0", got)
	}
}

// ---- from newbiebuff_test.go ----
func TestNewNewbieBuff(t *testing.T) {
	got := NewNewbieBuff(4322, 1, 8, 24, false)
	if got.Skill.ID != 4322 || got.Skill.Level != 1 || got.LowerLevel != 8 || got.UpperLevel != 24 || got.IsMagicClass {
		t.Fatalf("NewNewbieBuff() = %+v", got)
	}
}

func TestNewbieBuffTableQueries(t *testing.T) {
	table := NewNewbieBuffTable([]NewbieBuff{
		{Skill: Ref{ID: 4322, Level: 1}, LowerLevel: 8, UpperLevel: 24, IsMagicClass: false},
		{Skill: Ref{ID: 4323, Level: 1}, LowerLevel: 11, UpperLevel: 23, IsMagicClass: false},
		{Skill: Ref{ID: 4322, Level: 1}, LowerLevel: 8, UpperLevel: 24, IsMagicClass: true},
	})

	if got := table.LowestBuffLevel(false); got != 8 {
		t.Fatalf("LowestBuffLevel(false) = %d, want 8", got)
	}
	if got := table.LowestBuffLevel(true); got != 8 {
		t.Fatalf("LowestBuffLevel(true) = %d, want 8", got)
	}

	phys := table.ValidBuffs(false, 12)
	if len(phys) != 2 {
		t.Fatalf("len(ValidBuffs(false, 12)) = %d, want 2", len(phys))
	}
	mage := table.ValidBuffs(true, 12)
	if len(mage) != 1 || mage[0].Skill.ID != 4322 {
		t.Fatalf("ValidBuffs(true, 12) = %+v", mage)
	}
}

// ---- from spellbook_test.go ----
func TestNewSpellbook(t *testing.T) {
	got := NewSpellbook(2, 1512)
	want := Spellbook{SkillID: 2, ItemID: 1512}
	if got != want {
		t.Fatalf("NewSpellbook() = %+v, want %+v", got, want)
	}
}

func TestSpellbookTableBookForSkill(t *testing.T) {
	table, err := NewSpellbookTable([]Spellbook{{SkillID: 2, ItemID: 1512}})
	if err != nil {
		t.Fatalf("NewSpellbookTable() error: %v", err)
	}

	if got := table.BookForSkill(2, 1, true, true); got != 1512 {
		t.Fatalf("BookForSkill(2, 1, true, true) = %d, want 1512", got)
	}
	if got := table.BookForSkill(2, 2, true, true); got != 0 {
		t.Fatalf("BookForSkill(2, 2, true, true) = %d, want 0", got)
	}
	if got := table.BookForSkill(DivineInspirationSkillID, 3, true, true); got != 8620 {
		t.Fatalf("BookForSkill(divine inspiration, 3, true, true) = %d, want 8620", got)
	}
	if got := table.BookForSkill(DivineInspirationSkillID, 3, true, false); got != 0 {
		t.Fatalf("BookForSkill(divine inspiration, 3, true, false) = %d, want 0", got)
	}
	if got := table.BookForSkill(2, 1, false, true); got != 0 {
		t.Fatalf("BookForSkill(2, 1, false, true) = %d, want 0", got)
	}
}

// ---- from table_test.go ----
func TestTable(t *testing.T) {
	table := NewTable([]Definition{
		{ID: 1, Level: 1},
		{ID: 1, Level: 2},
		{ID: 1, Level: 101}, // enchant level, excluded from MaxLevel
		{ID: 2, Level: 1},
	})

	if got := table.Len(); got != 4 {
		t.Fatalf("Len() = %d, want 4", got)
	}

	if d, ok := table.Get(1, 2); !ok || d.Level != 2 {
		t.Fatalf("Get(1, 2) = %+v, %v", d, ok)
	}
	if _, ok := table.Get(1, 3); ok {
		t.Fatal("Get(1, 3) ok = true, want false")
	}
	if _, ok := table.Get(99, 1); ok {
		t.Fatal("Get(99, 1) ok = true, want false")
	}

	if got := table.MaxLevel(1); got != 2 {
		t.Fatalf("MaxLevel(1) = %d, want 2 (enchant level 101 excluded)", got)
	}
	if got := table.MaxLevel(99); got != 0 {
		t.Fatalf("MaxLevel(99) = %d, want 0 for an unloaded id", got)
	}
}

func TestNewTableLaterEntryOverwrites(t *testing.T) {
	table := NewTable([]Definition{
		{ID: 1, Level: 1, Name: "first"},
		{ID: 1, Level: 1, Name: "second"},
	})
	if got := table.Len(); got != 1 {
		t.Fatalf("Len() = %d, want 1", got)
	}
	d, ok := table.Get(1, 1)
	if !ok || d.Name != "second" {
		t.Fatalf("Get(1, 1) = %+v, %v, want Name=second", d, ok)
	}
}

// ---- from tree_test.go ----
func TestNewFishingSkill(t *testing.T) {
	got := NewFishingSkill(1312, 1, 1, 57, 1000, false)
	want := FishingSkill{ID: 1312, Level: 1, MinLevel: 1, ItemID: 57, ItemCount: 1000, Dwarven: false}
	if got != want {
		t.Fatalf("NewFishingSkill() = %+v, want %+v", got, want)
	}

	if got := NewFishingSkill(1312, 1, 1, 57, 1000, true); !got.Dwarven {
		t.Fatal("NewFishingSkill() Dwarven = false, want true")
	}
}

func TestNewClanSkill(t *testing.T) {
	got := NewClanSkill(370, 1, 5, 500, 8166)
	want := ClanSkill{ID: 370, Level: 1, MinLevel: 5, Cost: 500, ItemID: 8166}
	if got != want {
		t.Fatalf("NewClanSkill() = %+v, want %+v", got, want)
	}
}

func TestNewEnchantSkill(t *testing.T) {
	got := NewEnchantSkill(1, 101, 5500000, 550000, 82, 92, 97, 100, 100, 6622, 1)
	want := EnchantSkill{
		ID: 1, Level: 101, Exp: 5500000, SP: 550000,
		Rate76: 82, Rate77: 92, Rate78: 97, Rate79: 100, Rate80: 100,
		ItemID: 6622, ItemCount: 1,
	}
	if got != want {
		t.Fatalf("NewEnchantSkill() = %+v, want %+v", got, want)
	}

	t.Run("no item requirement", func(t *testing.T) {
		got := NewEnchantSkill(1, 101, 5500000, 550000, 82, 92, 97, 100, 100, 0, 0)
		if got.ItemID != 0 || got.ItemCount != 0 {
			t.Fatalf("NewEnchantSkill() item = %d/%d, want 0/0", got.ItemID, got.ItemCount)
		}
	})
}

func TestFishingSkillQueries(t *testing.T) {
	trees := &Trees{Fishing: []FishingSkill{
		{ID: 1312, Level: 1, MinLevel: 1, ItemID: 57, ItemCount: 1000},
		{ID: 1313, Level: 1, MinLevel: 1, ItemID: 57, ItemCount: 10},
		{ID: 1313, Level: 2, MinLevel: 4, ItemID: 57, ItemCount: 50},
		{ID: 1368, Level: 1, MinLevel: 1, ItemID: 57, ItemCount: 100, Dwarven: true},
	}}

	available := trees.FishingSkillsFor(1, false, SkillLevels{1312: 0, 1313: 0, 1368: 0})
	want := []FishingSkill{
		{ID: 1312, Level: 1, MinLevel: 1, ItemID: 57, ItemCount: 1000},
		{ID: 1313, Level: 1, MinLevel: 1, ItemID: 57, ItemCount: 10},
	}
	if !equalFishingSkills(available, want) {
		t.Fatalf("FishingSkillsFor(level 1, non-dwarf) = %+v, want %+v", available, want)
	}

	if _, ok := trees.FishingSkillFor(1, false, SkillLevels{}, 1368, 1); ok {
		t.Fatal("FishingSkillFor(dwarven, non-dwarf) found a skill")
	}

	grant, ok := trees.FishingSkillFor(4, true, SkillLevels{1313: 1}, 1313, 2)
	if !ok || grant.Level != 2 || grant.ItemCount != 50 {
		t.Fatalf("FishingSkillFor(level 4, known 1313:1) = %+v, %v; want level 2", grant, ok)
	}

	if _, ok := trees.FishingSkillFor(4, true, SkillLevels{1313: 0}, 1313, 2); ok {
		t.Fatal("FishingSkillFor(skipped previous level) found a skill")
	}

	if got := trees.RequiredLevelForNextFishingSkill(1, false); got != 4 {
		t.Fatalf("RequiredLevelForNextFishingSkill(level 1, non-dwarf) = %d, want 4", got)
	}
}

func TestClanSkillQueries(t *testing.T) {
	trees := &Trees{Clan: []ClanSkill{
		{ID: 370, Level: 1, MinLevel: 5, Cost: 500, ItemID: 8166},
		{ID: 370, Level: 2, MinLevel: 5, Cost: 500, ItemID: 8166},
		{ID: 371, Level: 1, MinLevel: 6, Cost: 800, ItemID: 8169},
	}}

	available := trees.ClanSkillsFor(5, SkillLevels{})
	want := []ClanSkill{{ID: 370, Level: 1, MinLevel: 5, Cost: 500, ItemID: 8166}}
	if !equalClanSkills(available, want) {
		t.Fatalf("ClanSkillsFor(level 5, none known) = %+v, want %+v", available, want)
	}

	grant, status := trees.CheckClanSkillLearn(5, 499, SkillLevels{}, 370, 1)
	if status != LearnNeedsCost || grant.Cost != 500 {
		t.Fatalf("CheckClanSkillLearn(not enough reputation) = %+v, %v; want cost 500 and LearnNeedsCost", grant, status)
	}

	grant, status = trees.CheckClanSkillLearn(5, 500, SkillLevels{}, 370, 1)
	if status != LearnAllowed || grant.ID != 370 || grant.Level != 1 {
		t.Fatalf("CheckClanSkillLearn(enough reputation) = %+v, %v; want skill 370 level 1 and LearnAllowed", grant, status)
	}

	if _, status = trees.CheckClanSkillLearn(5, 500, SkillLevels{370: 0}, 370, 2); status != LearnUnavailable {
		t.Fatalf("CheckClanSkillLearn(skipped previous level) = %v, want LearnUnavailable", status)
	}
}

func TestEnchantSkillQueries(t *testing.T) {
	defs := NewTable([]Definition{
		{ID: 1, Level: 1},
		{ID: 1, Level: 2},
		{ID: 1, Level: 101},
		{ID: 1, Level: 141},
		{ID: 2, Level: 1},
	})
	trees := &Trees{Enchant: []EnchantSkill{
		{ID: 1, Level: 101, Exp: 5500000, SP: 550000, Rate76: 82, Rate77: 92, Rate78: 97, Rate79: 100, Rate80: 100, ItemID: 6622, ItemCount: 1},
		{ID: 1, Level: 102, Exp: 5670000, SP: 567000, Rate76: 80, Rate77: 90, Rate78: 95, Rate79: 99, Rate80: 99},
		{ID: 1, Level: 141, Exp: 5500000, SP: 550000, Rate76: 82, Rate77: 92, Rate78: 97, Rate79: 100, Rate80: 100, ItemID: 6622, ItemCount: 1},
		{ID: 1, Level: 142, Exp: 5670000, SP: 567000, Rate76: 80, Rate77: 90, Rate78: 95, Rate79: 99, Rate80: 99},
		{ID: 2, Level: 101, Exp: 5500000, SP: 550000, Rate76: 82, Rate77: 92, Rate78: 97, Rate79: 100, Rate80: 100},
	}}

	available := trees.EnchantSkillsFor(defs, SkillLevels{1: 2})
	want := []EnchantSkill{
		trees.Enchant[0],
		trees.Enchant[2],
	}
	if !equalEnchantSkills(available, want) {
		t.Fatalf("EnchantSkillsFor(max regular skill 1) = %+v, want %+v", available, want)
	}

	grant, ok := trees.EnchantSkillFor(defs, SkillLevels{1: 101}, 1, 102)
	if !ok || grant.ID != 1 || grant.Level != 102 {
		t.Fatalf("EnchantSkillFor(route 1 next level) = %+v, %v; want skill 1 level 102", grant, ok)
	}
	if got, ok := grant.SuccessRateForLevel(77); !ok || got != 90 {
		t.Fatalf("SuccessRateForLevel(77) = %d, %v; want 90, true", got, ok)
	}
	if got, ok := grant.SuccessRateForLevel(81); ok || got != 0 {
		t.Fatalf("SuccessRateForLevel(81) = %d, %v; want 0, false", got, ok)
	}
	if grant.Route() != 1 || grant.RouteStep() != 2 {
		t.Fatalf("route metadata for level 102 = route %d step %d, want 1/2", grant.Route(), grant.RouteStep())
	}

	if _, ok := trees.EnchantSkillFor(defs, SkillLevels{1: 101}, 1, 141); ok {
		t.Fatal("EnchantSkillFor(route 2 first level while route 1 is known) found a skill")
	}
	if _, ok := trees.EnchantSkillFor(defs, SkillLevels{1: 1}, 1, 101); ok {
		t.Fatal("EnchantSkillFor(first enchant below max regular level) found a skill")
	}
	if _, ok := trees.EnchantSkillFor(defs, SkillLevels{1: 2}, 2, 101); ok {
		t.Fatal("EnchantSkillFor(unlearned skill id) found a skill")
	}
}

func equalFishingSkills(a, b []FishingSkill) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalEnchantSkills(a, b []EnchantSkill) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalClanSkills(a, b []ClanSkill) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
