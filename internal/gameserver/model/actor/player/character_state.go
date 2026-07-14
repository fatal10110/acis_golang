package player

func (c *Character) initStateLocked() {
	if c.stateInit {
		return
	}
	c.running = true
	c.standing = true
	c.stateInit = true
}

// Running reports whether this character is in run mode.
func (c *Character) Running() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return !c.stateInit || c.running
}

// SetRunning updates run mode and reports whether it changed.
func (c *Character) SetRunning(running bool) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.initStateLocked()
	if c.running == running {
		return false
	}
	c.running = running
	return true
}

// Standing reports whether this character is standing rather than sitting.
func (c *Character) Standing() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return !c.stateInit || c.standing
}

// SetStanding updates sit/stand mode and reports whether it changed.
func (c *Character) SetStanding(standing bool) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.initStateLocked()
	if c.standing == standing {
		return false
	}
	c.standing = standing
	return true
}

// InCombat reports whether this character has started an attack stance.
func (c *Character) InCombat() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.inCombat
}

// SetInCombat updates attack stance and reports whether it changed.
func (c *Character) SetInCombat(inCombat bool) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.initStateLocked()
	if c.inCombat == inCombat {
		return false
	}
	c.inCombat = inCombat
	return true
}
