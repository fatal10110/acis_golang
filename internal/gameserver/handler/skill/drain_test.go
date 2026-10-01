package skill

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// TestDrainAbsorbMatchesReference pins the DRAIN absorb amount against
// literal outputs of a probe that replays L2SkillDrain.useSkill's absorb
// block (L2SkillDrain.java:60-79) with the reference's own types: int casts
// of the target's CP and HP, and absorbAbs + absorbPart * drain evaluated
// in float before widening to addHp's double. Probe run on
// eclipse-temurin:21; the want column is Double.doubleToLongBits.
func TestDrainAbsorbMatchesReference(t *testing.T) {
	for _, tc := range []struct {
		playable, targetPlayer bool
		cp, hp                 float64
		damage, abs            int
		part                   float32
		want                   uint64
	}{
		{true, false, 0, 5000, 7, 0, 0.8, 4617991057798332416},
		{true, false, 0, 5000, 123, 0, 0.8, 4636624701471326208},
		{true, false, 0, 5000, 333, 0, 0.35, 4637901893748654080},
		{true, false, 0, 5000, 1001, 0, 0.2, 4641247927749050368},
		{true, false, 0, 5000, 77, 0, 0.4, 4629362647286939648},
		{true, false, 0, 99.9, 150, 0, 0.8, 4635273621797863424},
		{true, false, 0, 5000, 50, 105, 0, 4637089135075524608},
		{true, false, 0, 30, 50, 260, 0, 4643281584563159040},
		{true, true, 100.7, 900, 60, 0, 0.8, 0},
		{true, true, 100.7, 900, 100, 0, 0.8, 0},
		{true, true, 100.7, 900, 257, 0, 0.8, 4638538731098210304},
		{true, true, 0.4, 900, 257, 0, 0.8, 4641437923680452608},
		{true, true, 0, 120.5, 257, 0, 0.8, 4636455816377925632},
		{false, true, 100.7, 900, 257, 0, 0.8, 4641437923680452608},
		{false, true, 100.7, 200.9, 257, 0, 1, 4641240890982006784},
		{false, false, 0, 8000, 4321, 0, 0.2, 4650812799516147712},
		{true, false, 0, 16777217, 16777217, 0, 1, 4715268809856909312},
		{true, false, 0, 5000, 3, 7, 0.1, 4619905087962087424},
	} {
		caster := &skillTarget{fakeActor: fakeActor{objectID: 1}, isPlayer: tc.playable}
		target := &skillTarget{fakeActor: fakeActor{objectID: 2}, isPlayer: tc.targetPlayer, cp: tc.cp, hp: tc.hp}
		cast := Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "DRAIN", AbsorbAbs: tc.abs, AbsorbPart: tc.part}}
		if got := drainAbsorb(cast, target, tc.damage); math.Float64bits(got) != tc.want {
			t.Errorf("drainAbsorb(%+v) = %v, want %v", tc, got, math.Float64frombits(tc.want))
		}
	}
}

func drainFixture() (caster, target *skillTarget, def modelskill.Definition) {
	caster = &skillTarget{
		fakeActor: fakeActor{objectID: 1}, hp: 100, maxHP: 5000, isPlayer: true, name: "Caster",
		effects: newTestList(nil), charged: map[item.ShotKind]bool{item.ShotBlessedSpirit: true},
	}
	target = &skillTarget{
		fakeActor: fakeActor{objectID: 2}, hp: 5000, maxHP: 5000, name: "Target",
		effects:    newTestList(nil),
		magicInput: formulas.MagicDamageInput{MAtk: 400, MDef: 50, SkillPower: 20, PvPMul: 1, ElementalMul: 1},
		magicOK:    true, skillSuccessOK: true,
	}
	def = modelskill.Definition{ID: 1090, Level: 1, SkillType: "DRAIN", Target: modelskill.TargetOne, AbsorbPart: 0.8}
	return caster, target, def
}

// TestDrainDamagesTargetAndFeedsCaster pins the DRAIN hit on a live target:
// the caster regains 80% of the damage, the target's cast break is rolled
// before the damage report and its effects, the HP loss comes last and
// skips a second break, and the charged blessed spiritshot is spent with
// the static-reuse flag.
func TestDrainDamagesTargetAndFeedsCaster(t *testing.T) {
	caster, target, def := drainFixture()
	def.Effects = []modelskill.EffectTemplate{{Name: "Debuff", Time: 9}}
	damage := int(formulas.MagicDamage(target.magicInput))

	result, ok := NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
	if !ok {
		t.Fatal("UseResult() handled = false, want true for DRAIN")
	}
	if want := 100 + float64(float32(0.8)*float32(damage)); caster.hp != want {
		t.Fatalf("caster hp = %v, want %v", caster.hp, want)
	}
	if want := 5000 - float64(damage); target.hp != want {
		t.Fatalf("target hp = %v, want %v", target.hp, want)
	}
	wantLog := []string{"cast break", fmt.Sprintf("hp -%v with 1 effects", float64(damage))}
	if !slices.Equal(target.hitLog, wantLog) || !slices.Equal(target.castBreakDamage, []float64{float64(damage)}) {
		t.Fatalf("target hits = %q (breaks %v), want %q", target.hitLog, target.castBreakDamage, wantLog)
	}
	if got := damageMessages(t, result.Messages); len(got) != 1 || got[0].Amount != int32(damage) || got[0].RecipientID != 1 {
		t.Fatalf("damage messages = %+v, want one %d-damage report to the caster", got, damage)
	}
	if len(result.Messages) < 2 || result.Messages[0] != any(CasterVitalsChanged{}) {
		t.Fatalf("messages = %+v, want the caster's status change ahead of its damage report", result.Messages)
	}
	if landed := target.effects.All(); len(landed) != 1 || landed[0].Effector != effect.Actor(caster) {
		t.Fatalf("target effects = %+v, want the Debuff from the caster", landed)
	}
	if !slices.Equal(caster.shots, []item.ShotKind{item.ShotBlessedSpirit}) || !slices.Equal(caster.shotFlags, []bool{false}) {
		t.Fatalf("shots = %v %v, want blessed spiritshot spent", caster.shots, caster.shotFlags)
	}
}

// TestDrainAtFullHPReportsNoCasterStatus pins the absorb's status gate: a
// caster already at full HP gains nothing and has no status to report.
func TestDrainAtFullHPReportsNoCasterStatus(t *testing.T) {
	caster, target, def := drainFixture()
	caster.hp = caster.maxHP
	result, _ := NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
	if caster.hp != caster.maxHP || slices.Contains(result.Messages, any(CasterVitalsChanged{})) {
		t.Fatalf("caster hp %v, messages %+v; want full HP and no status change", caster.hp, result.Messages)
	}
}

// TestDrainNetsAPlayerTargetsCP pins the drain basis: a playable caster
// drains only the damage past a player target's CP, and nothing when the CP
// soaks it all.
func TestDrainNetsAPlayerTargetsCP(t *testing.T) {
	for _, tc := range []struct {
		cp   float64
		want float64
	}{{10000, 100}, {100.7, 100 + float64(float32(0.8)*float32(728-100))}} {
		caster, target, def := drainFixture()
		target.isPlayer, target.cp = true, tc.cp
		NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
		if caster.hp != tc.want {
			t.Fatalf("cp %v: caster hp = %v, want %v", tc.cp, caster.hp, tc.want)
		}
	}
}

// TestDrainGates pins which targets a DRAIN skips outright and what a
// corpse drain still does.
func TestDrainGates(t *testing.T) {
	t.Run("alike-dead caster", func(t *testing.T) {
		caster, target, def := drainFixture()
		caster.alikeDead = true
		NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
		if target.hp != 5000 || caster.hp != 100 || len(caster.shots) != 0 {
			t.Fatalf("target hp %v, caster hp %v, shots %v; want nothing done", target.hp, caster.hp, caster.shots)
		}
	})
	t.Run("alike-dead target", func(t *testing.T) {
		caster, target, def := drainFixture()
		target.alikeDead = true
		NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
		if target.hp != 5000 || caster.hp != 100 {
			t.Fatalf("target hp %v, caster hp %v; want the target skipped", target.hp, caster.hp)
		}
		if !slices.Equal(caster.shots, []item.ShotKind{item.ShotBlessedSpirit}) {
			t.Fatalf("shots = %v, want the cast still spends its shot", caster.shots)
		}
	})
	t.Run("invulnerable target", func(t *testing.T) {
		caster, target, def := drainFixture()
		guarded := &guardedSkillTarget{skillTarget: target, invul: true}
		NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{guarded}})
		if target.hp != 5000 || caster.hp != 100 {
			t.Fatalf("target hp %v, caster hp %v; want the target skipped", target.hp, caster.hp)
		}
	})
	t.Run("invulnerable caster on itself", func(t *testing.T) {
		caster, target, def := drainFixture()
		target.fakeActor = caster.fakeActor
		guarded := &guardedSkillTarget{skillTarget: target, invul: true}
		NewDefaultRegistry().UseResult(Cast{Caster: guarded, Skill: def, Targets: []Actor{guarded}})
		if target.hp >= 5000 {
			t.Fatalf("self-target hp = %v, want the drain to land", target.hp)
		}
	})
	t.Run("corpse drain feeds the caster only", func(t *testing.T) {
		caster, target, def := drainFixture()
		target.dead = true
		def.Target, def.AbsorbPart, def.AbsorbAbs = modelskill.TargetCorpseMob, 0, 260
		def.Effects = []modelskill.EffectTemplate{{Name: "Debuff", Time: 9}}
		result, _ := NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
		if caster.hp != 360 {
			t.Fatalf("caster hp = %v, want 360", caster.hp)
		}
		if target.hp != 5000 || len(target.hitLog) != 0 || len(target.effects.All()) != 0 || len(damageMessages(t, result.Messages)) != 0 {
			t.Fatalf("corpse hp %v, hits %q, effects %d, messages %+v; want untouched and unreported", target.hp, target.hitLog, len(target.effects.All()), result.Messages)
		}
	})
	t.Run("corpse-mob drain on a live target lands no effects", func(t *testing.T) {
		caster, target, def := drainFixture()
		target.alikeDead = true
		def.Target = modelskill.TargetCorpseMob
		def.Effects = []modelskill.EffectTemplate{{Name: "Debuff", Time: 9}}
		result, _ := NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
		if target.hp >= 5000 || len(damageMessages(t, result.Messages)) != 1 {
			t.Fatalf("target hp %v, messages %+v; want the hit reported", target.hp, result.Messages)
		}
		if len(target.effects.All()) != 0 {
			t.Fatalf("target effects = %+v, want none from a CORPSE_MOB skill", target.effects.All())
		}
	})
	t.Run("no damage", func(t *testing.T) {
		caster, target, def := drainFixture()
		target.magicOK = false
		NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
		if target.hp != 5000 || caster.hp != 100 || len(target.hitLog) != 0 {
			t.Fatalf("target hp %v, caster hp %v, hits %q; want nothing", target.hp, caster.hp, target.hitLog)
		}
	})
}

// TestDrainEffectRollResistsAtLevelOne pins the unreflected landing: the
// skill-level roll fails and the caster hears it at level 1, the id-only
// addSkillName overload, from the skill's own unconditional send.
func TestDrainEffectRollResistsAtLevelOne(t *testing.T) {
	caster, target, def := drainFixture()
	def.Level = 6
	def.Effects = []modelskill.EffectTemplate{{Name: "Debuff", Time: 9}}
	target.skillSuccessChance = chanceOf(0)
	result, _ := NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
	want := []Resisted{{TargetName: "Target", SkillID: 1090, SkillLevel: 1, Unconditional: true}}
	if !slices.Equal(result.Resisted, want) || len(target.effects.All()) != 0 {
		t.Fatalf("Resisted = %+v, effects %d; want %+v and none landed", result.Resisted, len(target.effects.All()), want)
	}
}

// TestDrainMagicFailureUsesDrainMessages pins calcMagicDam's DRAIN wording:
// a half failure reports DRAIN_HALF_SUCCESFUL instead of ATTACK_FAILED, and
// a player target hears RESISTED_S1_DRAIN instead of RESISTED_S1_MAGIC.
func TestDrainMagicFailureUsesDrainMessages(t *testing.T) {
	caster, target, def := drainFixture()
	target.isPlayer = true
	target.magicInput.Failure = formulas.MagicFailureHalf
	result, _ := NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
	if result.AttackFailed != 0 {
		t.Fatalf("AttackFailed = %d, want 0 for a drain", result.AttackFailed)
	}
	if len(result.Messages) < 2 || result.Messages[0] != any(DrainHalfSucceededMessage{}) || result.Messages[1] != any(MagicResist{TargetID: 2, AttackerName: "Caster", Drain: true}) {
		t.Fatalf("messages = %+v, want DrainHalfSucceeded then a drain MagicResist", result.Messages)
	}
}
