package player

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
// reduceDeathPenaltyBuffLevel().
func (c *Character) ReduceDeathPenaltyLevel() int {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.deathPenaltyLevel > 0 {
		c.deathPenaltyLevel--
	}
	return c.deathPenaltyLevel
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
