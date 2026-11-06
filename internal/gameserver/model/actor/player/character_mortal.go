package player

// Mortal reports whether a hit can kill c. Every character is mortal
// until a game master's //undying makes it otherwise; the setting is never
// stored, so a character logs in mortal again.
func (c *Character) Mortal() bool { return !c.immortal.Load() }

// SetMortal sets whether a hit can kill c. An immortal character is left
// on 1 HP by any hit that would take its last.
func (c *Character) SetMortal(v bool) { c.immortal.Store(!v) }
