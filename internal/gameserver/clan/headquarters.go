package clan

// Headquarters is the siege headquarters flag a clan put up in the world.
type Headquarters interface {
	ObjectID() int32
	Position() (x, y, z int)
	// DeleteNow takes the flag out of the world at once.
	DeleteNow()
}

// Headquarters returns the siege headquarters the clan holds.
func (c *Clan) Headquarters() (Headquarters, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.headquarters, c.headquarters != nil
}

// SetHeadquarters makes h the clan's siege headquarters. A clan holds one
// at most: it reports false, keeping the one it holds, when it already has
// one.
func (c *Clan) SetHeadquarters(h Headquarters) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.headquarters != nil {
		return false
	}
	c.headquarters = h
	return true
}

// DropHeadquarters takes the clan's siege headquarters, if any, out of the
// world, and the clan holds none after it.
func (c *Clan) DropHeadquarters() {
	c.mu.Lock()
	h := c.headquarters
	c.headquarters = nil
	c.mu.Unlock()
	if h != nil {
		h.DeleteNow()
	}
}

// DropHeadquartersOf is DropHeadquarters when the clan's headquarters is
// the flag objectID; any other headquarters is kept. It reports whether
// the flag was dropped.
func (c *Clan) DropHeadquartersOf(objectID int32) bool {
	c.mu.Lock()
	h := c.headquarters
	if h == nil || h.ObjectID() != objectID {
		c.mu.Unlock()
		return false
	}
	c.headquarters = nil
	c.mu.Unlock()
	h.DeleteNow()
	return true
}
