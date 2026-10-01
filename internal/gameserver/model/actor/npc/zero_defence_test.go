package npc

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// zeroDefenceHostiles spawns an attacker and a target whose finalized P.Def
// and M.Def are both 0.5: above calcStat's <= 0 floor, so the getters'
// truncation hands the damage formulas a defence of exactly 0.
//
// Expected values (issue #3002) follow Formulas.calcPhysicalAttackDamage,
// calcPhysicalSkillDamage, calcBlowDamage, calcMagicDam and calcManaDam
// dividing by that 0 with no guard, and the callers' (int) casts (JLS
// 5.1.3): +Infinity narrows to Integer.MAX_VALUE, NaN to 0.
func zeroDefenceHostiles(t *testing.T) (attacker, target *Hostile) {
	t.Helper()
	tpl := &Template{ID: 1, Type: "Monster", PAtk: 100, PDef: 50, MAtk: 100, MDef: 50, CritRate: 0, DEX: 30, HPMax: 500, MPMax: 300, CanBeAttacked: true}
	state := world.New()
	target = newCombatHostile(t, 1, tpl)
	attacker = newCombatHostile(t, 2, tpl)
	state.Spawn(target, 0, 0, 0, 0)
	state.Spawn(attacker, -100, 0, 0, 0)
	attacker.SetRollSource(func(int) int { return 0 })
	target.SetRollSource(func(int) int { return 0 })
	target.AddStatFuncs([]effect.Mod{
		{Stat: stat.PowerDefence, Op: effect.OpSet, Value: 0.5, Owner: effect.ModOwnerEffect(&effect.Effect{})},
		{Stat: stat.MagicDefence, Op: effect.OpSet, Value: 0.5, Owner: effect.ModOwnerEffect(&effect.Effect{})},
	})
	if target.PDef() != 0 || target.MDef() != 0 {
		t.Fatalf("PDef/MDef = %v/%v, want both truncated to 0", target.PDef(), target.MDef())
	}
	return attacker, target
}

func TestZeroTruncatedDefenceAutoAttackSaturates(t *testing.T) {
	attacker, target := zeroDefenceHostiles(t)

	in, shield := creature.ResolvePhysicalAttackInput(attacker, target, false)
	if in.Defence != 0 {
		t.Fatalf("Defence = %v, want 0 (no substitution)", in.Defence)
	}
	if got := creature.ApplyPhysicalAttackDamage(in, shield, false); got != math.MaxInt32 {
		t.Fatalf("non-critical damage = %d, want %d", got, math.MaxInt32)
	}
	// The dual-wield halving runs on the narrowed int: MAX_VALUE / 2.
	if got := creature.ApplyPhysicalAttackDamage(in, shield, true); got != math.MaxInt32/2 {
		t.Fatalf("split damage = %d, want %d", got, math.MaxInt32/2)
	}

	// A critical with no critical-damage-add stat scales 0 by 77/0 = NaN,
	// which poisons the whole hit: (int) NaN is 0.
	crit, _ := creature.ResolvePhysicalAttackInput(attacker, target, true)
	if got := creature.ApplyPhysicalAttackDamage(crit, shield, false); got != 0 {
		t.Fatalf("critical damage = %d, want 0", got)
	}
}

func TestZeroTruncatedDefenceSkillFormulasDivideByZero(t *testing.T) {
	attacker, target := zeroDefenceHostiles(t)

	pdam, ok := target.PhysicalSkillInput(attacker, skill.Definition{SkillType: "PDAM", Power: 100})
	if !ok || pdam.Defence != 0 {
		t.Fatalf("PDAM input ok=%v Defence=%v, want ok with Defence 0", ok, pdam.Defence)
	}
	if got := formulas.PhysicalSkillDamage(pdam); !math.IsInf(got, 1) {
		t.Fatalf("PDAM damage = %v, want +Inf", got)
	}

	blow, ok := target.BlowInput(attacker, skill.Definition{SkillType: "BLOW", Power: 100, BaseLandRate: 50})
	if !ok || !blow.Landed || blow.Defence != 0 {
		t.Fatalf("BLOW input ok=%v landed=%v Defence=%v, want a landed blow with Defence 0", ok, blow.Landed, blow.Defence)
	}
	if got := formulas.BlowDamage(blow); !math.IsInf(got, 1) {
		t.Fatalf("BLOW damage = %v, want +Inf", got)
	}

	mdam, ok := target.MagicDamageInput(attacker, skill.Definition{SkillType: "MDAM", Power: 10}, false)
	if !ok || mdam.MDef != 0 {
		t.Fatalf("MDAM input ok=%v MDef=%v, want ok with MDef 0", ok, mdam.MDef)
	}
	if got := formulas.MagicDamage(mdam); !math.IsInf(got, 1) {
		t.Fatalf("MDAM damage = %v, want +Inf", got)
	}

	mana, ok := creature.ResolveManaDamageInput(attacker, target, target.MaxMPValue(), skill.Definition{SkillType: "MANADAM", Power: 10})
	if !ok || mana.MDef != 0 {
		t.Fatalf("MANADAM input ok=%v MDef=%v, want ok with MDef 0", ok, mana.MDef)
	}
	if got := formulas.ManaDamage(mana); !math.IsInf(got, 1) {
		t.Fatalf("MANADAM damage = %v, want +Inf", got)
	}

	// The landing-rate M.Atk term divides by the same 0; the [1, 99] clamp
	// then caps it.
	def := skill.Definition{SkillType: "DEBUFF", EffectType: "DEBUFF", Magic: true, BaseLandRate: 10}
	if got := creature.SkillMAtkModifier(target, attacker, def, false); !math.IsInf(got, 1) {
		t.Fatalf("SkillMAtkModifier = %v, want +Inf", got)
	}
	success, ok := creature.ResolveSkillSuccessInput(attacker, target, def, false, formulas.ShieldFailed)
	if !ok {
		t.Fatal("ResolveSkillSuccessInput ok = false")
	}
	if got := formulas.SkillSuccessRate(success); got != 99 {
		t.Fatalf("SkillSuccessRate = %v, want 99", got)
	}
}

func TestHostileReduceHPWithUnboundedDamage(t *testing.T) {
	attacker, target := zeroDefenceHostiles(t)

	// NaN (a zero-defence hit scaled by a zero multiplier) fails every
	// "damage > 0" test the HP write sits behind: nothing changes.
	before := target.HP()
	target.ReduceHP(math.NaN(), attacker, skill.Definition{})
	if got := target.HP(); got != before || math.IsNaN(got) {
		t.Fatalf("HP after NaN hit = %v, want unchanged %v", got, before)
	}

	target.ReduceHP(math.Inf(1), attacker, skill.Definition{})
	if got := target.HP(); got != 0 || !target.Dead() {
		t.Fatalf("HP after +Inf hit = %v dead=%v, want 0 and dead", got, target.Dead())
	}
}
