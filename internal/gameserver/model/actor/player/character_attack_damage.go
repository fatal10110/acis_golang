package player

import (
	"math/rand/v2"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// SetRollSource overrides MakeAttackHit's random source for deterministic
// tests.
func (c *Character) SetRollSource(f func(int) int) {
	c.roll = f
}

// MakeAttackHit resolves one physical attack result.
func (c *Character) MakeAttackHit(target attackable.Combatant, split bool) attack.Hit {
	hit := attack.Hit{Target: target, TargetID: target.ObjectID()}
	other, ok := target.(physicalTarget)
	if !ok {
		hit.Miss = true
		return hit
	}

	tmpl := c.template()
	if tmpl == nil {
		hit.Miss = true
		return hit
	}
	formulaTarget, ok := target.(creature.FormulaActor)
	if !ok {
		hit.Miss = true
		return hit
	}

	accuracy := c.Accuracy()
	evasion := other.Evasion()

	_, _, sz := c.Position()
	_, _, tz := other.Position()
	behind, inFront := creature.AttackFacing(other, c)
	rate := formulas.HitRate(accuracy, evasion, sz-tz, creature.Night(), behind, inFront)
	if formulas.Missed(rate, c.rollValue(1000)) {
		hit.Miss = true
		return hit
	}

	crit := formulas.CritSucceeds(c.CriticalRate(), c.rollValue(1000))
	in, shield := creature.ResolvePhysicalAttackInput(c, formulaTarget, crit)
	hit.Damage = creature.ApplyPhysicalAttackDamage(in, shield, split)
	hit.Crit = crit
	hit.Shield = shield
	return hit
}

func (c *Character) rollValue(n int) int {
	if n <= 0 {
		return 0
	}
	if c.roll != nil {
		return c.roll(n)
	}
	return rand.IntN(n)
}

// Roll draws a uniform random integer in [0, n) from c's combat random source.
func (c *Character) Roll(n int) int {
	return c.rollValue(n)
}

// SetFloatRollSource overrides RollFloat's random source for deterministic tests.
func (c *Character) SetFloatRollSource(f func(float64) float64) {
	c.floatRoll = f
}

// RollFloat draws a uniform random value in [0, n) from c's combat random source.
func (c *Character) RollFloat(n float64) float64 {
	if n <= 0 {
		return 0
	}
	if c.floatRoll != nil {
		return c.floatRoll(n)
	}
	return rand.Float64() * n
}
