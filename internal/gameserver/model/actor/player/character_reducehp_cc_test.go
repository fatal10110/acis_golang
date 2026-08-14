package player

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// reduceHPPlayableAttacker is a minimal Playable-attacker stub for CP
// absorption tests (a distinct actor from the target, unlike self-damage).
type reduceHPPlayableAttacker struct{}

func (reduceHPPlayableAttacker) ObjectID() int32 { return 99 }
func (reduceHPPlayableAttacker) Dead() bool      { return false }
func (reduceHPPlayableAttacker) Playable() bool  { return true }

// reduceHPNpcAttacker is a non-Playable attacker stub.
type reduceHPNpcAttacker struct{}

func (reduceHPNpcAttacker) ObjectID() int32 { return 98 }
func (reduceHPNpcAttacker) Dead() bool      { return false }
func (reduceHPNpcAttacker) Playable() bool  { return false }

func TestReduceHPDrainsCPBeforeHPForPlayableAttacker(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 200})

	c.ReduceHP(50, reduceHPPlayableAttacker{}, modelskill.Definition{})

	if c.HP() != 500 {
		t.Fatalf("HP() = %v, want 500 (fully absorbed by CP)", c.HP())
	}
	if c.CP() != 150 {
		t.Fatalf("CP() = %v, want 150", c.CP())
	}
}

func TestReduceHPSpillsOverToHPOnceCPExhausted(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 30})

	c.ReduceHP(50, reduceHPPlayableAttacker{}, modelskill.Definition{})

	if c.CP() != 0 {
		t.Fatalf("CP() = %v, want 0", c.CP())
	}
	if c.HP() != 480 {
		t.Fatalf("HP() = %v, want 480 (20 dmg after CP absorbed 30)", c.HP())
	}
}

func TestReduceHPSkipsCPAbsorptionForNonPlayableAttacker(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 200})

	c.ReduceHP(50, reduceHPNpcAttacker{}, modelskill.Definition{})

	if c.CP() != 200 {
		t.Fatalf("CP() = %v, want 200 unchanged", c.CP())
	}
	if c.HP() != 450 {
		t.Fatalf("HP() = %v, want 450", c.HP())
	}
}

func TestReduceHPSkipsCPAbsorptionForSelfAttacker(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 200})

	c.ReduceHP(50, c, modelskill.Definition{})

	if c.CP() != 200 {
		t.Fatalf("CP() = %v, want 200 unchanged for self-attacker (matches PlayerStatus.java's attacker != _actor gate)", c.CP())
	}
	if c.HP() != 450 {
		t.Fatalf("HP() = %v, want 450", c.HP())
	}
}

func TestReduceHPSkipsCPAbsorptionForDirectHPDamageSkill(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 200})

	c.ReduceHP(50, reduceHPPlayableAttacker{}, modelskill.Definition{DirectHPDamage: true})

	if c.CP() != 200 {
		t.Fatalf("CP() = %v, want 200 unchanged for a dmgDirectlyToHp skill (matches PlayerStatus.reduceHp's ignoreCP=skill.getDmgDirectlyToHP())", c.CP())
	}
	if c.HP() != 450 {
		t.Fatalf("HP() = %v, want 450", c.HP())
	}
}

// TestReduceHPBreaksCastOnRawDamageNotCPAbsorbedRemainder pins
// Formulas.calcCastBreak's contract: every Java call site passes the skill's
// raw computed damage, never a CP-reduced remainder. A fully CP-absorbed hit
// (HP untouched) must still forward the full raw damage to the cast
// controller.
func TestReduceHPBreaksCastOnRawDamageNotCPAbsorbedRemainder(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 200})
	c.SetRollSource(func(int) int { return 42 })
	spy := &spyCastController{casting: true, magic: true}
	c.SetCastController(spy)

	c.ReduceHP(50, reduceHPPlayableAttacker{}, modelskill.Definition{})

	if len(spy.damageCalls) != 1 {
		t.Fatalf("InterruptCastOnDamage calls = %d, want 1", len(spy.damageCalls))
	}
	if got := spy.damageCalls[0].damage; got != 50 {
		t.Fatalf("damage = %v, want 50 (raw damage, not the CP-absorbed remainder of 0)", got)
	}
}
