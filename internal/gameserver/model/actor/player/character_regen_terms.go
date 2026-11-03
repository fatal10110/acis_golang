package player

// postureRegenMultiplier is what c's posture and movement multiply its
// regeneration by: 1.5 seated, 1.1 standing still, 0.7 running, and 1
// walking. A character still sitting down counts as standing.
func (c *Character) postureRegenMultiplier() float64 {
	switch {
	case c.Seated():
		return 1.5
	case !c.IsMoving():
		return 1.1
	case c.Running():
		return 0.7
	default:
		return 1
	}
}

// SetInMotherTreeZone records the live zone engine's current Mother Tree
// zone membership (zone.FlagMotherTree): inside a zone whose race
// restriction admits c.
func (c *Character) SetInMotherTreeZone(inside bool) {
	c.insideMotherTreeZone.Store(inside)
}

// motherTreeRegenBonus is the flat HP and MP regeneration bonus of the
// first Mother Tree zone holding c's position, while c stands in one that
// admits it; 0 otherwise.
func (c *Character) motherTreeRegenBonus() (hp, mp float64) {
	if !c.insideMotherTreeZone.Load() || c.zones == nil {
		return 0, 0
	}
	x, y, z := c.Position()
	h, m, ok := c.zones.MotherTreeBonus(x, y, z)
	if !ok {
		return 0, 0
	}
	return float64(h), float64(m)
}
