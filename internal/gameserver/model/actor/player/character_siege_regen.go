package player

import "github.com/fatal10110/acis_golang/internal/gameserver/model/location"

// headquartersRegenRange is how close, in 3D, an attacker must stand to its
// clan's siege headquarters to regenerate HP faster.
const headquartersRegenRange = 200

// hpRegenHeadquartersMultiplier is the siege headquarters HP recovery
// bonus: 1.5 while c's clan attacks the siege in progress where c stands
// and c stands within headquartersRegenRange of the clan's headquarters, 1
// otherwise.
func (c *Character) hpRegenHeadquartersMultiplier() float64 {
	clanID := c.ClanID()
	if clanID == 0 || c.sieges == nil || c.social == nil {
		return 1
	}
	x, y, z := c.Position()
	if !c.sieges.AttackerAt(clanID, x, y, z) {
		return 1
	}
	hx, hy, hz, ok := c.social.ClanHeadquarters(clanID)
	if !ok || !location.In3DRadius(x, y, z, hx, hy, hz, headquartersRegenRange) {
		return 1
	}
	return 1.5
}
