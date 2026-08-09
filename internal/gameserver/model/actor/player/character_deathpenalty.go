package player

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"

// maxDeathPenaltyLevel is the reference's hard cap on the death-penalty
// debuff level (skill 5076).
const maxDeathPenaltyLevel = 15

// raidRelatedKiller is implemented by a killer that can identify as tied to
// a raid encounter, exempting a Charm-of-Luck death from the penalty even
// without an identified killer.
type raidRelatedKiller interface {
	RaidRelated() bool
}

// SetDeathPenaltyChance records the players.properties chance used by the
// non-karma death-penalty gate.
func (c *Character) SetDeathPenaltyChance(chance int) {
	c.stateMu.Lock()
	c.deathPenaltyChance = chance
	c.stateMu.Unlock()
}

// DeathPenaltyLevel returns the current death-penalty debuff level.
func (c *Character) DeathPenaltyLevel() int {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.deathPenaltyLevel
}

// SetDeathPenaltyLevel sets the death-penalty debuff level, clamped to
// [0, maxDeathPenaltyLevel]. It is the persisted-load and admin hard-reset
// path; deciding whether and by how much a death raises the level (karma,
// PK chance, PvP/siege-zone exemption) is the death/PK-system's job, not
// this accessor's.
func (c *Character) SetDeathPenaltyLevel(level int) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.deathPenaltyLevel = clampDeathPenaltyLevel(level)
}

// ReduceDeathPenaltyLevel lowers the death-penalty debuff level by one, no
// lower than zero, and reports the resulting level. It is a no-op reporting
// the unchanged level (0) when already at zero, matching the reference's
// reduceDeathPenaltyBuffLevel() guard (Player.java:6537-6538). On an actual
// decrement it fires the reduced-updater with the new level, matching the
// reference's DEATH_PENALTY_LEVEL_S1_ADDED/DEATH_PENALTY_LIFTED + EtcStatusUpdate
// send (Player.java:6544-6553).
func (c *Character) ReduceDeathPenaltyLevel() int {
	c.stateMu.Lock()
	if c.deathPenaltyLevel <= 0 {
		c.stateMu.Unlock()
		return c.deathPenaltyLevel
	}
	c.deathPenaltyLevel--
	oldLevel := c.deathPenaltyLevel + 1
	level := c.deathPenaltyLevel
	skillUpdate := c.updateDeathPenaltySkill
	update := c.updateDeathPenaltyReduced
	c.stateMu.Unlock()

	if skillUpdate != nil {
		skillUpdate(oldLevel, level)
	}
	if update != nil {
		update(level)
	}
	return level
}

// SetDeathPenaltyRaisedUpdater records the packet-layer notification fired
// when a death raises the death-penalty debuff level, matching the
// reference's EtcStatusUpdate + DEATH_PENALTY_LEVEL_S1_ADDED send inside
// calculateDeathPenaltyBuffLevel (Player.java:6527-6528).
func (c *Character) SetDeathPenaltyRaisedUpdater(update func(level int)) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.updateDeathPenaltyRaised = update
}

// SetDeathPenaltyReducedUpdater records the packet-layer notification fired
// when a Recovery effect reduces the death-penalty debuff level, matching
// the reference's DEATH_PENALTY_LEVEL_S1_ADDED/DEATH_PENALTY_LIFTED +
// EtcStatusUpdate send in reduceDeathPenaltyBuffLevel (Player.java:6544-6553).
func (c *Character) SetDeathPenaltyReducedUpdater(update func(level int)) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.updateDeathPenaltyReduced = update
}

// SetDeathPenaltySkillUpdater records the runtime hook that replaces the
// death-penalty skill's transient passive stats after each level change.
func (c *Character) SetDeathPenaltySkillUpdater(update func(oldLevel, newLevel int)) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.updateDeathPenaltySkill = update
}

// RaiseDeathPenaltyLevel evaluates the death-penalty increment gate for a
// player death and, when it passes, raises the debuff level by one (capped
// at maxDeathPenaltyLevel). killer is the actor that caused the death, or
// nil for an environmental death; roll is a caller-supplied draw in [1,100]
// so callers can inject determinism in tests, matching the reference's
// Rnd.get(1,100) chance roll. It reports the resulting level and whether it
// changed.
//
// Gate, matching the reference's calculateDeathPenaltyBuffLevel: blocked by
// a Player killer, blocked by Charm of Luck unless the killer is unknown or
// raid-related, blocked by Phoenix Blessing, and — absent karma — only
// passes on the chance roll. It is also blocked in PvP and siege zones. On a
// passing gate it fires the raised-updater with the new level, matching the
// reference's
// EtcStatusUpdate + DEATH_PENALTY_LEVEL_S1_ADDED send (Player.java:6527-6528).
func (c *Character) RaiseDeathPenaltyLevel(killer any, roll int) (int, bool) {
	c.stateMu.Lock()

	if c.deathPenaltyLevel >= maxDeathPenaltyLevel {
		c.stateMu.Unlock()
		return c.deathPenaltyLevel, false
	}
	if _, byPlayer := killer.(*Character); byPlayer {
		c.stateMu.Unlock()
		return c.deathPenaltyLevel, false
	}
	if c.InPvPZone() || c.InSiegeZone() {
		c.stateMu.Unlock()
		return c.deathPenaltyLevel, false
	}
	if c.KarmaPoints <= 0 && roll > c.deathPenaltyChance {
		c.stateMu.Unlock()
		return c.deathPenaltyLevel, false
	}
	if c.EffectList().IsAffected(effect.FlagCharmOfLuck) {
		rr, _ := killer.(raidRelatedKiller)
		if killer == nil || (rr != nil && rr.RaidRelated()) {
			c.stateMu.Unlock()
			return c.deathPenaltyLevel, false
		}
	}
	if c.EffectList().IsAffected(effect.FlagPhoenixBlessing) {
		c.stateMu.Unlock()
		return c.deathPenaltyLevel, false
	}

	oldLevel := c.deathPenaltyLevel
	c.deathPenaltyLevel++
	level := c.deathPenaltyLevel
	skillUpdate := c.updateDeathPenaltySkill
	update := c.updateDeathPenaltyRaised
	c.stateMu.Unlock()

	if skillUpdate != nil {
		skillUpdate(oldLevel, level)
	}
	if update != nil {
		update(level)
	}
	return level, true
}

func clampDeathPenaltyLevel(level int) int {
	if level < 0 {
		return 0
	}
	if level > maxDeathPenaltyLevel {
		return maxDeathPenaltyLevel
	}
	return level
}
