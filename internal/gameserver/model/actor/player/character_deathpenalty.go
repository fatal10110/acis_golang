package player

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"

// maxDeathPenaltyLevel is the reference's hard cap on the death-penalty
// debuff level (skill 5076).
const maxDeathPenaltyLevel = 15

// deathPenaltyChance is the reference's default per-death percent chance to
// raise the death-penalty debuff level for a death that carries no karma.
const deathPenaltyChance = 20

// raidRelatedKiller is implemented by a killer that can identify as tied to
// a raid encounter, exempting a Charm-of-Luck death from the penalty even
// without an identified killer.
type raidRelatedKiller interface {
	RaidRelated() bool
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
// reduceDeathPenaltyBuffLevel().
func (c *Character) ReduceDeathPenaltyLevel() int {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.deathPenaltyLevel > 0 {
		c.deathPenaltyLevel--
	}
	return c.deathPenaltyLevel
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
// passes on the chance roll. The reference's additional PvP/siege-zone
// exemption isn't evaluated here: no zone-membership state exists on
// Character yet (tracked as a follow-up gap).
func (c *Character) RaiseDeathPenaltyLevel(killer any, roll int) (int, bool) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	if c.deathPenaltyLevel >= maxDeathPenaltyLevel {
		return c.deathPenaltyLevel, false
	}
	if _, byPlayer := killer.(*Character); byPlayer {
		return c.deathPenaltyLevel, false
	}
	if c.KarmaPoints <= 0 && roll > deathPenaltyChance {
		return c.deathPenaltyLevel, false
	}
	if c.EffectList().IsAffected(effect.FlagCharmOfLuck) {
		rr, _ := killer.(raidRelatedKiller)
		if killer == nil || (rr != nil && rr.RaidRelated()) {
			return c.deathPenaltyLevel, false
		}
	}
	if c.EffectList().IsAffected(effect.FlagPhoenixBlessing) {
		return c.deathPenaltyLevel, false
	}

	c.deathPenaltyLevel++
	return c.deathPenaltyLevel, true
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
