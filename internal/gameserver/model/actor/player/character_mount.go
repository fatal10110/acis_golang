package player

const wyvernNPCID int32 = 12621

// Mount records the active mount.
func (c *Character) Mount(npcID, controlItemID int32) bool {
	if npcID <= 0 || controlItemID <= 0 {
		return false
	}
	c.stateMu.Lock()
	c.initStateLocked()
	if c.mountNPCID == npcID && c.mountObjectID == controlItemID {
		c.stateMu.Unlock()
		return false
	}
	c.mountNPCID = npcID
	c.mountObjectID = controlItemID
	c.mountType = 0
	c.flying = false
	if npcID == wyvernNPCID {
		c.mountType = 2
		c.flying = true
	}
	c.stateMu.Unlock()
	return true
}

func (c *Character) MountType() int32 {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.mountType
}

func (c *Character) MountNPCID() int32 {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.mountNPCID
}

func (c *Character) MountObjectID() int32 {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.mountObjectID
}

// Mounted reports whether this character currently rides a mount, matching
// Player.isMounted() (checkSummoner's gate, SummonFriend.java:107).
func (c *Character) Mounted() bool {
	return c.MountNPCID() != 0
}
