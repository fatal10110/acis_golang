package player

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

// truncFixture is a level-1 character whose four combat stats all finalize
// fractional: fists pAtk 5 and the template's pDef 51 / mAtk 25 / mDef 40,
// run through FuncPAtkMod, FuncPDefMod, FuncMAtkMod and FuncMDefMod.
func truncFixture(id int32) (*Character, *Template) {
	tmpl := combatTemplate()
	tmpl.PDef = 51
	tmpl.MAtk = 25
	tmpl.MDef = 40
	return liveCharacter(id, tmpl, combatItems()), tmpl
}

// TestCombatStatGettersTruncateTheFinalizedStat pins the (int) cast in
// CreatureStatus.getPAtk / getPDef / getMAtk / getMDef
// (CreatureStatus.java:628-675): the finalized stat reaches every caller
// without its fractional part.
func TestCombatStatGettersTruncateTheFinalizedStat(t *testing.T) {
	c, tmpl := truncFixture(1)
	lm := javaLevelMod(1)
	intMod := statbonus.INTBonus[tmpl.INT]
	for _, tc := range []struct {
		name string
		got  float64
		raw  float64 // the finalized double, from the reference funcs
	}{
		{"PAtk", c.PAtk(), 5 * statbonus.STRBonus[tmpl.STR] * lm},
		{"PDef", c.PDef(), tmpl.PDef * lm},
		{"MAtk", c.MAtk(), tmpl.MAtk * ((lm * lm) * (intMod * intMod))},
		{"MDef", c.MDef(), tmpl.MDef * statbonus.MENBonus[tmpl.MEN] * lm},
	} {
		if tc.raw == math.Trunc(tc.raw) {
			t.Fatalf("%s fixture finalizes whole (%v); the oracle needs a fraction", tc.name, tc.raw)
		}
		if want := math.Trunc(tc.raw); tc.got != want {
			t.Fatalf("%s() = %v, want %v (finalized %v)", tc.name, tc.got, want, tc.raw)
		}
	}
}

// TestPhysicalHitReadsTruncatedStats pins Formulas.calcPhysicalAttackDamage
// (Formulas.java:386-406): attack power and defence are the getters' ints,
// so the hit scales 5 * 77 / 45, not 5.4 * 77 / 45.9.
func TestPhysicalHitReadsTruncatedStats(t *testing.T) {
	attacker, _ := truncFixture(1)
	target, _ := truncFixture(2)

	in, _ := creature.ResolvePhysicalAttackInput(attacker, target, false)
	if in.AttackPower != 5 || in.Defence != 45 {
		t.Fatalf("hit input attack %v / defence %v, want 5 / 45", in.AttackPower, in.Defence)
	}
	want := (5 * in.PosMul * in.RandomMul * in.RaceMul * in.PvPMul * in.ElementalMul * in.WeaponVulnMul) * 77. / 45
	if got := formulas.PhysicalAttackDamage(in); !closeFloat(got, want) {
		t.Fatalf("hit damage = %v, want %v", got, want)
	}
}

// TestMagicDamageReadsTruncatedStats pins Formulas.calcMagicDam
// (Formulas.java:571-589): M.Atk 13.286025 and M.Def 46.08 reach the
// formula as 13 and 46.
func TestMagicDamageReadsTruncatedStats(t *testing.T) {
	caster, _ := truncFixture(1)
	caster.SetRollSource(func(int) int { return 999 }) // no magic critical
	target, _ := truncFixture(2)

	in, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM", Magic: true}, false)
	if !ok {
		t.Fatal("MagicDamageInput() ok = false")
	}
	if in.MAtk != 13 || in.MDef != 46 {
		t.Fatalf("MDAM input M.Atk %v / M.Def %v, want 13 / 46", in.MAtk, in.MDef)
	}
	want := 91 * math.Sqrt(13) / 46 * 40
	if got := formulas.MagicDamage(in); got != want {
		t.Fatalf("MDAM damage = %v, want %v", got, want)
	}
	if untruncated := 91 * math.Sqrt(13.286025) / 46.08 * 40; int(untruncated) == int(want) {
		t.Fatalf("fixture cannot tell truncated from untruncated inputs (both %d)", int(want))
	}
}
