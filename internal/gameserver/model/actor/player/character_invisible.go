package player

// Invisible reports whether the character is hidden from other players: a
// game master's hide mode. Other clients are told to draw it invisible, its
// summon is not shown to them, and only game masters know it (Knows): no
// one else may target, attack or trade with it, and NPCs neither aggro on
// nor keep hating it.
func (c *Character) Invisible() bool { return c.invisible.Load() }

// SetInvisible turns the character's hide mode on or off. Callers own the
// client refresh that follows.
func (c *Character) SetInvisible(v bool) { c.invisible.Store(v) }

// SeesInvisible reports whether the character knows invisible players: its
// access level makes it a game master.
func (c *Character) SeesInvisible() bool { return c.seesInvisible.Load() }

// SetSeesInvisible records whether the character's access level makes it
// a game master.
func (c *Character) SetSeesInvisible(v bool) { c.seesInvisible.Store(v) }
