package player

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

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

func TestCharacterSkillReflectInputUsesMagicSpecificStat(t *testing.T) {
	target := liveCharacter(1, combatTemplate(), combatItems())
	target.AddStatFuncs([]basefunc.Func{
		basefunc.NewSet(target, stat.ReflectSkillMagic, 17, nil),
		basefunc.NewSet(target, stat.ReflectSkillPhysic, 29, nil),
	})

	if got := target.SkillReflectInput(modelskill.Definition{Magic: true}).ReflectChance; got != 17 {
		t.Fatalf("magic ReflectChance = %v, want 17", got)
	}
	if got := target.SkillReflectInput(modelskill.Definition{}).ReflectChance; got != 29 {
		t.Fatalf("physical ReflectChance = %v, want 29", got)
	}
}

func TestCharacterEffectSuccessInputRespectsTemplateResistance(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 100
	tmpl.MDef = 50
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	target.AddStatFuncs([]basefunc.Func{basefunc.NewMul(&struct{}{}, stat.StunVuln, 0.5, nil)})
	def := modelskill.Definition{IgnoreResists: true, Magic: true, MagicLevel: 40, EffectType: "ROOT"}

	in, ok := target.EffectSuccessInput(caster, def, modelskill.EffectTemplate{EffectPower: 50, EffectType: "STUN"}, false, formulas.ShieldFailed)
	if !ok {
		t.Fatal("EffectSuccessInput() ok = false")
	}
	if in.IgnoreResists {
		t.Fatal("EffectSuccessInput() ignored template resistance")
	}
	if in.BaseChance != 50 || !closeFloat(in.VulnModifier, 0.5) {
		t.Fatalf("EffectSuccessInput() = %+v, want template power and STUN resistance", in)
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
