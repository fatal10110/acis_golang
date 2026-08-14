package summon

type peaceZoneQuery interface {
	EffectRangeInPeaceZone(regionX, regionY, x, y, z, effectRange int) bool
}

// SetZones records the zone query launch revalidation uses.
func (a *Actor) SetZones(zones peaceZoneQuery) {
	a.zones = zones
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
