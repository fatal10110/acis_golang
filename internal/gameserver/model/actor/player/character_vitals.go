package player

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"

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
	res := Resources{
		MaxHP: c.maxHP, CurrentHP: c.curHP,
		MaxMP: c.maxMP, CurrentMP: c.curMP,
		MaxCP: c.maxCP, CurrentCP: c.curCP,
	}
	c.vitalsMu.RUnlock()
	if c.template() == nil {
		return res
	}
	res.MaxHP = c.calcStat(stat.MaxHP, res.MaxHP)
	res.MaxMP = c.calcStat(stat.MaxMP, res.MaxMP)
	res.MaxCP = c.calcStat(stat.MaxCP, res.MaxCP)
	return res
}

// SetResourceValues replaces c's persisted HP/MP/CP resource values.
func (c *Character) SetResourceValues(res Resources) {
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	c.maxHP, c.curHP = res.MaxHP, res.CurrentHP
	c.maxMP, c.curMP = res.MaxMP, res.CurrentMP
	c.maxCP, c.curCP = res.MaxCP, res.CurrentCP
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
	if c.curHP <= 0 {
		return false
	}
	c.curHP -= float64(amount)
	if c.curHP > 0 {
		return false
	}
	c.curHP = 0
	return true
}

// ReduceCurrentMP subtracts MP and clamps at zero.
func (c *Character) ReduceCurrentMP(amount int) {
	if amount <= 0 {
		return
	}
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	c.curMP -= float64(amount)
	if c.curMP < 0 {
		c.curMP = 0
	}
}

func (c *Character) refillResources(maxHP, maxMP, maxCP float64) {
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	c.maxHP, c.curHP = maxHP, maxHP
	c.maxMP, c.curMP = maxMP, maxMP
	c.maxCP, c.curCP = maxCP, maxCP
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
