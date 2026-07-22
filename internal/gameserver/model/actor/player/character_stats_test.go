package player

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
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

func TestCharacterSkillDamageInputsUseElementalSkillModifier(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 25
	tmpl.MDef = 40
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	owner := &struct{}{}
	target.AddStatFuncs([]basefunc.Func{basefunc.NewMul(owner, stat.FireRes, 0.75, nil)})

	phys, ok := target.PhysicalSkillInput(caster, modelskill.Definition{Power: 30, SkillType: "PDAM", Element: modelskill.ElementFire})
	if !ok {
		t.Fatal("PhysicalSkillInput() ok = false")
	}
	if !closeFloat(phys.ElementalMul, 0.75) {
		t.Fatalf("PhysicalSkillInput ElementalMul = %v, want 0.75", phys.ElementalMul)
	}

	magic, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM", Element: modelskill.ElementFire})
	if !ok {
		t.Fatal("MagicDamageInput() ok = false")
	}
	if !closeFloat(magic.ElementalMul, 0.75) {
		t.Fatalf("MagicDamageInput ElementalMul = %v, want 0.75", magic.ElementalMul)
	}

	neutral, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM"})
	if !ok {
		t.Fatal("MagicDamageInput(neutral) ok = false")
	}
	if !closeFloat(neutral.ElementalMul, 1) {
		t.Fatalf("neutral MagicDamageInput ElementalMul = %v, want 1", neutral.ElementalMul)
	}
}

func TestCharacterMagicDamageInputRollsMagicCritical(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 25
	tmpl.MDef = 40
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())

	caster.SetRollSource(func(int) int { return 7 })
	magic, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM"})
	if !ok {
		t.Fatal("MagicDamageInput() ok = false")
	}
	if !magic.MagicCrit {
		t.Fatal("MagicDamageInput MagicCrit = false, want true for roll below mCrit rate")
	}

	caster.SetRollSource(func(int) int { return 8 })
	magic, ok = target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM"})
	if !ok {
		t.Fatal("MagicDamageInput() second call ok = false")
	}
	if magic.MagicCrit {
		t.Fatal("MagicDamageInput MagicCrit = true, want false for roll at mCrit rate")
	}
}

func TestCharacterBlowInputUsesTargetRelativeHeading(t *testing.T) {
	tmpl := combatTemplate()
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	target.SetLastKnownPosition(location.Location{X: 0, Y: 0, Z: 0}, 0)

	caster.SetLastKnownPosition(location.Location{X: -80, Y: 0, Z: 0}, 0)
	behind, ok := target.BlowInput(caster, modelskill.Definition{Power: 30, SkillType: "BLOW"})
	if !ok {
		t.Fatal("BlowInput(behind) ok = false")
	}
	if !closeFloat(behind.PosMul, 1.1) {
		t.Fatalf("behind BlowInput PosMul = %v, want 1.1", behind.PosMul)
	}

	caster.SetLastKnownPosition(location.Location{X: 0, Y: 80, Z: 0}, 0)
	side, ok := target.BlowInput(caster, modelskill.Definition{Power: 30, SkillType: "BLOW"})
	if !ok {
		t.Fatal("BlowInput(side) ok = false")
	}
	if !closeFloat(side.PosMul, 1.025) {
		t.Fatalf("side BlowInput PosMul = %v, want 1.025", side.PosMul)
	}

	caster.SetLastKnownPosition(location.Location{X: 80, Y: 0, Z: 0}, 0)
	front, ok := target.BlowInput(caster, modelskill.Definition{Power: 30, SkillType: "BLOW"})
	if !ok {
		t.Fatal("BlowInput(front) ok = false")
	}
	if !closeFloat(front.PosMul, 1) {
		t.Fatalf("front BlowInput PosMul = %v, want 1", front.PosMul)
	}
}

func TestCharacterDamageInputsUseChargedShots(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 25
	tmpl.MDef = 40
	items := combatItems()
	soulWeapon := &item.Instance{
		ObjectID: 10, TemplateID: 2, Location: item.LocationPaperdoll, LocationData: itemcontainer.RHand,
		ShotsMask: item.ShotSoul.Mask(),
	}
	soulCaster := liveCharacter(1, tmpl, items, soulWeapon)
	target := liveCharacter(2, tmpl, items)

	phys, ok := target.PhysicalSkillInput(soulCaster, modelskill.Definition{Power: 30, SkillType: "PDAM", SoulShotBoost: 2})
	if !ok {
		t.Fatal("PhysicalSkillInput() ok = false")
	}
	if !phys.SoulShot || phys.SkillPower != 60 {
		t.Fatalf("PhysicalSkillInput soulshot = %v skillPower = %v, want true/60", phys.SoulShot, phys.SkillPower)
	}

	blow, ok := target.BlowInput(soulCaster, modelskill.Definition{Power: 30, SkillType: "BLOW", SoulShotBoost: 2})
	if !ok {
		t.Fatal("BlowInput() ok = false")
	}
	if !blow.SoulShot || blow.SkillPower != 60 {
		t.Fatalf("BlowInput soulshot = %v skillPower = %v, want true/60", blow.SoulShot, blow.SkillPower)
	}

	spiritWeapon := &item.Instance{
		ObjectID: 11, TemplateID: 2, Location: item.LocationPaperdoll, LocationData: itemcontainer.RHand,
		ShotsMask: item.ShotSpirit.Mask(),
	}
	spiritCaster := liveCharacter(3, tmpl, items, spiritWeapon)
	magic, ok := target.MagicDamageInput(spiritCaster, modelskill.Definition{Power: 40, SkillType: "MDAM"})
	if !ok {
		t.Fatal("MagicDamageInput(spirit) ok = false")
	}
	if !magic.SoulShot || magic.BlessedSoulShot {
		t.Fatalf("MagicDamageInput spirit flags = soul %v blessed %v, want true/false", magic.SoulShot, magic.BlessedSoulShot)
	}
	mana, ok := target.ManaDamageInput(spiritCaster, modelskill.Definition{Power: 20, SkillType: "MANADAM"})
	if !ok {
		t.Fatal("ManaDamageInput(spirit) ok = false")
	}
	if !mana.SoulShot || mana.BlessedSoulShot {
		t.Fatalf("ManaDamageInput spirit flags = soul %v blessed %v, want true/false", mana.SoulShot, mana.BlessedSoulShot)
	}

	blessedWeapon := &item.Instance{
		ObjectID: 12, TemplateID: 2, Location: item.LocationPaperdoll, LocationData: itemcontainer.RHand,
		ShotsMask: item.ShotBlessedSpirit.Mask(),
	}
	blessedCaster := liveCharacter(4, tmpl, items, blessedWeapon)
	magic, ok = target.MagicDamageInput(blessedCaster, modelskill.Definition{Power: 40, SkillType: "MDAM"})
	if !ok {
		t.Fatal("MagicDamageInput(blessed) ok = false")
	}
	if magic.SoulShot || !magic.BlessedSoulShot {
		t.Fatalf("MagicDamageInput blessed flags = soul %v blessed %v, want false/true", magic.SoulShot, magic.BlessedSoulShot)
	}
}

func TestCharacterDamageInputsUsePvPMultipliers(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 25
	tmpl.MDef = 40
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	owner := &struct{}{}
	caster.AddStatFuncs([]basefunc.Func{
		basefunc.NewMul(owner, stat.PvPPhysSkillDmg, 0.8, nil),
		basefunc.NewMul(owner, stat.PvPMagicalDmg, 1.3, nil),
	})

	phys, ok := target.PhysicalSkillInput(caster, modelskill.Definition{Power: 30, SkillType: "PDAM"})
	if !ok {
		t.Fatal("PhysicalSkillInput() ok = false")
	}
	if !closeFloat(phys.PvPMul, 0.8) {
		t.Fatalf("PhysicalSkillInput PvPMul = %v, want 0.8", phys.PvPMul)
	}

	blow, ok := target.BlowInput(caster, modelskill.Definition{Power: 30, SkillType: "BLOW"})
	if !ok {
		t.Fatal("BlowInput() ok = false")
	}
	if !blow.IsPvP || !closeFloat(blow.PvPMul, 0.8) {
		t.Fatalf("BlowInput PvP = %v mul %v, want true/0.8", blow.IsPvP, blow.PvPMul)
	}

	magic, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM", Magic: true})
	if !ok {
		t.Fatal("MagicDamageInput(magic) ok = false")
	}
	if !closeFloat(magic.PvPMul, 1.3) {
		t.Fatalf("MagicDamageInput magic PvPMul = %v, want 1.3", magic.PvPMul)
	}

	physicalMagic, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM"})
	if !ok {
		t.Fatal("MagicDamageInput(physical skill type) ok = false")
	}
	if !closeFloat(physicalMagic.PvPMul, 0.8) {
		t.Fatalf("MagicDamageInput physical PvPMul = %v, want 0.8", physicalMagic.PvPMul)
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
	target.CharLevel = 44
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

	in, ok := target.SkillSuccessInput(caster, def, false, formulas.ShieldFailed)
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
	if want := 1 + 0.01*(float64(def.MagicLevel+def.LevelDepend-target.CharLevel)); !closeFloat(in.LevelModifier, want) {
		t.Fatalf("LevelModifier = %v, want %v", in.LevelModifier, want)
	}
}

func TestCharacterSkillSuccessInputFoldsElementalResistanceIntoVulnerability(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 100
	tmpl.MDef = 50
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	owner := &struct{}{}
	target.AddStatFuncs([]basefunc.Func{
		basefunc.NewMul(owner, stat.FireRes, 0.36, nil),
		basefunc.NewMul(owner, stat.StunVuln, 0.5, nil),
	})

	in, ok := target.SkillSuccessInput(caster, modelskill.Definition{
		SkillType:    "STUN",
		EffectType:   "STUN",
		Element:      modelskill.ElementFire,
		BaseLandRate: 50,
	}, false, formulas.ShieldFailed)
	if !ok {
		t.Fatal("SkillSuccessInput() ok = false")
	}

	// Java folds sqrt(elemental resistance) in as the vulnerability base
	// before applying the stat-specific (here STUN) vulnerability on top:
	// sqrt(0.36) * 0.5 = 0.3.
	if want := 0.3; !closeFloat(in.VulnModifier, want) {
		t.Fatalf("VulnModifier = %v, want %v", in.VulnModifier, want)
	}
}

func TestCharacterManaDamageInputFoldsElementalResistanceIntoVulnerability(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 100
	tmpl.MDef = 50
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	owner := &struct{}{}
	target.AddStatFuncs([]basefunc.Func{basefunc.NewMul(owner, stat.FireRes, 0.36, nil)})

	mana, ok := target.ManaDamageInput(caster, modelskill.Definition{
		SkillType: "MANADAM",
		Element:   modelskill.ElementFire,
		Power:     20,
	})
	if !ok {
		t.Fatal("ManaDamageInput() ok = false")
	}
	// MANADAM has no matching vulnerability case (see the STUN/POISON/...
	// switch), so Java's calcSkillVulnerability returns the elemental base
	// unchanged: sqrt(0.36) = 0.6.
	if want := 0.6; !closeFloat(mana.VulnMul, want) {
		t.Fatalf("VulnMul = %v, want %v", mana.VulnMul, want)
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
	}, false, formulas.ShieldFailed)
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

// TestCharacterSkillSuccessInputQuadruplesMAtkOnBlessedSpiritshot asserts a
// blessed-spiritshot charge quadruples the caster's effective magic attack
// before the square root is taken, exactly doubling the resulting modifier
// (sqrt(4x) == 2*sqrt(x)) relative to an uncharged cast against the same
// stats used by TestCharacterSkillSuccessInputUsesStatsAndCasterMagicAttack.
func TestCharacterSkillSuccessInputQuadruplesMAtkOnBlessedSpiritshot(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 100
	tmpl.MDef = 50
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	def := modelskill.Definition{SkillType: "STUN", EffectType: "STUN", Magic: true, BaseLandRate: 50}

	without, ok := target.SkillSuccessInput(caster, def, false, formulas.ShieldFailed)
	if !ok {
		t.Fatal("SkillSuccessInput(bss=false) ok = false")
	}
	with, ok := target.SkillSuccessInput(caster, def, true, formulas.ShieldFailed)
	if !ok {
		t.Fatal("SkillSuccessInput(bss=true) ok = false")
	}

	if want := without.MAtkModifier * 2; !closeFloat(with.MAtkModifier, want) {
		t.Fatalf("MAtkModifier with bss = %v, want %v (2x the uncharged modifier)", with.MAtkModifier, want)
	}
}

// TestCharacterSkillSuccessInputCarriesShieldOutcome asserts the
// already-resolved shield-block outcome passed into SkillSuccessInput
// reaches the returned formula input unchanged, and that a perfect block
// fails the landing roll outright through the real formulas pipeline.
func TestCharacterSkillSuccessInputCarriesShieldOutcome(t *testing.T) {
	tmpl := combatTemplate()
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	def := modelskill.Definition{SkillType: "STUN", BaseLandRate: 100, IgnoreResists: true}

	in, ok := target.SkillSuccessInput(caster, def, false, formulas.ShieldPerfect)
	if !ok {
		t.Fatal("SkillSuccessInput() ok = false")
	}
	if in.Shield != formulas.ShieldPerfect {
		t.Fatalf("Shield = %v, want ShieldPerfect", in.Shield)
	}
	if rate := formulas.SkillSuccessRate(in); rate != 0 {
		t.Fatalf("SkillSuccessRate() = %v, want 0 for a perfect block despite IgnoreResists", rate)
	}
}

type shieldDefenseResolver interface {
	ShieldDefense(caster any, def modelskill.Definition, isCrit bool) formulas.ShieldDefense
}

func TestCharacterShieldDefenseUsesLiveShieldStatsFacingAndRoll(t *testing.T) {
	tmpl := combatTemplate()
	items := shieldDefenseItems()
	caster := liveCharacter(1, tmpl, items)
	target := liveCharacter(2, tmpl, items, equippedShield())
	caster.SetLastKnownPosition(location.Location{X: 80, Y: 0, Z: 0}, 0)
	target.SetLastKnownPosition(location.Location{X: 0, Y: 0, Z: 0}, 0)
	target.AddStatFuncs([]basefunc.Func{
		basefunc.NewSet(target, stat.ShieldRate, 20, nil),
		basefunc.NewSet(target, stat.ShieldDefenceAngle, 120, nil),
	})

	src, ok := any(target).(shieldDefenseResolver)
	if !ok {
		t.Fatal("Character must resolve live shield defense")
	}

	tests := []struct {
		name string
		roll int
		want formulas.ShieldDefense
	}{
		{name: "perfect block", roll: 0, want: formulas.ShieldPerfect},
		{name: "ordinary block", roll: 5, want: formulas.ShieldSuccess},
		{name: "failed block", roll: 99, want: formulas.ShieldFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target.SetRollSource(func(n int) int {
				if n != 100 {
					t.Fatalf("shield roll bound = %d, want 100", n)
				}
				return tt.roll
			})
			if got := src.ShieldDefense(caster, modelskill.Definition{SkillType: "STUN"}, false); got != tt.want {
				t.Fatalf("ShieldDefense() = %v, want %v", got, tt.want)
			}
		})
	}

	caster.SetLastKnownPosition(location.Location{X: -80, Y: 0, Z: 0}, 0)
	target.AddStatFuncs([]basefunc.Func{basefunc.NewSet(target, stat.ShieldDefenceAngle, 360, nil)})
	target.SetRollSource(func(int) int { return 0 })
	if got := src.ShieldDefense(caster, modelskill.Definition{SkillType: "STUN"}, false); got != formulas.ShieldPerfect {
		t.Fatalf("ShieldDefense() with 360-degree stat = %v, want ShieldPerfect", got)
	}
}

func TestCharacterShieldDefenseGatesEquipStatsAndFacing(t *testing.T) {
	tmpl := combatTemplate()
	items := shieldDefenseItems()
	def := modelskill.Definition{SkillType: "STUN"}

	tests := []struct {
		name      string
		equipped  []*item.Instance
		rate      float64
		angle     float64
		casterLoc location.Location
		def       modelskill.Definition
	}{
		{
			name:      "no shield equipped",
			rate:      20,
			angle:     120,
			casterLoc: location.Location{X: 80, Y: 0, Z: 0},
			def:       def,
		},
		{
			name:      "left hand is not armor",
			equipped:  []*item.Instance{equippedArrow()},
			rate:      20,
			angle:     120,
			casterLoc: location.Location{X: 80, Y: 0, Z: 0},
			def:       def,
		},
		{
			name:      "zero shield rate",
			equipped:  []*item.Instance{equippedShield()},
			angle:     120,
			casterLoc: location.Location{X: 80, Y: 0, Z: 0},
			def:       def,
		},
		{
			name:      "outside shield angle",
			equipped:  []*item.Instance{equippedShield()},
			rate:      20,
			angle:     120,
			casterLoc: location.Location{X: -80, Y: 0, Z: 0},
			def:       def,
		},
		{
			name:      "skill ignores shield",
			equipped:  []*item.Instance{equippedShield()},
			rate:      20,
			angle:     120,
			casterLoc: location.Location{X: 80, Y: 0, Z: 0},
			def:       modelskill.Definition{SkillType: "STUN", IgnoreShield: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caster := liveCharacter(1, tmpl, items)
			target := liveCharacter(2, tmpl, items, tt.equipped...)
			caster.SetLastKnownPosition(tt.casterLoc, 0)
			target.SetLastKnownPosition(location.Location{X: 0, Y: 0, Z: 0}, 0)
			target.SetRollSource(func(int) int { return 0 })
			target.AddStatFuncs([]basefunc.Func{
				basefunc.NewSet(target, stat.ShieldRate, tt.rate, nil),
				basefunc.NewSet(target, stat.ShieldDefenceAngle, tt.angle, nil),
			})

			src, ok := any(target).(shieldDefenseResolver)
			if !ok {
				t.Fatal("Character must resolve live shield defense")
			}
			if got := src.ShieldDefense(caster, tt.def, false); got != formulas.ShieldFailed {
				t.Fatalf("ShieldDefense() = %v, want ShieldFailed", got)
			}
		})
	}
}

func shieldDefenseItems() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{Type: item.WeaponFist}},
		{ID: 2, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{Type: item.WeaponSword}},
		{ID: 3, Kind: item.KindArmor, Slot: item.SlotLHand, Armor: &item.ArmorDetail{Type: item.ArmorShield}},
		{ID: 4, Kind: item.KindEtcItem, Slot: item.SlotLHand, EtcItem: &item.EtcItemDetail{Type: item.EtcItemArrow}},
	})
}

func equippedShield() *item.Instance {
	return &item.Instance{ObjectID: 30, TemplateID: 3, Location: item.LocationPaperdoll, LocationData: itemcontainer.LHand}
}

func equippedArrow() *item.Instance {
	return &item.Instance{ObjectID: 40, TemplateID: 4, Location: item.LocationPaperdoll, LocationData: itemcontainer.LHand}
}

func closeFloat(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
