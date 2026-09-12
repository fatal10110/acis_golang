package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// TakeDamage applies physical damage, broadcasts the resulting HP to nearby
// observers, and runs the once-only death path when HP reaches zero. A hit
// against an already-dead or invulnerable (spawn protection, GM invul,
// mid-teleport) character is a no-op before any change (PlayerStatus.java:
// 103-116). Otherwise the sleep/immobile-stop, stand-up, and stun-break side
// effects always run for a live hit (PlayerStatus.java:118-134) before the
// damage-permission check (:136-140): an attacker without damage permission
// still wakes/interrupts the target and can still break its cast — only the
// HP/CP change itself is dropped. A Playable attacker other than the actor
// itself drains CP before HP (CreatureAttack.java:263 -> PlayerStatus.reduceHp,
// PlayerStatus.java:166-184); melee never sets ignoreCP (Player.java:6154).
func (c *Character) TakeDamage(dmg int, attacker creature.DeathActor) bool {
	if c.AlikeDead() || c.Invul() {
		return false
	}
	if dmg > 0 {
		c.applyNonConsumptionDamageEffects(false)
	}
	if !creature.CanDealDamage(attacker) {
		c.breakCastOnDamage(float64(dmg))
		return false
	}
	c.vitalsMu.Lock()
	newlyDead := c.absorbCPThenReduceHP(float64(dmg), attacker, false)
	c.vitalsMu.Unlock()
	c.breakCastOnDamage(float64(dmg))
	c.BroadcastStatus()
	if !newlyDead {
		return false
	}
	return c.Die(attacker)
}

// Dead reports whether the player has died.
func (c *Character) Dead() bool {
	return c.dead.Load()
}

// AlikeDead reports whether this player is dead or dead-equivalent,
// including a Fake Death toggle that is currently active.
func (c *Character) AlikeDead() bool {
	return c.Dead() || c.FakeDead()
}

// MarkDead transitions this player into its dead state.
func (c *Character) MarkDead() bool {
	return c.dead.CompareAndSwap(false, true)
}

// Revive clears this player's dead state and restores HP to fraction of
// calculated max HP. It reports whether the player was dead and is now
// revived; a call on a living player is a no-op.
func (c *Character) Revive(fraction float64) bool {
	if !c.dead.CompareAndSwap(true, false) {
		return false
	}

	maxHP := c.ResourceValues().MaxHP
	c.vitalsMu.Lock()
	c.curHP = maxHP * fraction
	c.vitalsMu.Unlock()
	return true
}

// Die runs this player's death sequence: the once-only dead-state
// transition, then the death packet broadcast to this player's own session
// and every observer, so the corpse-fall animation plays live instead of
// only on a later dead reconnect.
func (c *Character) Die(killer creature.DeathActor) bool {
	if !creature.Die(c, killer, nil) {
		return false
	}
	c.StopCast()
	c.clearEffectsOnDeath()
	c.ClearCharges()
	c.RaiseDeathPenaltyLevel(killer, c.rollValue(100)+1)
	c.awardKillerPKKarma(killer)
	c.awardKillerPvPKill(killer)
	c.applyDeathExpKarmaLoss(killer)
	c.BroadcastDie()
	return true
}

func (c *Character) clearEffectsOnDeath() {
	list := c.EffectList()
	if list == nil {
		return
	}
	has := func(want effect.Type) bool {
		for _, e := range list.All() {
			if e.Type == want {
				return true
			}
		}
		return false
	}
	if has(effect.TypePhoenixBless) {
		list.StopByType(effect.TypeCharmOfLuck)
		list.StopByType(effect.TypeNoblesseBless)
		return
	}
	if has(effect.TypeNoblesseBless) {
		list.StopByType(effect.TypeNoblesseBless)
		list.StopByType(effect.TypeCharmOfLuck)
		return
	}
	list.StopAllExceptThoseThatLastThroughDeath()
}
