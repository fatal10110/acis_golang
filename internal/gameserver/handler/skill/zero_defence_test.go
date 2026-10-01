package skill

import (
	"math"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// TestZeroDefenceDamageNarrowsLikeAnIntCast pins issue #3002: a target whose
// truncated P.Def/M.Def is 0 makes the damage formulas divide by 0, and each
// handler then reproduces the reference's own narrowing. Pdam.java keeps the
// +Infinity double for reduceCurrentHp and reports (int) damage;
// Mdam.java/Blow.java narrow first, Blow.java then doubles a critical on the
// widened double. (int) +Infinity is Integer.MAX_VALUE and (int) NaN is 0
// (JLS 5.1.3).
func TestZeroDefenceDamageNarrowsLikeAnIntCast(t *testing.T) {
	const hp = 5000.0
	caster := func() *skillTarget { return &skillTarget{fakeActor: fakeActor{objectID: 1}, isPlayer: true} }
	use := func(target *skillTarget, skillType string) Result {
		t.Helper()
		result, ok := NewDefaultRegistry().UseResult(Cast{Caster: caster(), Skill: modelskill.Definition{SkillType: skillType}, Targets: []Actor{target}})
		if !ok {
			t.Fatalf("%s UseResult ok = false", skillType)
		}
		return result
	}
	wantReport := func(t *testing.T, result Result, amount int32, pcrit bool) {
		t.Helper()
		want := Damage{RecipientID: 1, Source: DamageByPlayer, Amount: amount, PhysicalCrit: pcrit}
		if got := damageMessages(t, result.Messages); len(got) != 1 || got[0] != want {
			t.Fatalf("damage messages = %#v, want %#v", got, want)
		}
	}

	t.Run("PDAM", func(t *testing.T) {
		target := &skillTarget{hp: hp, physicalOK: true, physicalInput: formulas.PhysicalSkillInput{
			AttackPower: 100, SkillPower: 50, Defence: 0,
			RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1, ElementalMul: 1,
		}}
		result := use(target, "PDAM")
		if !math.IsInf(target.hp, -1) {
			t.Fatalf("hp = %v, want the +Inf hit to take it to -Inf", target.hp)
		}
		wantReport(t, result, math.MaxInt32, false)
	})

	t.Run("PDAM 0/0 fails the attack", func(t *testing.T) {
		target := &skillTarget{hp: hp, physicalOK: true, physicalInput: formulas.PhysicalSkillInput{
			Defence: 0, RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1, ElementalMul: 1,
		}}
		result := use(target, "PDAM")
		if target.hp != hp || result.AttackFailed != 1 || len(damageMessages(t, result.Messages)) != 0 {
			t.Fatalf("hp = %v AttackFailed = %d messages = %#v, want untouched and ATTACK_FAILED", target.hp, result.AttackFailed, result.Messages)
		}
	})

	t.Run("MDAM", func(t *testing.T) {
		target := &skillTarget{hp: hp, magicOK: true, magicInput: formulas.MagicDamageInput{
			MAtk: 400, MDef: 0, SkillPower: 20, PvPMul: 1, ElementalMul: 1,
		}}
		result := use(target, "MDAM")
		if target.hp != hp-math.MaxInt32 {
			t.Fatalf("hp = %v, want %v", target.hp, hp-math.MaxInt32)
		}
		wantReport(t, result, math.MaxInt32, false)
	})

	blow := formulas.BlowInput{
		AttackPower: 100, SkillPower: 50, Defence: 0, RandomMul: 1, PosMul: 1,
		CritDamageMul: 1, CritDamagePosMul: 1, CritVulnMul: 1, DaggerVulnMul: 1, Landed: true,
	}
	t.Run("BLOW", func(t *testing.T) {
		target := &skillTarget{hp: hp, blowOK: true, blowInput: blow}
		result := use(target, "BLOW")
		if target.hp != hp-math.MaxInt32 {
			t.Fatalf("hp = %v, want %v", target.hp, hp-math.MaxInt32)
		}
		wantReport(t, result, math.MaxInt32, true)
	})

	t.Run("critical BLOW doubles past the int range", func(t *testing.T) {
		crit := blow
		crit.Crit = true
		target := &skillTarget{hp: hp, blowOK: true, blowInput: crit}
		result := use(target, "BLOW")
		if want := hp - 2*float64(math.MaxInt32); target.hp != want {
			t.Fatalf("hp = %v, want %v", target.hp, want)
		}
		wantReport(t, result, math.MaxInt32, true)
	})

	t.Run("MANADAM", func(t *testing.T) {
		target := &playerActor{skillTarget{mp: 100, maxMP: 100, manaOK: true, manaInput: formulas.ManaDamageInput{
			MAtk: 400, MDef: 0, SkillPower: 20, TargetMaxMp: 100, VulnMul: 1, Affected: true,
		}}}
		result, _ := NewDefaultRegistry().UseResult(Cast{Caster: &playerActor{skillTarget{name: "Caster"}}, Skill: modelskill.Definition{SkillType: "MANADAM"}, Targets: []Actor{target}})
		if target.mp != 0 {
			t.Fatalf("mp = %v, want the +Inf drain clamped to the 100 the target had", target.mp)
		}
		if len(result.ManaDrains) != 1 || result.ManaDrains[0].MP != 100 || len(result.OpponentMPReduced) != 1 || result.OpponentMPReduced[0] != 100 {
			t.Fatalf("drains = %#v reduced = %v, want 100 both", result.ManaDrains, result.OpponentMPReduced)
		}
	})
}
