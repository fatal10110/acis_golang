package player

// VarkaKetraAlliance returns the character's standing with the Ketra Orc
// and Varka Silenos factions: 1 to 5 is the level of an alliance with
// Ketra, -1 to -5 the level of one with Varka, and 0 is neutral. It is kept
// across sessions.
func (c *Character) VarkaKetraAlliance() int { return int(c.varkaKetraAlliance.Load()) }

// SetVarkaKetraAlliance sets the faction standing VarkaKetraAlliance
// reports. The value is stored as given; the faction quests only ever set
// a level in [-5, 5].
func (c *Character) SetVarkaKetraAlliance(level int) { c.varkaKetraAlliance.Store(int32(level)) }

// AlliedWithVarka reports whether the character is allied with Varka
// Silenos, at any level.
func (c *Character) AlliedWithVarka() bool { return c.varkaKetraAlliance.Load() < 0 }

// AlliedWithKetra reports whether the character is allied with the Ketra
// Orcs, at any level.
func (c *Character) AlliedWithKetra() bool { return c.varkaKetraAlliance.Load() > 0 }
