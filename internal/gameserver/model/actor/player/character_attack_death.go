package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
)

// TakeDamage applies physical damage, broadcasts the resulting HP to nearby
// observers, and runs the once-only death path when HP reaches zero. A hit
// against an already-dead character is a no-op: no damage is applied and no
// status is broadcast.
func (c *Character) TakeDamage(dmg int, attacker creature.DeathActor) bool {
	if c.AlikeDead() {
		return false
	}
	newlyDead := c.ReduceCurrentHP(dmg)
	c.breakCastOnDamage(float64(dmg))
	c.BroadcastStatus()
	if !newlyDead {
		return false
	}
	return c.Die(attacker)
}

// Dead reports whether the player has died.
func (c *Character) Dead() bool {
	c.deathMu.Lock()
	defer c.deathMu.Unlock()
	return c.dead
}

// AlikeDead reports whether this player is dead or dead-equivalent,
// including a Fake Death toggle that is currently active.
func (c *Character) AlikeDead() bool {
	return c.Dead() || c.FakeDead()
}

// MarkDead transitions this player into its dead state.
func (c *Character) MarkDead() bool {
	c.deathMu.Lock()
	defer c.deathMu.Unlock()
	if c.dead {
		return false
	}
	c.dead = true
	return true
}

// Revive clears this player's dead state and restores HP to fraction of
// calculated max HP. It reports whether the player was dead and is now
// revived; a call on a living player is a no-op.
func (c *Character) Revive(fraction float64) bool {
	c.deathMu.Lock()
	if !c.dead {
		c.deathMu.Unlock()
		return false
	}
	c.dead = false
	c.deathMu.Unlock()

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
	c.ClearCharges()
	c.RaiseDeathPenaltyLevel(killer, c.rollValue(100)+1)
	c.BroadcastDie()
	return true
}
