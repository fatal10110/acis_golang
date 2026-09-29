package summon

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// recordingCast is a CastControl that is casting while casting is set and
// records each cast-break roll, together with the summon's HP at the moment
// of the roll.
type recordingCast struct {
	a       *Actor
	casting bool
	breaks  bool
	rolls   []castBreakRoll
}

type castBreakRoll struct {
	damage float64
	roll   int
	immune bool
	hp     float64
}

func (c *recordingCast) CastingNow() bool        { return c.casting }
func (*recordingCast) CurrentSkillIsMagic() bool { return false }
func (*recordingCast) Now() time.Time            { return time.Time{} }
func (*recordingCast) Interrupt(time.Time) bool  { return false }
func (*recordingCast) StopCast()                 {}
func (c *recordingCast) InterruptCastOnDamage(damage float64, _ int, _ func(float64) float64, roll int, immune bool) bool {
	c.rolls = append(c.rolls, castBreakRoll{damage: damage, roll: roll, immune: immune, hp: c.a.HP()})
	if c.breaks && !immune {
		c.casting = false
		return true
	}
	return false
}

func newCastingPet(t *testing.T, breaks bool) (*Actor, *recordingCast) {
	t.Helper()
	a := mustPet(t, PetConfig{ObjectID: 2, Level: 10, Stats: CombatStats{MaxHP: 500}, Roll: func(int) int { return 7 }})
	c := &recordingCast{a: a, casting: true, breaks: breaks}
	a.Attach(Runtime{Cast: c})
	return a, c
}

// TestSummonSkillDamageRollsCastBreakBeforeHPChange pins the skill-damage
// order of the Pdam, Mdam, Blow and signet handlers: Formulas.calcCastBreak
// runs on the target before reduceCurrentHp, with the summon's own roll,
// whether or not the attacker may deal damage.
func TestSummonSkillDamageRollsCastBreakBeforeHPChange(t *testing.T) {
	for _, tt := range []struct {
		name   string
		denied bool
	}{
		{name: "attacker with damage permission"},
		{name: "attacker without damage permission", denied: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, c := newCastingPet(t, false)
			full := a.HP()
			attacker := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 10, Stats: CombatStats{MaxHP: 500}})
			var from attackable.Combatant = attacker
			if tt.denied {
				from = deniedLethalCaster{attacker}
			}

			a.ReduceHP(50, from, modelskill.Definition{})

			want := castBreakRoll{damage: 50, roll: 7, hp: full}
			if len(c.rolls) != 1 || c.rolls[0] != want {
				t.Fatalf("cast-break rolls = %+v, want [%+v]", c.rolls, want)
			}
			wantHP := full - 50
			if tt.denied {
				wantHP = full
			}
			if got := a.HP(); got != wantHP {
				t.Fatalf("HP = %v after the hit, want %v", got, wantHP)
			}
		})
	}
}

// TestSummonAutoAttackRollsCastBreakAfterHPChange pins the auto-attack
// order of CreatureAttack.doHit: the HP is reduced first, with no cast-break
// roll of its own; the attacker then rolls the break through
// BreakCastOnDamage. The attack controller skips the break on a killing hit
// (TestControllerReflectAbsorbOrder).
func TestSummonAutoAttackRollsCastBreakAfterHPChange(t *testing.T) {
	a, c := newCastingPet(t, false)
	full := a.HP()

	a.TakeDamage(50, nil)
	if len(c.rolls) != 0 {
		t.Fatalf("TakeDamage rolled cast breaks %+v, want none", c.rolls)
	}
	a.BreakCastOnDamage(50)

	want := castBreakRoll{damage: 50, roll: 7, hp: full - 50}
	if len(c.rolls) != 1 || c.rolls[0] != want {
		t.Fatalf("cast-break rolls = %+v, want [%+v]", c.rolls, want)
	}
}

// TestSummonDamageBreaksCastOnlyWhileCasting pins that a hit on a summon
// with no cast in flight draws no cast-break roll, and a breaking roll
// sends the summon idle.
func TestSummonDamageBreaksCastOnlyWhileCasting(t *testing.T) {
	a, c := newCastingPet(t, true)
	a.setIntent(IntentAttackTarget)

	a.ReduceHP(10, nil, modelskill.Definition{})
	if len(c.rolls) != 1 || c.casting {
		t.Fatalf("after the breaking hit: rolls = %+v, casting = %v; want one roll and the cast broken", c.rolls, c.casting)
	}
	if got := a.Intent(); got == IntentAttackTarget {
		t.Fatal("intent still attack after the cast broke, want the summon sent idle")
	}

	a.ReduceHP(10, nil, modelskill.Definition{})
	if len(c.rolls) != 1 {
		t.Fatalf("rolls = %+v after a hit with no cast in flight, want no new roll", c.rolls)
	}
}

// TestSummonBreakFirstPairRollsOncePerHit pins the split MDAM, DRAIN and the
// signet MDAM tick use on a summon target: BreakCastOnDamage rolls the cast
// break once on the raw damage without touching HP, ReduceHPWithoutCastBreak
// then applies the hit without a second roll, and neither does anything to
// a dead summon.
func TestSummonBreakFirstPairRollsOncePerHit(t *testing.T) {
	a, c := newCastingPet(t, false)
	full := a.HP()
	attacker := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 10, Stats: CombatStats{MaxHP: 500}})

	a.BreakCastOnDamage(50)

	want := castBreakRoll{damage: 50, roll: 7, hp: full}
	if len(c.rolls) != 1 || c.rolls[0] != want {
		t.Fatalf("BreakCastOnDamage rolls = %+v, want [%+v]", c.rolls, want)
	}
	if got := a.HP(); got != full {
		t.Fatalf("HP = %v after BreakCastOnDamage, want the untouched %v", got, full)
	}

	a.ReduceHPWithoutCastBreak(50, attacker, modelskill.Definition{})

	if len(c.rolls) != 1 {
		t.Fatalf("rolls = %+v after ReduceHPWithoutCastBreak, want no new roll", c.rolls)
	}
	if got := a.HP(); got != full-50 {
		t.Fatalf("HP = %v after ReduceHPWithoutCastBreak, want %v", got, full-50)
	}

	a.ReduceHPWithoutCastBreak(100_000, attacker, modelskill.Definition{})
	if !a.Dead() || a.HP() != 0 {
		t.Fatalf("dead/hp = %v/%v after a lethal ReduceHPWithoutCastBreak, want true/0", a.Dead(), a.HP())
	}
	if len(c.rolls) != 1 {
		t.Fatalf("rolls = %+v after the lethal hit, want no new roll", c.rolls)
	}

	// The recording cast keeps reporting a cast in flight, so only the dead
	// guard stops these.
	c.casting = true
	a.BreakCastOnDamage(50)
	a.ReduceHPWithoutCastBreak(50, attacker, modelskill.Definition{})
	if len(c.rolls) != 1 {
		t.Fatalf("rolls = %+v on a dead summon, want no new roll", c.rolls)
	}
	if got := a.HP(); got != 0 || !a.Dead() {
		t.Fatalf("dead/hp = %v/%v after hits on a dead summon, want true/0", a.Dead(), got)
	}
}
