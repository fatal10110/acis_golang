package summon

import "github.com/fatal10110/acis_golang/internal/gameserver/model/zone"

// ZoneQuery is the position-based peace query launch revalidation uses. A
// *zone.Index also gives the summon its zone membership.
type ZoneQuery interface {
	EffectRangeInPeaceZone(regionX, regionY, x, y, z, effectRange int) bool
}

// EffectRangeInPeaceZone reports whether an effect overlaps a peace zone in
// this summon's current region.
func (a *Actor) EffectRangeInPeaceZone(x, y, z, effectRange int) bool {
	if a.zones == nil {
		return false
	}
	rx, ry, _ := a.Position()
	return a.zones.EffectRangeInPeaceZone(rx, ry, x, y, z, effectRange)
}

// CollisionRadius returns this summon's template body radius.
func (a *Actor) CollisionRadius() float64 {
	return a.radius
}

// CollisionHeight returns this summon's template body height for line-of-sight
// eye-height calculations.
func (a *Actor) CollisionHeight() float64 {
	return a.height
}

// InPvPZone reports whether this summon's zones hold it in a PvP zone: an
// arena, an active siege battlefield or a stadium with a match running, and
// no peace zone. It is the summon's own membership, not its owner's.
func (a *Actor) InPvPZone() bool { return a.membership.has(zone.FlagPvP) }

// InSiegeZone reports whether this summon's zones hold it in an active siege
// battlefield.
func (a *Actor) InSiegeZone() bool { return a.membership.has(zone.FlagSiege) }
