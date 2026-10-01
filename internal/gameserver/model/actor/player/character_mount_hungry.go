package player

// MountHungry reports whether the character's mount is fed below its
// hungry limit, which refuses a /mount dismount.
func (c *Character) MountHungry() bool {
	f := &c.mountFeed
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hungryLocked()
}
