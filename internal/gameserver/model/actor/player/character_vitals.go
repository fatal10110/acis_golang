package player

// Vitals snapshots the character's current visible resources.
type Vitals struct {
	HP int
	MP int
}

// Resources snapshots the character's maximum and current visible resources.
type Resources struct {
	MaxHP, CurrentHP float64
	MaxMP, CurrentMP float64
	MaxCP, CurrentCP float64
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

// ResourceValues returns a synchronized HP/MP/CP resource snapshot.
func (c *Character) ResourceValues() Resources {
	c.vitalsMu.RLock()
	defer c.vitalsMu.RUnlock()
	return Resources{
		MaxHP: c.MaxHP, CurrentHP: c.CurHP,
		MaxMP: c.MaxMP, CurrentMP: c.CurMP,
		MaxCP: c.MaxCP, CurrentCP: c.CurCP,
	}
}

// CurrentHP returns current HP as an integer resource value.
func (c *Character) CurrentHP() int {
	return int(c.ResourceValues().CurrentHP)
}

// CurrentMP returns current MP as an integer resource value.
func (c *Character) CurrentMP() int {
	return int(c.ResourceValues().CurrentMP)
}

// CurrentCP returns current CP as an integer resource value.
func (c *Character) CurrentCP() int {
	return int(c.ResourceValues().CurrentCP)
}

// ReduceCurrentHP subtracts non-negative HP, clamps at zero, and reports
// whether this call newly reached zero.
func (c *Character) ReduceCurrentHP(amount int) bool {
	if amount < 0 {
		amount = 0
	}
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	if c.CurHP <= 0 {
		return false
	}
	c.CurHP -= float64(amount)
	if c.CurHP > 0 {
		return false
	}
	c.CurHP = 0
	return true
}

// ReduceCurrentMP subtracts MP and clamps at zero.
func (c *Character) ReduceCurrentMP(amount int) {
	if amount <= 0 {
		return
	}
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	c.CurMP -= float64(amount)
	if c.CurMP < 0 {
		c.CurMP = 0
	}
}

func (c *Character) refillResources(maxHP, maxMP, maxCP float64) {
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	c.MaxHP, c.CurHP = maxHP, maxHP
	c.MaxMP, c.CurMP = maxMP, maxMP
	c.MaxCP, c.CurCP = maxCP, maxCP
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
