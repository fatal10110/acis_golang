package summon

// ZoneQuery is the position-based zone membership a summon reads: the peace
// query launch revalidation uses, and the combat zones (PvP, siege) the
// zones at its position give it.
type ZoneQuery interface {
	EffectRangeInPeaceZone(regionX, regionY, x, y, z, effectRange int) bool
	SummonCombatZones(x, y, z int) (pvp, siege bool)
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

// InPvPZone reports whether this summon stands inside a PvP zone: an arena,
// an active siege battlefield or a stadium with a match running, and no
// peace zone. It is the summon's own membership, not its owner's.
func (a *Actor) InPvPZone() bool {
	pvp, _ := a.combatZones()
	return pvp
}

// InSiegeZone reports whether this summon stands inside an active siege
// battlefield.
func (a *Actor) InSiegeZone() bool {
	_, siege := a.combatZones()
	return siege
}

func (a *Actor) combatZones() (pvp, siege bool) {
	if a.zones == nil {
		return false, false
	}
	x, y, z := a.Position()
	return a.zones.SummonCombatZones(x, y, z)
}
