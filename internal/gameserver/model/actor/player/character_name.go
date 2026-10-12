package player

// CharacterName is the character's name.
func (c *Character) CharacterName() string {
	if p := c.name.Load(); p != nil {
		return *p
	}
	return ""
}

// SetName renames the character. Callers that index players by name
// rename through the world's index so both change at once; see
// world.State.RenamePlayer.
func (c *Character) SetName(name string) {
	c.name.Store(&name)
}
