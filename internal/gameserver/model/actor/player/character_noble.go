package player

// IsNoble reports whether the character holds noblesse status.
func (c *Character) IsNoble() bool { return c.noble.Load() }

// SetNoble sets the noblesse status. It grants or takes nothing by itself:
// the noble skills follow the status where the character's skills are
// built.
func (c *Character) SetNoble(noble bool) { c.noble.Store(noble) }
