package player

// In7sDungeon reports whether the character went down into a Seven Signs
// dungeon (a necropolis or a catacomb) through its gatekeeper and has not
// left it through a way out that clears the membership: the dungeon exit, a
// restart point, a recall, the jail, //sendhome, or an expulsion as the
// Seven Signs period changes or at login. It is kept across sessions.
func (c *Character) In7sDungeon() bool { return c.in7sDungeon.Load() }

// SetIn7sDungeon sets the Seven Signs dungeon membership. It moves nothing
// by itself.
func (c *Character) SetIn7sDungeon(in bool) { c.in7sDungeon.Store(in) }
