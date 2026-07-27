package player

// SeedPower returns the charge power of the active elemental-seed effect
// (one of the Fire/Water/Wind seed skill ids) named by effectID, or 0 if
// that seed isn't charged at all. Matches the reference's
// getFirstEffect(seedSkillId).getPower() lookup.
func (c *Character) SeedPower(effectID int) int {
	level, _ := c.EffectList().ActiveBySkillID(effectID)
	return level
}

// ForceLevel returns the level of the active Force effect (Battle or Spell
// Force skill id) named by skillID, and whether one is currently active at
// all. Matches the reference's getFirstEffect(forceSkillId)._effect lookup.
func (c *Character) ForceLevel(skillID int) (int, bool) {
	return c.EffectList().ActiveBySkillID(skillID)
}
