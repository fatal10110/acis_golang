package npc

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestHostileReduceHPStopsSleepAndImmobileUntilAttackedEffects mirrors
// NpcStatus.reduceHp's inherited CreatureStatus.reduceHp non-DOT block
// (CreatureStatus.java:228-248): a non-DOT hit stops SLEEP and
// IMMOBILE_UNTIL_ATTACKED.
func TestHostileReduceHPStopsSleepAndImmobileUntilAttackedEffects(t *testing.T) {
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	addHostileEffect(t, hostile, "Sleep")
	addHostileEffect(t, hostile, "ImmobileUntilAttacked")

	hostile.ReduceHP(10, nil, modelskill.Definition{})

	if hostile.Sleeping() {
		t.Fatal("Sleeping() = true after ReduceHP, want the sleep effect stopped")
	}
	if hostile.ImmobileUntilAttacked() {
		t.Fatal("ImmobileUntilAttacked() = true after ReduceHP, want the effect stopped")
	}
}

// TestHostileReduceHPByDOTLeavesEffectsAloneOnRealDOTTick mirrors the
// !isDOT gate on CreatureStatus.reduceHp's whole SLEEP/IMMOBILE/STUN block:
// NpcStatus has no PlayerStatus-style override, so a real DOT tick
// (isDOT=true) skips the block entirely.
func TestHostileReduceHPByDOTLeavesEffectsAloneOnRealDOTTick(t *testing.T) {
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	addHostileEffect(t, hostile, "Sleep")
	addHostileEffect(t, hostile, "ImmobileUntilAttacked")
	addHostileEffect(t, hostile, "Stun")
	hostile.SetRollSource(func(int) int { return 0 })

	hostile.ReduceHPByDOT(10, nil, true)

	if !hostile.Sleeping() {
		t.Fatal("Sleeping() = false after a DOT tick, want the sleep effect untouched")
	}
	if !hostile.ImmobileUntilAttacked() {
		t.Fatal("ImmobileUntilAttacked() = false after a DOT tick, want the effect untouched")
	}
	if !hostile.Stunned() {
		t.Fatal("Stunned() = false after a DOT tick, want the stun effect untouched")
	}
}

// TestHostileReduceHPByDOTAppliesEffectsWhenNotARealDOTTick mirrors
// drowning's WaterTaskManager.reduceCurrentHp(hp, player, false, false,
// null) call: isDOT=false periodic damage still runs the block.
func TestHostileReduceHPByDOTAppliesEffectsWhenNotARealDOTTick(t *testing.T) {
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	addHostileEffect(t, hostile, "Sleep")

	hostile.ReduceHPByDOT(10, nil, false)

	if hostile.Sleeping() {
		t.Fatal("Sleeping() = true after a non-DOT periodic hit, want the sleep effect stopped")
	}
}

// TestHostileTakeDamageStopsSleepAndImmobileUntilAttackedEffects mirrors the
// melee auto-attack path (CreatureAttack.java:263 -> NpcStatus.reduceHp),
// which is always non-DOT.
func TestHostileTakeDamageStopsSleepAndImmobileUntilAttackedEffects(t *testing.T) {
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	addHostileEffect(t, hostile, "Sleep")
	addHostileEffect(t, hostile, "ImmobileUntilAttacked")

	hostile.TakeDamage(10, nil)

	if hostile.Sleeping() {
		t.Fatal("Sleeping() = true after TakeDamage, want the sleep effect stopped")
	}
	if hostile.ImmobileUntilAttacked() {
		t.Fatal("ImmobileUntilAttacked() = true after TakeDamage, want the effect stopped")
	}
}

// TestHostileReduceHPBreaksStunOnOneInTenRollForNonDOTDamage mirrors
// !isDOT && isStunned() && Rnd.get(10) == 0.
func TestHostileReduceHPBreaksStunOnOneInTenRollForNonDOTDamage(t *testing.T) {
	tests := []struct {
		name      string
		roll      int
		wantAfter bool
	}{
		{"winning roll breaks stun", 0, false},
		{"losing roll leaves stun active", 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
			addHostileEffect(t, hostile, "Stun")
			hostile.SetRollSource(func(int) int { return tt.roll })

			hostile.ReduceHP(10, nil, modelskill.Definition{})

			if got := hostile.Stunned(); got != tt.wantAfter {
				t.Fatalf("Stunned() = %v, want %v", got, tt.wantAfter)
			}
		})
	}
}
