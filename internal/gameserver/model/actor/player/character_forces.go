package player

// SeedPower returns the charge power of the first elemental-seed effect
// (one of the Fire/Water/Wind seed skill ids) named by effectID, preferring
// an in-use effect over a held one, or 0 if that seed isn't charged at all.
func (c *Character) SeedPower(effectID int) int {
	level, _ := c.EffectList().ActiveBySkillID(effectID)
	return level
}

// ForceLevel returns the level of the first Force effect (Battle or Spell
// Force skill id) named by skillID, preferring an in-use effect over a held
// one, and whether any such effect exists.
func (c *Character) ForceLevel(skillID int) (int, bool) {
	return c.EffectList().ActiveBySkillID(skillID)
}
