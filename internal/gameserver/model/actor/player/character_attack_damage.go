package player

import (
	"math/rand/v2"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
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
	weapon := c.activeWeapon()

	accuracy := c.Accuracy()
	evasion := other.Evasion()

	_, _, sz := c.Position()
	_, _, tz := other.Position()
	rate := formulas.HitRate(accuracy, evasion, sz-tz, false, false, true)
	if formulas.Missed(rate, c.rollValue(1000)) {
		hit.Miss = true
		return hit
	}

	critRate := c.CriticalRate()
	crit := formulas.CritSucceeds(critRate, c.rollValue(1000))

	randomMul := 1.0
	if weapon.tmpl != nil && weapon.tmpl.Weapon != nil {
		if spread := int(weapon.tmpl.Weapon.RandomDamage); spread > 0 {
			randomMul = 1 + float64(c.rollValue(2*spread+1)-spread)/100
		}
	}

	defence := other.PDef()
	if defence <= 0 {
		defence = 1
	}
	damage := formulas.PhysicalAttackDamage(formulas.PhysicalAttackInput{
		AttackPower:       c.pAtk(weapon),
		Defence:           defence,
		Crit:              crit,
		PosMul:            formulas.PosMul(false, true, crit),
		ElementalMul:      1,
		RandomMul:         randomMul,
		RaceMul:           1,
		WeaponVulnMul:     1,
		PvPMul:            1,
		CritDamageMul:     1,
		CritDamagePosMul:  1,
		CritVulnMul:       1,
		CritDamageAddBase: 0,
	})
	if split {
		damage /= 2
	}

	hit.Damage = int(damage)
	hit.Crit = crit
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
