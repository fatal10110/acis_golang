package player

import modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"

const wyvernNPCID int32 = 12621

// Mount records the active mount and the feeding data of the mount at the
// character's level. StartMountFeed starts feeding it. A wyvern flies and
// gives its rider Wyvern Breath for as long as it is ridden; the skill is
// not stored.
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
	flying := c.flying
	c.stateMu.Unlock()
	if flying {
		c.SetSkillLevel(int(modelskill.WyvernBreathSkillID), 1)
	}
	c.loadMountFeed(npcID)
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

// MountEats reports whether the ridden mount eats food item templateID.
func (c *Character) MountEats(templateID int32) bool {
	if templateID == 0 || !c.Mounted() {
		return false
	}
	f := &c.mountFeed
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.data.Food1 == templateID || f.data.Food2 == templateID
}

// Mounted reports whether this character currently rides a mount, matching
// Player.isMounted() (checkSummoner's gate, SummonFriend.java:107).
func (c *Character) Mounted() bool {
	return c.MountNPCID() != 0
}

// mountBody returns the active mount's collision footprint, or false when
// not mounted or when no mount lookup is attached (e.g. in tests).
func (c *Character) mountBody() (radius, height float64, ok bool) {
	c.stateMu.RLock()
	npcID, mounts := c.mountNPCID, c.mounts
	c.stateMu.RUnlock()
	if npcID == 0 || mounts == nil {
		return 0, 0, false
	}
	return mounts.CollisionBody(npcID)
}
