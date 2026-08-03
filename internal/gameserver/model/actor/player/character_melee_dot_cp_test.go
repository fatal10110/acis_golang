package player

import "testing"

// TestTakeDamageDrainsCPBeforeHPForPlayableAttacker pins melee auto-attack
// (CreatureAttack.java:263 -> PlayerStatus.reduceHp, PlayerStatus.java:166-184)
// to the same CP-first absorption ReduceHP already applies to skill-cast
// damage (#1143).
func TestTakeDamageDrainsCPBeforeHPForPlayableAttacker(t *testing.T) {
	defender := liveCharacter(1, combatTemplate(), combatItems())
	defender.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 200})
	attacker := liveCharacter(2, combatTemplate(), combatItems())

	defender.TakeDamage(50, attacker)

	if defender.HP() != 500 {
		t.Fatalf("HP() = %v, want 500 (fully absorbed by CP)", defender.HP())
	}
	if defender.CP() != 150 {
		t.Fatalf("CP() = %v, want 150", defender.CP())
	}
}

func TestTakeDamageSpillsOverToHPOnceCPExhausted(t *testing.T) {
	defender := liveCharacter(1, combatTemplate(), combatItems())
	defender.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 30})
	attacker := liveCharacter(2, combatTemplate(), combatItems())

	defender.TakeDamage(50, attacker)

	if defender.CP() != 0 {
		t.Fatalf("CP() = %v, want 0", defender.CP())
	}
	if defender.HP() != 480 {
		t.Fatalf("HP() = %v, want 480 (20 dmg after CP absorbed 30)", defender.HP())
	}
}

func TestTakeDamageSkipsCPAbsorptionForSelfAttacker(t *testing.T) {
	defender := liveCharacter(1, combatTemplate(), combatItems())
	defender.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 200})

	defender.TakeDamage(50, defender)

	if defender.CP() != 200 {
		t.Fatalf("CP() = %v, want 200 unchanged for self-attacker", defender.CP())
	}
	if defender.HP() != 450 {
		t.Fatalf("HP() = %v, want 450", defender.HP())
	}
}

// TestReduceHPByDOTDrainsCPBeforeHPForPlayableAttacker pins DOT damage
// (EffectDamOverTime.java:48 -> PlayerStatus.reduceHp) to the same CP-first
// absorption, not gated on isDOT (#1143).
func TestReduceHPByDOTDrainsCPBeforeHPForPlayableAttacker(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 200})
	attacker := liveCharacter(2, combatTemplate(), combatItems())

	c.ReduceHPByDOT(50, attacker, true)

	if c.HP() != 500 {
		t.Fatalf("HP() = %v, want 500 (fully absorbed by CP)", c.HP())
	}
	if c.CP() != 150 {
		t.Fatalf("CP() = %v, want 150", c.CP())
	}
}

func TestReduceHPByDOTSpillsOverToHPOnceCPExhausted(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 30})
	attacker := liveCharacter(2, combatTemplate(), combatItems())

	c.ReduceHPByDOT(50, attacker, true)

	if c.CP() != 0 {
		t.Fatalf("CP() = %v, want 0", c.CP())
	}
	if c.HP() != 480 {
		t.Fatalf("HP() = %v, want 480 (20 dmg after CP absorbed 30)", c.HP())
	}
}

func TestReduceHPByDOTSkipsCPAbsorptionForNonPlayableAttacker(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 200})

	c.ReduceHPByDOT(50, reduceHPNpcAttacker{}, true)

	if c.CP() != 200 {
		t.Fatalf("CP() = %v, want 200 unchanged for non-Playable attacker", c.CP())
	}
	if c.HP() != 450 {
		t.Fatalf("HP() = %v, want 450", c.HP())
	}
}
