package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

const baseWeightLimit = 69000

// SetWeightLimitMultiplier records the players.properties WeightLimit rate.
func (c *Character) SetWeightLimitMultiplier(multiplier float64) {
	c.stateMu.Lock()
	c.weightLimitMultiplier = multiplier
	c.stateMu.Unlock()
}

// WeightLimit returns the current stat-modified carrying limit.
func (c *Character) WeightLimit() int {
	c.stateMu.RLock()
	multiplier := c.weightLimitMultiplier
	c.stateMu.RUnlock()
	return int(c.CalcStat(stat.WeightLimit, baseWeightLimit*statbonus.CONBonus[c.CON()]*multiplier))
}

// CurrentWeight returns the attached inventory's current carried weight.
func (c *Character) CurrentWeight() int {
	if c == nil || c.Inventory() == nil {
		return 0
	}
	return c.Inventory().TotalWeight()
}

// RefreshWeightPenalty updates the overload band and notifies the runtime only
// when it changes.
func (c *Character) RefreshWeightPenalty() {
	if c == nil || c.Inventory() == nil {
		return
	}
	limit := c.WeightLimit()
	if limit <= 0 {
		return
	}
	weight := c.CurrentWeight() - int(c.CalcStat(stat.WeightPenalty, 0))
	penalty := 4
	switch ratio := float64(weight) / float64(limit); {
	case ratio < .5:
		penalty = 0
	case ratio < .666:
		penalty = 1
	case ratio < .8:
		penalty = 2
	case ratio < 1:
		penalty = 3
	}
	c.stateMu.Lock()
	changed := c.weightPenalty != penalty
	c.weightPenalty = penalty
	update := c.updateWeightPenalty
	c.stateMu.Unlock()
	if changed && c.Live != nil {
		c.Move().SetSpeed(c.RunSpeed())
	}
	if changed && update != nil {
		update()
	}
}

// WeightPenalty returns the current overload band ordinal.
func (c *Character) WeightPenalty() int {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.weightPenalty
}

// SetWeightPenaltyUpdater records the packet-layer notification for a band change.
func (c *Character) SetWeightPenaltyUpdater(update func()) {
	c.stateMu.Lock()
	c.updateWeightPenalty = update
	c.stateMu.Unlock()
}

// weightPenaltySpeedMultiplier mirrors WeightPenalty's per-band speed
// multiplier (WeightPenalty.java:5-9): NONE/LEVEL_1 1, LEVEL_2/LEVEL_3 0.5,
// LEVEL_4 0 — a fully overloaded player cannot move.
func (c *Character) weightPenaltySpeedMultiplier() float64 {
	switch c.WeightPenalty() {
	case 2, 3:
		return .5
	case 4:
		return 0
	default:
		return 1
	}
}

func (c *Character) weightPenaltyRegenMultiplier() float64 {
	switch c.WeightPenalty() {
	case 1, 2, 3:
		return .5
	case 4:
		return .1
	default:
		return 1
	}
}
