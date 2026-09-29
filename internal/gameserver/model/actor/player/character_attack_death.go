package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
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
// The status and damage report of the hit go out before the cast-break
// roll: an auto-attack rolls the break only once the hit has landed, and a
// killing hit's death has already ended the cast.
func (c *Character) TakeDamage(dmg int, attacker attackable.Combatant) bool {
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
	hit := c.absorbCPThenReduceHP(float64(dmg), attacker, false)
	c.vitalsMu.Unlock()
	if !hit.applied {
		return false
	}
	c.sendHitFeedback(float64(dmg), attacker, hit, false)
	if hit.dead {
		return c.Die(attacker)
	}
	c.breakCastOnDamage(float64(dmg))
	return false
}

// NotifyAttacked reports a damaging physical hit, or an offensive skill,
// from attacker reaching this character: it enters its attack stance.
func (c *Character) NotifyAttacked(attacker attackable.Combatant) {
	c.emit(event.Attacked{Attacker: attacker})
}

// NotifyEvaded reports a physical hit from attacker that missed this
// character, which is told whose attack it avoided.
func (c *Character) NotifyEvaded(attacker attackable.Combatant) {
	c.emit(event.Evaded{Attacker: attacker})
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

// MarkDead clears HP and transitions this player into its dead state.
func (c *Character) MarkDead() bool {
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	if c.dead.Load() {
		return false
	}
	c.curHP = 0
	c.dead.Store(true)
	return true
}

// Revive stands this dead player back up and reports whether it did; a call
// on a living player is a no-op. A Phoenix Blessing restores full HP and MP
// and is used up; otherwise HP comes back to the configured respawn
// fraction of max HP. Observers see the new HP and the revive, the player
// loses a Charm of Courage and gets its status flags refreshed, and any
// pending resurrection offer lapses. A rider's mount is full again and
// starts eating anew.
func (c *Character) Revive() bool {
	c.reviveMu.Lock()
	defer c.reviveMu.Unlock()
	return c.revive()
}

// ReviveRestoringExp is a resurrection's revive: it restores restorePercent
// of the exp the last death took, then revives the player as Revive does.
// It does nothing to a player that is not dead — a player who already went
// back to town keeps the loss — and reports whether it revived.
func (c *Character) ReviveRestoringExp(restorePercent float64) bool {
	c.reviveMu.Lock()
	defer c.reviveMu.Unlock()
	return c.reviveRestoringExp(restorePercent)
}

// reviveRestoringExp is ReviveRestoringExp; the caller holds reviveMu.
func (c *Character) reviveRestoringExp(restorePercent float64) bool {
	if !c.dead.Load() {
		return false
	}
	c.RestoreExp(restorePercent)
	return c.revive()
}

// revive is Revive's state transition and its follow-ups; the caller holds
// reviveMu. A player in the middle of a teleport is not revived. HP (and MP
// for a Phoenix Blessing) is set in the same step that clears the dead
// flag, so no hit lands on a living player at 0 HP.
func (c *Character) revive() bool {
	if live := c.liveLocked(); live != nil && live.Teleporting() {
		return false
	}
	res := c.ResourceValues()
	blessed := c.EffectList().IsAffected(effect.FlagPhoenixBlessing)
	c.vitalsMu.Lock()
	if !c.dead.CompareAndSwap(true, false) {
		c.vitalsMu.Unlock()
		return false
	}
	if blessed {
		c.curHP, c.curMP = res.MaxHP, res.MaxMP
	} else {
		c.curHP = min(res.MaxHP*c.respawnRestoreHP, res.MaxHP)
	}
	c.vitalsMu.Unlock()

	if blessed {
		c.stopPhoenixBlessing()
	}
	c.BroadcastStatus()
	c.emit(event.Revived{})
	c.EffectList().StopByType(effect.TypeCharmOfCourage)
	c.emit(event.EtcStatusChanged{})
	c.reviveRequested, c.revivePower = false, 0
	c.StartMountFeed()
	return true
}

// Die runs this player's death sequence: the once-only dead-state
// transition and zero-HP status, then the death packet broadcast to this
// player's own session and every observer, so the corpse-fall animation
// plays live and reaches clients before any death side effect's updates. A
// rider's mount stops eating. The killer's PK/PvP credit follows, then this
// player's own costs: charges, the experience/karma loss, the stop of every
// fusion channel on this player, and the death-penalty level, whose karma
// gate reads the karma left after that loss. A player whose Phoenix Blessing
// survived the death is then offered its own resurrection, and the effect
// icons are refreshed last.
//
// Stripping a Phoenix or Noblesse Blessing's companions (the other blessing
// and a Charm of Luck) refreshes the player's appearance for observers once
// per blessing stopped, on top of each removed effect's own refresh.
func (c *Character) Die(killer attackable.Combatant) bool {
	if !c.MarkDead() {
		return false
	}
	c.BroadcastStatus()
	c.StopCast()
	for range c.EffectList().StopOnDeath() {
		c.BroadcastAbnormalEffect()
	}
	c.BroadcastStatus()
	c.BroadcastDie()
	c.stopMountFeed()
	c.awardKillerPKKarma(killer)
	c.awardKillerPvPKill(killer)
	c.ClearCharges()
	c.applyDeathExpKarmaLoss(killer)
	c.emit(event.FusionCastersStopRequested{})
	c.RaiseDeathPenaltyLevel(killer, c.rollValue(100)+1)
	c.emit(event.DeathSettled{})
	if c.EffectList().IsAffected(effect.FlagPhoenixBlessing) {
		c.ReviveRequest(c, 0, false)
	}
	// The retained effects' icons are resent once the death has settled.
	c.UpdateEffectIcons()
	return true
}
