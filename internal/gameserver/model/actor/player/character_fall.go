package player

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

const fallingValidationDelay = 10 * time.Second

// CheckFall applies a reported ground drop and says whether position
// validation should be skipped while an earlier fall is still in progress.
func (c *Character) CheckFall(reportedZ int, ground, enabled bool, now time.Time) (damage int, falling bool) {
	if c.Dead() || !ground {
		return 0, false
	}
	if now.Before(c.fallingUntil) {
		return 0, true
	}

	// The body is the base class's whichever class is active.
	base := c.BaseTemplate()
	safeHeight := base.SafeFallHeightFemale
	if c.Sex() == SexMale {
		safeHeight = base.SafeFallHeightMale
	}
	deltaZ := int(int32(c.CurrentLocation().Z) - int32(reportedZ))
	if deltaZ <= safeHeight {
		return 0, false
	}

	if enabled {
		// Multiply as 32-bit integers before floating-point division.
		base := float64(int32(deltaZ)*int32(c.MaxHPValue())) / 1000
		damage = int(c.CalcStat(stat.Fall, base))
		if damage > 0 {
			applied := min(float64(damage), c.HP()-1)
			if applied > 0 {
				c.ReduceHPByDOT(applied, nil, true)
			} else if !c.Invul() {
				c.applyNonConsumptionDamageEffects(true)
			}
		}
	}
	c.fallingUntil = now.Add(fallingValidationDelay)
	return damage, false
}
