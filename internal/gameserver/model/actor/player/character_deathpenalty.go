package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// maxDeathPenaltyLevel is the reference's hard cap on the death-penalty
// debuff level (skill 5076).
const maxDeathPenaltyLevel = 15

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
// decrement it emits DeathPenaltyChanged with the new level, matching the
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
	c.stateMu.Unlock()

	c.emit(event.DeathPenaltyChanged{Old: oldLevel, New: level})
	return level
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
// passing gate it emits DeathPenaltyChanged with the new level, matching the
// reference's
// EtcStatusUpdate + DEATH_PENALTY_LEVEL_S1_ADDED send (Player.java:6527-6528).
func (c *Character) RaiseDeathPenaltyLevel(killer attackable.Combatant, roll int) (int, bool) {
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
		if killer == nil || killer.RaidRelated() {
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
	c.stateMu.Unlock()

	c.emit(event.DeathPenaltyChanged{Old: oldLevel, New: level, Raised: true})
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
