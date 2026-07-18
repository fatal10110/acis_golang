package player

import (
	"math"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

func TestCharacterStatFuncsAffectCombatStatsAndCanBeRemoved(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 20
	tmpl.MDef = 30
	tmpl.HPRegenTable = []float64{2}
	tmpl.MPRegenTable = []float64{0.9}
	tmpl.CPRegenTable = []float64{2}
	c := liveCharacter(1, tmpl, combatItems())

	basePAtk := c.PAtk()
	basePDef := c.PDef()
	baseMAtk := c.MAtk()
	baseMDef := c.MDef()
	baseMaxHP := c.MaxHPValue()
	baseAttackSpeed := c.AttackSpeed()
	baseRunSpeed := c.RunSpeed()
	baseHPRegen := c.HPRegenRate()
	owner := &struct{}{}

	c.AddStatFuncs([]basefunc.Func{
		basefunc.NewAdd(owner, stat.PowerAttack, 7, nil),
		basefunc.NewMul(owner, stat.PowerDefence, 2, nil),
		basefunc.NewAdd(owner, stat.MagicAttack, 3, nil),
		basefunc.NewMul(owner, stat.MagicDefence, 2, nil),
		basefunc.NewMul(owner, stat.MaxHP, 2, nil),
		basefunc.NewAdd(owner, stat.PowerAttackSpeed, 10, nil),
		basefunc.NewAdd(owner, stat.RunSpeed, 5, nil),
		basefunc.NewAdd(owner, stat.RegenerateHPRate, 1, nil),
	})

	if got, want := c.PAtk(), basePAtk+7; !closeFloat(got, want) {
		t.Fatalf("PAtk() with stat func = %v, want %v", got, want)
	}
	if got, want := c.PDef(), basePDef*2; !closeFloat(got, want) {
		t.Fatalf("PDef() with stat func = %v, want %v", got, want)
	}
	if got, want := c.MAtk(), baseMAtk+3; !closeFloat(got, want) {
		t.Fatalf("MAtk() with stat func = %v, want %v", got, want)
	}
	if got, want := c.MDef(), baseMDef*2; !closeFloat(got, want) {
		t.Fatalf("MDef() with stat func = %v, want %v", got, want)
	}
	if got, want := c.MaxHPValue(), baseMaxHP*2; !closeFloat(got, want) {
		t.Fatalf("MaxHPValue() with stat func = %v, want %v", got, want)
	}
	if got, want := c.AttackSpeed(), baseAttackSpeed+10; got != want {
		t.Fatalf("AttackSpeed() with stat func = %v, want %v", got, want)
	}
	if got, want := c.RunSpeed(), baseRunSpeed+5; !closeFloat(got, want) {
		t.Fatalf("RunSpeed() with stat func = %v, want %v", got, want)
	}
	if got, want := c.HPRegenRate(), baseHPRegen+1; !closeFloat(got, want) {
		t.Fatalf("HPRegenRate() with stat func = %v, want %v", got, want)
	}

	c.RemoveStatsByOwner(owner)

	if got := c.PAtk(); !closeFloat(got, basePAtk) {
		t.Fatalf("PAtk() after stat removal = %v, want %v", got, basePAtk)
	}
	if got := c.PDef(); !closeFloat(got, basePDef) {
		t.Fatalf("PDef() after stat removal = %v, want %v", got, basePDef)
	}
	if got := c.MAtk(); !closeFloat(got, baseMAtk) {
		t.Fatalf("MAtk() after stat removal = %v, want %v", got, baseMAtk)
	}
	if got := c.MDef(); !closeFloat(got, baseMDef) {
		t.Fatalf("MDef() after stat removal = %v, want %v", got, baseMDef)
	}
	if got := c.MaxHPValue(); !closeFloat(got, baseMaxHP) {
		t.Fatalf("MaxHPValue() after stat removal = %v, want %v", got, baseMaxHP)
	}
	if got := c.AttackSpeed(); got != baseAttackSpeed {
		t.Fatalf("AttackSpeed() after stat removal = %v, want %v", got, baseAttackSpeed)
	}
	if got := c.RunSpeed(); !closeFloat(got, baseRunSpeed) {
		t.Fatalf("RunSpeed() after stat removal = %v, want %v", got, baseRunSpeed)
	}
	if got := c.HPRegenRate(); !closeFloat(got, baseHPRegen) {
		t.Fatalf("HPRegenRate() after stat removal = %v, want %v", got, baseHPRegen)
	}
}

func TestCharacterFormulaInputsResolveLiveStats(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 25
	tmpl.MDef = 40
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	skill := modelskill.Definition{Power: 30, SkillType: "PDAM"}

	phys, ok := target.PhysicalSkillInput(caster, skill)
	if !ok {
		t.Fatal("PhysicalSkillInput() ok = false")
	}
	if got, want := phys.AttackPower, 5.4; !closeFloat(got, want) {
		t.Fatalf("PhysicalSkillInput AttackPower = %v, want %v", got, want)
	}
	if got, want := phys.SkillPower, float64(skill.Power); !closeFloat(got, want) {
		t.Fatalf("PhysicalSkillInput SkillPower = %v, want %v", got, want)
	}
	if got, want := phys.Defence, 45.0; !closeFloat(got, want) {
		t.Fatalf("PhysicalSkillInput Defence = %v, want %v", got, want)
	}
	if phys.RandomMul != 1 || phys.ElementalMul != 1 || phys.RaceMul != 1 || phys.WeaponVulnMul != 1 || phys.PvPMul != 1 {
		t.Fatalf("PhysicalSkillInput neutral multipliers = %+v", phys)
	}

	magic, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM"})
	if !ok {
		t.Fatal("MagicDamageInput() ok = false")
	}
	if got, want := magic.MAtk, 13.286025000000002; !closeFloat(got, want) {
		t.Fatalf("MagicDamageInput MAtk = %v, want %v", got, want)
	}
	if got, want := magic.MDef, 46.080000000000005; !closeFloat(got, want) {
		t.Fatalf("MagicDamageInput MDef = %v, want %v", got, want)
	}
	if magic.SkillPower != 40 || magic.PvPMul != 1 || magic.ElementalMul != 1 {
		t.Fatalf("MagicDamageInput = %+v", magic)
	}

	mana, ok := target.ManaDamageInput(caster, modelskill.Definition{Power: 20, SkillType: "MANADAM"})
	if !ok {
		t.Fatal("ManaDamageInput() ok = false")
	}
	if got, want := mana.MAtk, 13.286025000000002; !closeFloat(got, want) {
		t.Fatalf("ManaDamageInput MAtk = %v, want %v", got, want)
	}
	if got, want := mana.MDef, 46.080000000000005; !closeFloat(got, want) {
		t.Fatalf("ManaDamageInput MDef = %v, want %v", got, want)
	}
	if got, want := mana.TargetMaxMp, 38.4; !closeFloat(got, want) {
		t.Fatalf("ManaDamageInput TargetMaxMp = %v, want %v", got, want)
	}
	if mana.SkillPower != 20 || mana.VulnMul != 1 {
		t.Fatalf("ManaDamageInput = %+v", mana)
	}
}

func TestCharacterHealAmountUsesMagicAttackAndHealProficiency(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 49
	c := liveCharacter(1, tmpl, combatItems())
	owner := &struct{}{}
	c.AddStatFuncs([]basefunc.Func{basefunc.NewAdd(owner, stat.HealProficiency, 11, nil)})

	amount, ok := c.HealAmount(modelskill.Definition{SkillType: "HEAL", Power: 25})
	if !ok {
		t.Fatal("HealAmount() ok = false")
	}
	if want := 41.099019513592786; !closeFloat(amount, want) {
		t.Fatalf("HealAmount() = %v, want %v", amount, want)
	}

	static, ok := c.HealAmount(modelskill.Definition{SkillType: "HEAL_STATIC", Power: 25})
	if !ok {
		t.Fatal("HealAmount(HEAL_STATIC) ok = false")
	}
	if want := 25.0 + 11; !closeFloat(static, want) {
		t.Fatalf("HealAmount(HEAL_STATIC) = %v, want %v", static, want)
	}
}

func TestCharacterSkillSuccessInputUsesStatsAndCasterMagicAttack(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 100
	tmpl.MDef = 50
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	target.Level = 44
	owner := &struct{}{}
	target.AddStatFuncs([]basefunc.Func{basefunc.NewMul(owner, stat.StunVuln, 0.5, nil)})
	def := modelskill.Definition{
		SkillType:    "STUN",
		EffectType:   "STUN",
		Magic:        true,
		MagicLevel:   40,
		LevelDepend:  2,
		BaseLandRate: 50,
	}

	in, ok := target.SkillSuccessInput(caster, def)
	if !ok {
		t.Fatal("SkillSuccessInput() ok = false")
	}

	if in.BaseChance != 50 {
		t.Fatalf("BaseChance = %v, want 50", in.BaseChance)
	}
	if want := 0.7430194910023464; !closeFloat(in.StatModifier, want) {
		t.Fatalf("StatModifier = %v, want %v", in.StatModifier, want)
	}
	if !closeFloat(in.VulnModifier, 0.5) {
		t.Fatalf("VulnModifier = %v, want 0.5", in.VulnModifier)
	}
	if want := 0.9420817669172932; !closeFloat(in.MAtkModifier, want) {
		t.Fatalf("MAtkModifier = %v, want %v", in.MAtkModifier, want)
	}
	if want := 1 + 0.01*(float64(def.MagicLevel+def.LevelDepend-target.Level)); !closeFloat(in.LevelModifier, want) {
		t.Fatalf("LevelModifier = %v, want %v", in.LevelModifier, want)
	}
}

func TestCharacterSkillSuccessInputDoesNotFallbackToSkillType(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 100
	tmpl.MDef = 50
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	owner := &struct{}{}
	target.AddStatFuncs([]basefunc.Func{basefunc.NewMul(owner, stat.StunVuln, 0.5, nil)})

	in, ok := target.SkillSuccessInput(caster, modelskill.Definition{
		SkillType:    "STUN",
		Magic:        true,
		BaseLandRate: 50,
	})
	if !ok {
		t.Fatal("SkillSuccessInput() ok = false")
	}

	if in.StatModifier != 1 {
		t.Fatalf("StatModifier = %v, want 1 without EffectType", in.StatModifier)
	}
	if in.VulnModifier != 1 {
		t.Fatalf("VulnModifier = %v, want 1 without EffectType", in.VulnModifier)
	}
}

func closeFloat(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
