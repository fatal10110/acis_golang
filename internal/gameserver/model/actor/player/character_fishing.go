package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// FishingBait returns where this character's fishing line is cast, the
// zero location when none is. The status and appearance packets show it.
func (c *Character) FishingBait() location.Location {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.fishingBait
}

// SetFishingBait records where this character's fishing line is cast; the
// zero location takes it out of the water.
func (c *Character) SetFishingBait(bait location.Location) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.fishingBait = bait
}

// FishingRod returns the grade of the fishing rod this character holds;
// ok is false when its hand holds no fishing rod.
func (c *Character) FishingRod() (grade item.CrystalType, ok bool) {
	w := c.activeWeapon()
	if w.inst == nil || w.tmpl == nil || w.tmpl.Weapon == nil || w.tmpl.Weapon.Type != item.WeaponFishingRod {
		return item.CrystalNone, false
	}
	return w.tmpl.Crystal, true
}
