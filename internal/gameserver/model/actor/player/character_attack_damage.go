package player

import (
	"math/rand/v2"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
)

var _ attack.Target = (*Character)(nil)

// SetRollSource overrides MakeAttackHit's random source for deterministic
// tests.
func (c *Character) SetRollSource(f func(int) int) {
	c.roll = f
}

// MakeAttackHit resolves one physical attack result.
func (c *Character) MakeAttackHit(target attackable.Combatant, split bool) attack.Hit {
	if _, ok := target.(creature.FormulaActor); !ok || c.template() == nil {
		return attack.Hit{TargetID: target.ObjectID(), Miss: true}
	}
	return attack.ResolveHit(c, target, attack.HitRoll{
		Accuracy: c.Accuracy(),
		Night:    c.EffectList().IsNight(),
		CritRate: c.criticalRate,
	}, split)
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
