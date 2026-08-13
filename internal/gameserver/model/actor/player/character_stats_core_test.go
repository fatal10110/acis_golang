package player

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
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
	// caster's test fist template carries no random_damage, so it falls
	// back to Creature.getRandomDamageMultiplier's weaponless spread
	// `5 + sqrt(level)` (Creature.java:1699-1710); level 1 -> spread 6,
	// and zeroRoll pins the roll at the bottom of the range: 1 - 6/100.
	if got, want := phys.RandomMul, 0.94; !closeFloat(got, want) {
		t.Fatalf("PhysicalSkillInput RandomMul = %v, want %v", got, want)
	}
	if phys.ElementalMul != 1 || phys.RaceMul != 1 || phys.WeaponVulnMul != 1 || phys.PvPMul != 1 {
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

func TestCharacterLethalInputAndOutcomes(t *testing.T) {
	tmpl := combatTemplate()
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	target.SetResourceValues(Resources{
		MaxHP: 500, CurrentHP: 500,
		MaxMP: 30, CurrentMP: 30,
		MaxCP: 200, CurrentCP: 200,
	})
	caster.CharLevel = 40
	target.CharLevel = 45
	caster.AddStatFuncs([]basefunc.Func{
		basefunc.NewMul(&struct{}{}, stat.LethalRate, 1.5, nil),
	})

	skill := modelskill.Definition{MagicLevel: 40, LethalChance1: 30, LethalChance2: 10}
	in, ok := target.LethalInput(caster, skill)
	if !ok {
		t.Fatal("LethalInput() ok = false")
	}
	if in.Chance1 != 30 || in.Chance2 != 10 || in.MagicLevel != 40 {
		t.Fatalf("LethalInput skill fields = %+v, want chances 30/10 and magic level 40", in)
	}
	if in.AttackerLevel != 40 || in.TargetLevel != 45 || in.LethalMul != 1.5 {
		t.Fatalf("LethalInput actor fields = %+v, want attacker 40 target 45 lethal mul 1.5", in)
	}

	target.SetHP(500)
	target.SetCP(200)
	target.ApplyLethalOutcome(formulas.LethalHalf, caster, skill)
	if target.HP() != 500 || target.CP() != 1 {
		t.Fatalf("half lethal hp/cp = %v/%v, want 500/1", target.HP(), target.CP())
	}

	target.SetHP(500)
	target.SetCP(200)
	target.ApplyLethalOutcome(formulas.LethalFull, caster, skill)
	if target.HP() != 1 || target.CP() != 1 {
		t.Fatalf("full lethal hp/cp = %v/%v, want 1/1", target.HP(), target.CP())
	}
}
