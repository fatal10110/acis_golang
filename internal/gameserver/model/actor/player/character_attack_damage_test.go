package player

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// ---- from character_melee_dot_cp_test.go ----
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

	c.ReduceHPByDOT(50, &reduceHPNpcAttacker{}, true)

	if c.CP() != 200 {
		t.Fatalf("CP() = %v, want 200 unchanged for non-Playable attacker", c.CP())
	}
	if c.HP() != 450 {
		t.Fatalf("HP() = %v, want 450", c.HP())
	}
}

func TestReduceHPDrainsCPBeforeHPForPlayableAttacker(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 200})

	c.ReduceHP(50, &reduceHPPlayableAttacker{}, modelskill.Definition{})

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

	c.ReduceHP(50, &reduceHPPlayableAttacker{}, modelskill.Definition{})

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

	c.ReduceHP(50, &reduceHPNpcAttacker{}, modelskill.Definition{})

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

	c.ReduceHP(50, &reduceHPPlayableAttacker{}, modelskill.Definition{DirectHPDamage: true})

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

	c.ReduceHP(50, &reduceHPPlayableAttacker{}, modelskill.Definition{})

	if len(spy.damageCalls) != 1 {
		t.Fatalf("InterruptCastOnDamage calls = %d, want 1", len(spy.damageCalls))
	}
	if got := spy.damageCalls[0].damage; got != 50 {
		t.Fatalf("damage = %v, want 50 (raw damage, not the CP-absorbed remainder of 0)", got)
	}
}

// ---- from character_stats_damage_effects_test.go ----
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
		operate     privatestore.OperateType
		wantStanded bool
	}{
		{"stands up out of store mode", privatestore.OperateNone, true},
		{"stands up while setting a store up", privatestore.OperateSellManage, true},
		{"stays seated in store mode", privatestore.OperateSell, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := liveCharacter(1, combatTemplate(), combatItems())
			c.SetHP(100)
			attachTestLive(t, c)
			sitDownSettled(c)
			c.SetOperateType(tt.operate)

			c.ReduceHP(10, nil, modelskill.Definition{})

			if got := c.Standing(); got != tt.wantStanded {
				t.Fatalf("Standing() = %v, want %v", got, tt.wantStanded)
			}
		})
	}
}

// sitDownSettled sits c down and ends the sit-down at once, the way its
// timer would.
func sitDownSettled(c *Character) {
	c.Sit()
	c.settlePosture(false)
}

// TestDamageLeavesSitDownAndFakeDeathLieDownRunning pins the stand-up gate
// to a finished sit-down: a hit or a damage-over-time tick during the
// sit-down or the fake-death lie-down leaves the transition running and the
// character down, with no standing broadcast.
func TestDamageLeavesSitDownAndFakeDeathLieDownRunning(t *testing.T) {
	for _, tc := range []struct {
		name string
		down func(*Character)
		hit  func(*Character)
	}{
		{"sit-down, hit", func(c *Character) { c.Sit() }, func(c *Character) { c.TakeDamage(10, nil) }},
		{"sit-down, skill", func(c *Character) { c.Sit() }, func(c *Character) { c.ReduceHP(10, nil, modelskill.Definition{}) }},
		{"sit-down, damage over time", func(c *Character) { c.Sit() }, func(c *Character) { c.ReduceHPByDOT(10, c, true) }},
		{"fake-death lie-down, hit", func(c *Character) { c.StartFakeDeath() }, func(c *Character) { c.TakeDamage(10, nil) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := liveCharacter(1, combatTemplate(), combatItems())
			c.SetHP(100)
			attachTestLive(t, c)
			tc.down(c)
			rec := recordEvents(c)

			tc.hit(c)

			if c.Standing() || !c.SittingNow() {
				t.Fatalf("Standing() = %v, SittingNow() = %v after damage mid-transition, want still going down", c.Standing(), c.SittingNow())
			}
			if got := event.Count[event.StanceChanged](rec); got != 0 {
				t.Fatalf("stance broadcasts = %d, want none", got)
			}
		})
	}
}

// TestReduceHPByToggleUpkeepSparesSleepAndSitting mirrors
// PlayerStatus.reduceHp with isHPConsumption = skill.isToggle(): a toggle's
// own upkeep tick skips the whole wake-up block, so a seated, sleeping,
// immobile-until-attacked character stays that way while the HP comes off,
// invulnerability included, since it is the character's own tick.
func TestReduceHPByToggleUpkeepSparesSleepAndSitting(t *testing.T) {
	for _, invul := range []bool{false, true} {
		t.Run(fmt.Sprintf("invul=%v", invul), func(t *testing.T) {
			c := liveCharacter(1, combatTemplate(), combatItems())
			c.SetHP(100)
			attachTestLive(t, c)
			sitDownSettled(c)
			addCharacterEffect(t, c, "Sleep")
			addCharacterEffect(t, c, "ImmobileUntilAttacked")
			c.SetInvul(invul)
			rec := recordEvents(c)

			c.ReduceHPByToggleUpkeep(10, c)

			if got := c.HP(); got != 90 {
				t.Fatalf("HP() = %v after the upkeep tick, want 90", got)
			}
			if !c.Seated() {
				t.Fatal("Seated() = false after the upkeep tick, want still seated")
			}
			if got := event.Count[event.StanceChanged](rec); got != 0 {
				t.Fatalf("stance broadcasts = %d, want none", got)
			}
			if !c.Sleeping() {
				t.Fatal("Sleeping() = false after the upkeep tick, want the sleep effect kept")
			}
			if !c.EffectList().IsAffected(effect.FlagMeditating) {
				t.Fatal("IMMOBILE_UNTIL_ATTACKED stopped by the upkeep tick, want it kept")
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
// !isDOT gate for a real damage-over-time skill tick (isDOT=true, e.g.
// Poison/Bleed): it stops SLEEP/IMMOBILE_UNTIL_ATTACKED and stands the
// character up like any other hit, but never rolls to break STUN.
func TestReduceHPByDOTNeverBreaksStunEvenOnWinningRoll(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(100)
	attachTestLive(t, c)
	addCharacterEffect(t, c, "Stun")
	addCharacterEffect(t, c, "Sleep")
	c.SetRollSource(func(int) int { return 0 })

	c.ReduceHPByDOT(10, nil, true)

	if !c.Stunned() {
		t.Fatal("Stunned() = false after ReduceHPByDOT(isDOT=true), want STUN untouched on a real DOT tick")
	}
	if c.Sleeping() {
		t.Fatal("Sleeping() = true after ReduceHPByDOT, want the sleep effect stopped")
	}
}

// TestReduceHPByDOTBreaksStunWhenNotARealDOTTick covers drowning's exact
// reference call (WaterTaskManager.java: reduceCurrentHp(hp, player, false,
// false, null), isDOT=false): periodic non-attack damage that still allows
// the 1-in-10 STUN-break roll, unlike a real DOT skill tick.
func TestReduceHPByDOTBreaksStunWhenNotARealDOTTick(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(100)
	attachTestLive(t, c)
	addCharacterEffect(t, c, "Stun")
	c.SetRollSource(func(int) int { return 0 })

	c.ReduceHPByDOT(10, nil, false)

	if c.Stunned() {
		t.Fatal("Stunned() = true after ReduceHPByDOT(isDOT=false) with a winning roll, want STUN broken")
	}
}

// TestReduceHPSkipsDamageEffectsOnAlreadyDeadCharacter mirrors the
// reference's top-of-method isDead() early return: an already-dead
// character (dead state set, with HP clamped to 0) must not have its SLEEP effect
// stopped or get stood up by a stray hit landing after death.
func TestReduceHPSkipsDamageEffectsOnAlreadyDeadCharacter(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.MarkDead()
	attachTestLive(t, c)
	addCharacterEffect(t, c, "Sleep")
	sitDownSettled(c)

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
	c.MarkDead()
	attachTestLive(t, c)
	addCharacterEffect(t, c, "Sleep")
	sitDownSettled(c)

	c.ReduceHPByDOT(10, nil, true)

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
	sitDownSettled(c)

	c.TakeDamage(10, nil)

	if c.Sleeping() {
		t.Fatal("Sleeping() = true after TakeDamage, want the sleep effect stopped")
	}
	if !c.Standing() {
		t.Fatal("Standing() = false after TakeDamage, want the character stood up")
	}
}

// TestTakeDamageSkipsDamageEffectsOnZeroDamage covers a zero-damage hit
// (currently unreachable from attack.Controller.deliverHit, which filters
// hit.Damage <= 0 before calling TakeDamage, but TakeDamage is exported and
// exercised directly by other tests): it must not wake, unstun, or stand the
// character up, matching the existing convention ReduceHP/ReduceHPByDOT
// already use for a non-positive amount.
func TestTakeDamageSkipsDamageEffectsOnZeroDamage(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(100)
	attachTestLive(t, c)
	addCharacterEffect(t, c, "Sleep")
	sitDownSettled(c)

	c.TakeDamage(0, nil)

	if !c.Sleeping() {
		t.Fatal("Sleeping() = false after a zero-damage TakeDamage, want the sleep effect untouched")
	}
	if c.Standing() {
		t.Fatal("Standing() = true after a zero-damage TakeDamage, want it left seated")
	}
}
