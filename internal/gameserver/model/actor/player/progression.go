package player

import "math"

// maxSP is the largest SP value a character can hold, matching the 32-bit
// signed integer ceiling the persisted column was sized for.
const maxSP = math.MaxInt32

// AddExpAndSp adds exp and sp to c independently — either amount is
// ignored if negative — resyncing c.Level from the resulting experience
// via table and, on a level increase, refilling HP, MP and CP to the full
// amount tmpl's per-level tables define for the new level. tmpl may be
// nil, in which case a level increase still updates c.Level and c.Exp but
// leaves HP/MP/CP untouched. It reports whether the level increased.
func (c *Character) AddExpAndSp(table *LevelTable, tmpl *Template, exp int64, sp int) bool {
	leveledUp := false
	if exp >= 0 {
		leveledUp = c.AddExp(table, tmpl, exp)
	}
	if sp >= 0 {
		c.AddSp(sp)
	}
	return leveledUp
}

// RewardExpAndSp applies a kill reward using this live character's runtime
// template for level-up stat refills.
func (c *Character) RewardExpAndSp(table *LevelTable, exp int64, sp int) bool {
	if table == nil {
		if sp >= 0 {
			c.AddSp(sp)
		}
		return false
	}
	return c.AddExpAndSp(table, c.runtimeTemplate, exp, sp)
}

// AddExp adds delta experience to c. An addition that would overflow
// c.Exp negative is silently dropped, and an addition that would reach the
// top of the highest level's experience band is clamped just below it. It
// resyncs c.Level from the new experience via table, applying the same
// HP/MP/CP refill as AddLevel on an increase. It reports whether the level
// increased.
func (c *Character) AddExp(table *LevelTable, tmpl *Template, delta int64) bool {
	if c.Exp+delta < 0 {
		return false
	}

	capExp := table.RequiredExpForHighestLevel()
	if c.Exp+delta >= capExp {
		delta = capExp - 1 - c.Exp
	}
	c.Exp += delta

	level := table.levelForExp(c.Exp)
	if level == c.Level {
		return false
	}
	return c.AddLevel(table, tmpl, level-c.Level)
}

// AddSp adds delta sp to c.SP, saturating at the 32-bit signed integer
// maximum the persisted column was sized for. A negative delta is a no-op.
func (c *Character) AddSp(delta int) {
	if delta < 0 || c.SP >= maxSP {
		return
	}
	if delta > maxSP-c.SP {
		delta = maxSP - c.SP
	}
	c.SP += delta
}

// RemoveExpAndSp removes exp and sp from c independently — either amount
// is ignored unless positive — resyncing c.Level the same way AddExpAndSp
// does. A level drop never refills HP/MP/CP, matching AddLevel.
func (c *Character) RemoveExpAndSp(table *LevelTable, tmpl *Template, exp int64, sp int) {
	if exp > 0 {
		c.RemoveExp(table, tmpl, exp)
	}
	if sp > 0 {
		c.RemoveSp(sp)
	}
}

// RemoveExp subtracts delta experience from c, flooring at 1 experience
// (never 0) rather than going negative, and resyncs c.Level from the
// result via table.
func (c *Character) RemoveExp(table *LevelTable, tmpl *Template, delta int64) {
	if c.Exp-delta < 0 {
		delta = c.Exp - 1
	}
	c.Exp -= delta

	if level := table.levelForExp(c.Exp); level != c.Level {
		c.AddLevel(table, tmpl, level-c.Level)
	}
}

// RemoveSp subtracts delta sp from c.SP, flooring at 0.
func (c *Character) RemoveSp(delta int) {
	c.SP = max(0, c.SP-delta)
}

// AddLevel changes c.Level by delta levels (positive to level up, negative
// to level down), refusing entirely — leaving c untouched — if that would
// put the level above table's real max. It resyncs c.Exp to stay inside
// the resulting level's experience band, and only when the level actually
// increases, refills HP, MP and CP to the full amount tmpl's per-level
// tables define for the new level (skipped if tmpl is nil or has no row
// for it). It reports whether the level increased.
func (c *Character) AddLevel(table *LevelTable, tmpl *Template, delta int) bool {
	if c.Level+delta > table.RealMaxLevel() {
		return false
	}

	increased := delta > 0
	c.Level += delta

	lower := table.RequiredExpForLevel(c.Level)
	upper := table.RequiredExpForLevel(c.Level + 1)
	if c.Exp >= upper || lower > c.Exp {
		c.Exp = lower
	}

	if !increased {
		return false
	}

	if idx := c.Level - 1; tmpl != nil && idx >= 0 && idx < len(tmpl.HPTable) && idx < len(tmpl.MPTable) && idx < len(tmpl.CPTable) {
		c.MaxHP, c.CurHP = tmpl.HPTable[idx], tmpl.HPTable[idx]
		c.MaxMP, c.CurMP = tmpl.MPTable[idx], tmpl.MPTable[idx]
		c.MaxCP, c.CurCP = tmpl.CPTable[idx], tmpl.CPTable[idx]
	}
	return true
}
