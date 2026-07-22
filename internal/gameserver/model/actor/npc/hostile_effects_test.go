package npc

import (
	"math"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

func TestHostileSkillSuccessInputUsesTemplateStatsAndCasterMagicAttack(t *testing.T) {
	caster := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", Level: 12, MAtk: 200})
	target := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster", Level: 10, MEN: 40, MDef: 50})
	def := modelskill.Definition{BaseLandRate: 50, EffectType: "ROOT", Magic: true, LevelDepend: 1}

	without, ok := target.SkillSuccessInput(caster, def, false, formulas.ShieldFailed)
	if !ok {
		t.Fatal("SkillSuccessInput() ok = false")
	}
	with, ok := target.SkillSuccessInput(caster, def, true, formulas.ShieldFailed)
	if !ok {
		t.Fatal("SkillSuccessInput(bss=true) ok = false")
	}

	if without.BaseChance != 50 {
		t.Fatalf("BaseChance = %v, want 50", without.BaseChance)
	}
	if want := math.Max(0, 2-math.Sqrt(statbonus.MENBonus[40])); !closeNPCFloat(without.StatModifier, want) {
		t.Fatalf("StatModifier = %v, want %v", without.StatModifier, want)
	}
	if want := without.MAtkModifier * 2; !closeNPCFloat(with.MAtkModifier, want) {
		t.Fatalf("MAtkModifier with bss = %v, want %v", with.MAtkModifier, want)
	}
	if want := 1.015; !closeNPCFloat(without.LevelModifier, want) {
		t.Fatalf("LevelModifier = %v, want %v", without.LevelModifier, want)
	}
}

func TestHostileSkillSuccessInputAllowsIgnoreResistsWithoutCasterStats(t *testing.T) {
	target := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster"})

	in, ok := target.SkillSuccessInput(nil, modelskill.Definition{
		BaseLandRate:  100,
		IgnoreResists: true,
	}, false, formulas.ShieldPerfect)
	if !ok {
		t.Fatal("SkillSuccessInput(ignore resists) ok = false")
	}
	if !in.IgnoreResists || in.BaseChance != 100 || in.Shield != formulas.ShieldPerfect {
		t.Fatalf("SkillSuccessInput(ignore resists) = %+v, want base chance, ignore flag, and shield preserved", in)
	}
}

func closeNPCFloat(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
