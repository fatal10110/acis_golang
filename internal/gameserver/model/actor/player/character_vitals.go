package player

// Vitals snapshots the character's current visible resources.
type Vitals struct {
	HP int
	MP int
}

// VitalsChange describes which player resources changed and their new values.
type VitalsChange struct {
	HP        int
	HPChanged bool
	MP        int
	MPChanged bool
}

// Vitals returns the character's current HP and MP as integer resource values.
func (c *Character) Vitals() Vitals {
	return Vitals{HP: c.CurrentHP(), MP: c.CurrentMP()}
}

// CurrentMP returns current MP as an integer resource value.
func (c *Character) CurrentMP() int {
	return int(c.CurMP)
}

// ChangesTo reports which resources differ in next.
func (v Vitals) ChangesTo(next Vitals) VitalsChange {
	return VitalsChange{
		HP:        next.HP,
		HPChanged: v.HP != next.HP,
		MP:        next.MP,
		MPChanged: v.MP != next.MP,
	}
}

// Changed reports whether any resource changed.
func (c VitalsChange) Changed() bool {
	return c.HPChanged || c.MPChanged
}
