package player

import "time"

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

// Flying reports whether this character is in a flying transform/mount state.
func (c *Character) Flying() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.flying
}

// SetFlying updates flying state and reports whether it changed.
func (c *Character) SetFlying(flying bool) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.initStateLocked()
	if c.flying == flying {
		return false
	}
	c.flying = flying
	return true
}

// Transformed reports whether this character is in a non-flying transform state.
func (c *Character) Transformed() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.transformed
}

// SetTransformed updates transform state and reports whether it changed.
func (c *Character) SetTransformed(transformed bool) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.initStateLocked()
	if c.transformed == transformed {
		return false
	}
	c.transformed = transformed
	return true
}

// Operating reports whether this character is operating a store/workshop.
func (c *Character) Operating() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.operating
}

// SetOperating updates store/workshop operation state and reports whether it changed.
func (c *Character) SetOperating(operating bool) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.initStateLocked()
	if c.operating == operating {
		return false
	}
	c.operating = operating
	return true
}

// Fishing reports whether this character is currently fishing.
func (c *Character) Fishing() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.fishing
}

// SetFishing updates fishing state and reports whether it changed.
func (c *Character) SetFishing(fishing bool) bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.initStateLocked()
	if c.fishing == fishing {
		return false
	}
	c.fishing = fishing
	return true
}

// DisableItem marks an inventory object id unusable until delay expires.
func (c *Character) DisableItem(objectID int32, delay time.Duration) {
	if objectID <= 0 {
		return
	}
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if delay <= 0 {
		delete(c.disabledItems, objectID)
		return
	}
	if c.disabledItems == nil {
		c.disabledItems = make(map[int32]time.Time)
	}
	c.disabledItems[objectID] = time.Now().Add(delay)
}

// ItemDisabled reports whether an inventory object id is still disabled.
func (c *Character) ItemDisabled(objectID int32) bool {
	if objectID <= 0 {
		return false
	}
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	until, ok := c.disabledItems[objectID]
	if !ok {
		return false
	}
	if time.Now().Before(until) {
		return true
	}
	delete(c.disabledItems, objectID)
	return false
}
