package player

// WantsPeace reports whether c has personally surrendered a war of its
// clan. The flag outlives the war it was raised for: it is stored with the
// character and only cleared as c leaves its clan.
func (c *Character) WantsPeace() bool { return c.wantsPeace.Load() }

// SetWantsPeace sets c's personal-surrender flag, as restored from its row
// or cleared on leaving its clan.
func (c *Character) SetWantsPeace(wants bool) { c.wantsPeace.Store(wants) }

// RequestPeace raises c's personal-surrender flag, reporting false when it
// was already raised.
func (c *Character) RequestPeace() bool { return c.wantsPeace.CompareAndSwap(false, true) }
