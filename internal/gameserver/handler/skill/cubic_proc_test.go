package skill

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// cubicProcOwner is a cubic's owner: a player whose rolls come from rolls
// in order (n-1, the best roll, once they run out), with its own M.Atk and
// level.
type cubicProcOwner struct {
	skillTarget
	level int
	mAtk  float64
	rolls []int
}

func newCubicProcOwner() *cubicProcOwner {
	return &cubicProcOwner{skillTarget: skillTarget{fakeActor: fakeActor{objectID: 1}, isPlayer: true, name: "Owner", hp: 100, maxHP: 1000}, level: 80, mAtk: 5000}
}

func (o *cubicProcOwner) Level() int    { return o.level }
func (o *cubicProcOwner) MAtk() float64 { return o.mAtk }
func (o *cubicProcOwner) Roll(n int) int {
	if len(o.rolls) == 0 {
		return n - 1
	}
	r := o.rolls[0]
	o.rolls = o.rolls[1:]
	return r
}

// cubicProcTarget is a cubic's target with a fixed M.Def and level.
type cubicProcTarget struct {
	skillTarget
	level        int
	mDef         float64
	perfectBlock bool
	attackable   bool
	aggression   []int
}

func newCubicProcTarget() *cubicProcTarget {
	return &cubicProcTarget{skillTarget: skillTarget{fakeActor: fakeActor{objectID: 2}, name: "Target", hp: 100000, maxHP: 100000, effects: newTestList(nil)}, level: 40, mDef: 385}
}

func (t *cubicProcTarget) Level() int    { return t.level }
func (t *cubicProcTarget) MDef() float64 { return t.mDef }
func (t *cubicProcTarget) Attackable() bool {
	return t.attackable
}

func (t *cubicProcTarget) NotifyAggression(_ attackable.Combatant, power int) {
	t.aggression = append(t.aggression, power)
}

func (t *cubicProcTarget) ShieldDefense(creature.FormulaActor, modelskill.Definition, bool) formulas.ShieldDefense {
	if t.perfectBlock {
		return formulas.ShieldPerfect
	}
	return formulas.ShieldFailed
}

func useCubic(t *testing.T, owner *cubicProcOwner, target *cubicProcTarget, def modelskill.Definition, mAtk float64) Result {
	t.Helper()
	result, ok := NewDefaultRegistry().UseCubic(Cast{Caster: owner, Skill: def, Targets: []Actor{target}}, mAtk)
	if !ok {
		t.Fatalf("UseCubic(%s) not handled", def.SkillType)
	}
	return result
}

// stormStrike is skill 4049 at level 8 (Cubic Drain, MDAM).
var stormStrike = modelskill.Definition{ID: 4049, Level: 8, SkillType: "MDAM", Power: 2399, MagicLevel: 74, Offensive: true}

// TestCubicMdamDamageComesFromTheCubicFormula pins Cubic.useMdamSkill: the
// damage is 91 / M.Def * power (Formulas.calcMagicDam(Cubic), with no owner
// M.Atk term), the target's cast break is rolled, the damage is reported to
// the owner, then the target takes it, and no shot is spent.
func TestCubicMdamDamageComesFromTheCubicFormula(t *testing.T) {
	owner := newCubicProcOwner()
	target := newCubicProcTarget()
	result := useCubic(t, owner, target, stormStrike, 1975)

	const want = 567 // int(91 / 385 * 2399), from the Java probe
	if got := target.maxHP - target.hp; got != want {
		t.Fatalf("target lost %v HP, want %d", got, want)
	}
	if !slices.Equal(target.hitLog, []string{"cast break", "hp -567 with 0 effects"}) {
		t.Fatalf("hit log = %v, want the cast break before the HP", target.hitLog)
	}
	if !slices.Equal(result.Messages, []any{Damage{RecipientID: owner.objectID, Amount: want}}) {
		t.Fatalf("messages = %+v, want the owner's damage report only", result.Messages)
	}
	if len(owner.shots) != 0 {
		t.Fatalf("owner shot writes = %v, want none", owner.shots)
	}
}

// TestCubicMdamCriticalQuadruples: an owner magic critical multiplies the
// cubic damage by 4.
func TestCubicMdamCriticalQuadruples(t *testing.T) {
	owner := newCubicProcOwner()
	owner.rolls = []int{0} // the magic critical roll; the rate below is 1 in 1000
	critOwner := &critCubicOwner{cubicProcOwner: owner}
	target := newCubicProcTarget()
	result, _ := NewDefaultRegistry().UseCubic(Cast{Caster: critOwner, Skill: stormStrike, Targets: []Actor{target}}, 0)

	const want = 2268
	if got := target.maxHP - target.hp; got != want {
		t.Fatalf("target lost %v HP, want %d", got, want)
	}
	if !slices.Equal(result.Messages, []any{Damage{RecipientID: owner.objectID, Amount: want, MagicCrit: true}}) {
		t.Fatalf("messages = %+v, want a critical damage report", result.Messages)
	}
}

type critCubicOwner struct{ *cubicProcOwner }

func (critCubicOwner) MagicCriticalRate() float64 { return 1 }

// TestCubicMdamReflectDealsNothing: a reflected strike deals no damage and
// reports nothing.
func TestCubicMdamReflectDealsNothing(t *testing.T) {
	target := newCubicProcTarget()
	target.reflects = true
	result := useCubic(t, newCubicProcOwner(), target, stormStrike, 0)
	if target.hp != target.maxHP || len(result.Messages) != 0 {
		t.Fatalf("reflected strike: HP %v/%v, messages %+v; want no damage and no report", target.hp, target.maxHP, result.Messages)
	}
}

// TestCubicMagicFailureGapUsesSkillMagicLevel pins the cubic failure
// branch: the half-resist gap is the target level minus the skill's magic
// level, not the owner's level.
func TestCubicMagicFailureGapUsesSkillMagicLevel(t *testing.T) {
	for _, tt := range []struct {
		name       string
		magicLevel int
		wantHP     float64
		wantMsg    any
	}{
		// Target 50 - magic level 45 = 5: half damage, ATTACK_FAILED.
		{"half", 45, 567 / 2, AttackFailedMessage{}},
		// Target 50 - magic level 40 = 10, although the owner is level 80:
		// a full resist, damage 1.
		{"full", 40, 1, Resisted{TargetName: "Target", SkillID: 4049, SkillLevel: 8}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			owner := newCubicProcOwner()
			owner.rolls = []int{999, 0, 9999} // no critical; first check fails, second passes
			target := newCubicProcTarget()
			target.level = 50
			def := stormStrike
			def.MagicLevel = tt.magicLevel
			result := useCubic(t, owner, target, def, 0)
			if got := target.maxHP - target.hp; got != float64(int(tt.wantHP)) {
				t.Fatalf("target lost %v HP, want %v", got, int(tt.wantHP))
			}
			if len(result.Messages) == 0 || result.Messages[0] != tt.wantMsg {
				t.Fatalf("messages = %+v, want %+v first", result.Messages, tt.wantMsg)
			}
		})
	}
}

// TestCubicMdamEffectsLandAfterTheDamageReport: a strike with effects
// drops the target's own copy of the skill's effects and lands them when
// the cubic landing roll passes, before the target takes the HP.
func TestCubicMdamEffectsLandAfterTheDamageReport(t *testing.T) {
	target := newCubicProcTarget()
	def := stormStrike
	def.IgnoreResists, def.EffectPower = true, 100
	def.Effects = []modelskill.EffectTemplate{{Name: "Debuff", Time: 30, EffectPower: -1}}
	useCubic(t, newCubicProcOwner(), target, def, 0)
	if !slices.Equal(target.hitLog, []string{"cast break", "hp -567 with 1 effects"}) {
		t.Fatalf("hit log = %v, want the effect landed before the HP", target.hitLog)
	}
}

// TestCubicLandingUsesCubicMAtk pins Formulas.calcCubicSkillSuccess's magic
// term: the cubic's own M.Atk, x4 under the owner's blessed spiritshot,
// against the target's M.Def; the owner's M.Atk plays no part. The base
// chance is the skill's landing power.
func TestCubicLandingUsesCubicMAtk(t *testing.T) {
	owner := newCubicProcOwner()
	target := newCubicProcTarget()
	def := modelskill.Definition{SkillType: "DEBUFF", Power: 80, Magic: true, LevelDepend: 2, MagicLevel: 40}
	for _, bss := range []bool{false, true} {
		in, ok := creature.ResolveCubicSkillSuccessInput(owner, target, def, 1975, bss, formulas.ShieldFailed)
		if !ok {
			t.Fatal("ResolveCubicSkillSuccessInput() not ok")
		}
		if want := formulas.CubicMAtkModifier(1975, 385, bss); in.MAtkModifier != want {
			t.Fatalf("bss=%v: MAtkModifier = %v, want %v", bss, in.MAtkModifier, want)
		}
		if in.BaseChance != 80 {
			t.Fatalf("BaseChance = %v, want the landing power 80", in.BaseChance)
		}
	}
}

// TestCubicContinuousFailedRollTellsOwner: an offensive cubic POISON,
// DEBUFF or DOT that fails its landing roll (here a perfect block) tells
// the owner the attack failed and lands nothing.
func TestCubicContinuousFailedRollTellsOwner(t *testing.T) {
	target := newCubicProcTarget()
	target.perfectBlock = true
	def := modelskill.Definition{
		ID: 4052, Level: 6, SkillType: "POISON", Power: 70, Magic: true, Offensive: true, Debuff: true,
		Effects: []modelskill.EffectTemplate{{Name: "DamOverTime", Count: 10, Time: 3, Value: 144}},
	}
	result := useCubic(t, newCubicProcOwner(), target, def, 1975)
	if result.AttackFailed != 1 || !slices.Equal(result.Messages, []any{AttackFailedMessage{}}) {
		t.Fatalf("AttackFailed = %d, messages = %+v; want one ATTACK_FAILED", result.AttackFailed, result.Messages)
	}
	if n := len(target.effects.All()); n != 0 {
		t.Fatalf("target holds %d effects, want none", n)
	}
}

// TestCubicDisablerFailureIsSilent: a cubic disabler that fails its
// landing roll tells nobody, unlike the owner's own cast.
func TestCubicDisablerFailureIsSilent(t *testing.T) {
	target := newCubicProcTarget()
	target.perfectBlock = true
	def := modelskill.Definition{
		ID: 4164, Level: 9, SkillType: "PARALYZE", Power: 15, Magic: true, Offensive: true, Debuff: true,
		Effects: []modelskill.EffectTemplate{{Name: "Paralyze", Time: 120}},
	}
	result := useCubic(t, newCubicProcOwner(), target, def, 1975)
	if len(result.Messages) != 0 || len(result.Resisted) != 0 || len(target.effects.All()) != 0 {
		t.Fatalf("messages %+v, resisted %+v, effects %d; want a silent miss", result.Messages, result.Resisted, len(target.effects.All()))
	}
}

// TestCubicAggDamageProvokesNPC pins Cubic.useDisablerSkill's AGGDAMAGE
// aggression: (int) (150 * power / (level + 7)), sent once the landing roll
// passes.
func TestCubicAggDamageProvokesNPC(t *testing.T) {
	target := newCubicProcTarget()
	target.attackable = true
	target.level = 20
	def := modelskill.Definition{ID: 5115, Level: 1, SkillType: "AGGDAMAGE", Power: 80, IgnoreResists: true, EffectPower: 100}
	useCubic(t, newCubicProcOwner(), target, def, 0)
	if !slices.Equal(target.aggression, []int{444}) { // from the Java probe
		t.Fatalf("aggression = %v, want [444]", target.aggression)
	}
}

// TestCubicDrainFeedsOwnerFromFullDamage pins Cubic.useDrainSkill: the
// owner regains absorbAbs + absorbPart * damage on the full damage, even
// against a player target whose CP would absorb it; the target takes the
// HP, then rolls its cast break, then the damage is reported.
func TestCubicDrainFeedsOwnerFromFullDamage(t *testing.T) {
	owner := newCubicProcOwner()
	target := newCubicProcTarget()
	target.isPlayer = true
	target.cp, target.maxCP = 5000, 5000
	def := modelskill.Definition{ID: 4050, Level: 7, SkillType: "DRAIN", Power: 5221, Magic: true, MagicLevel: 72, Offensive: true, AbsorbAbs: 15, AbsorbPart: 0.4}
	result := useCubic(t, owner, target, def, 0)

	const damage = 1234 // int(91 / 385 * 5221)
	// 15 + 0.4f * 1234 in single precision, from the Java probe.
	if want := 100 + 508.6000061035156; owner.hp != want {
		t.Fatalf("owner HP = %v, want %v", owner.hp, want)
	}
	if !slices.Equal(target.hitLog, []string{"hp -1234 with 0 effects", "cast break"}) {
		t.Fatalf("hit log = %v, want the HP before the cast break", target.hitLog)
	}
	want := []any{CasterVitalsChanged{}, Damage{RecipientID: owner.objectID, Amount: damage}}
	if !slices.Equal(result.Messages, want) {
		t.Fatalf("messages = %+v, want %+v", result.Messages, want)
	}
	if len(owner.shots) != 0 {
		t.Fatalf("owner shot writes = %v, want none", owner.shots)
	}
}

var _ effect.Actor = (*cubicProcTarget)(nil)
