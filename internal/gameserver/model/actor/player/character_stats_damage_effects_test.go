package player

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestReduceHPStopsSleepAndImmobileUntilAttackedEffects mirrors
// PlayerStatus.reduceHp's unconditional stopEffects(SLEEP)/
// stopEffects(IMMOBILE_UNTIL_ATTACKED) calls on non-HP-consumption damage.
func TestReduceHPStopsSleepAndImmobileUntilAttackedEffects(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(100)
	attachTestLive(t, c)
	addCharacterEffect(t, c, "Sleep")
	addCharacterEffect(t, c, "ImmobileUntilAttacked")

	c.ReduceHP(10, nil, modelskill.Definition{})

	if c.Sleeping() {
		t.Fatal("Sleeping() = true after ReduceHP, want the sleep effect stopped")
	}
	if c.ImmobileUntilAttacked() {
		t.Fatal("ImmobileUntilAttacked() = true after ReduceHP, want the effect stopped")
	}
}

// TestReduceHPStandsUpSittingCharacterUnlessInStoreMode mirrors the
// reference's isSitting() && !isInStoreMode() standUp() gate.
func TestReduceHPStandsUpSittingCharacterUnlessInStoreMode(t *testing.T) {
	tests := []struct {
		name        string
		operating   bool
		wantStanded bool
	}{
		{"stands up out of store mode", false, true},
		{"stays seated in store mode", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := liveCharacter(1, combatTemplate(), combatItems())
			c.SetHP(100)
			attachTestLive(t, c)
			c.Sit()
			c.SetOperating(tt.operating)

			c.ReduceHP(10, nil, modelskill.Definition{})

			if got := c.Standing(); got != tt.wantStanded {
				t.Fatalf("Standing() = %v, want %v", got, tt.wantStanded)
			}
		})
	}
}

// TestReduceHPBreaksStunOnOneInTenRollForNonDOTDamage mirrors
// !isDOT && isStunned() && Rnd.get(10) == 0.
func TestReduceHPBreaksStunOnOneInTenRollForNonDOTDamage(t *testing.T) {
	tests := []struct {
		name      string
		roll      int
		wantStun  bool
		wantAfter bool
	}{
		{"winning roll breaks stun", 0, true, false},
		{"losing roll leaves stun active", 1, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := liveCharacter(1, combatTemplate(), combatItems())
			c.SetHP(100)
			attachTestLive(t, c)
			addCharacterEffect(t, c, "Stun")
			c.SetRollSource(func(int) int { return tt.roll })

			c.ReduceHP(10, nil, modelskill.Definition{})

			if got := c.Stunned(); got != tt.wantAfter {
				t.Fatalf("Stunned() = %v, want %v", got, tt.wantAfter)
			}
		})
	}
}

// TestReduceHPByDOTNeverBreaksStunEvenOnWinningRoll mirrors the reference's
// !isDOT gate: DOT damage stops SLEEP/IMMOBILE_UNTIL_ATTACKED and stands the
// character up like any other hit, but never rolls to break STUN.
func TestReduceHPByDOTNeverBreaksStunEvenOnWinningRoll(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(100)
	attachTestLive(t, c)
	addCharacterEffect(t, c, "Stun")
	addCharacterEffect(t, c, "Sleep")
	c.SetRollSource(func(int) int { return 0 })

	c.ReduceHPByDOT(10, nil)

	if !c.Stunned() {
		t.Fatal("Stunned() = false after ReduceHPByDOT, want STUN untouched on DOT damage")
	}
	if c.Sleeping() {
		t.Fatal("Sleeping() = true after ReduceHPByDOT, want the sleep effect stopped")
	}
}

// TestReduceHPSkipsDamageEffectsOnAlreadyDeadCharacter mirrors the
// reference's top-of-method isDead() early return: an already-dead
// character (curHP already clamped to 0) must not have its SLEEP effect
// stopped or get stood up by a stray hit landing after death.
func TestReduceHPSkipsDamageEffectsOnAlreadyDeadCharacter(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(0)
	attachTestLive(t, c)
	addCharacterEffect(t, c, "Sleep")
	c.Sit()

	c.ReduceHP(10, nil, modelskill.Definition{})

	if !c.Sleeping() {
		t.Fatal("Sleeping() = false after ReduceHP on an already-dead character, want the sleep effect untouched")
	}
	if c.Standing() {
		t.Fatal("Standing() = true after ReduceHP on an already-dead character, want it left seated")
	}
}

// TestReduceHPByDOTSkipsDamageEffectsOnAlreadyDeadCharacter is
// ReduceHPByDOT's counterpart to the above.
func TestReduceHPByDOTSkipsDamageEffectsOnAlreadyDeadCharacter(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(0)
	attachTestLive(t, c)
	addCharacterEffect(t, c, "Sleep")
	c.Sit()

	c.ReduceHPByDOT(10, nil)

	if !c.Sleeping() {
		t.Fatal("Sleeping() = false after ReduceHPByDOT on an already-dead character, want the sleep effect untouched")
	}
	if c.Standing() {
		t.Fatal("Standing() = true after ReduceHPByDOT on an already-dead character, want it left seated")
	}
}

// TestTakeDamageAppliesNonConsumptionDamageEffects covers the melee-hit
// entrypoint, which reuses the same non-DOT hook as ReduceHP.
func TestTakeDamageAppliesNonConsumptionDamageEffects(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(100)
	attachTestLive(t, c)
	addCharacterEffect(t, c, "Sleep")
	c.Sit()

	c.TakeDamage(10, nil)

	if c.Sleeping() {
		t.Fatal("Sleeping() = true after TakeDamage, want the sleep effect stopped")
	}
	if !c.Standing() {
		t.Fatal("Standing() = false after TakeDamage, want the character stood up")
	}
}
