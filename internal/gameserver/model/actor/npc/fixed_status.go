package npc

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// FixedStatus is the status of an NPC that holds no effects and takes no
// damage, a decoration or an effect point: every stat finalized from its
// template and passives, at full HP and MP.
type FixedStatus struct{ fs fixedStats }

// Status returns the decoration's status.
func (d *Decoration) Status() FixedStatus { return FixedStatus{d.stats} }

// Status returns the effect point's status.
func (ep *EffectPoint) Status() FixedStatus { return FixedStatus{ep.stats} }

// CalcStat finalizes base for s.
func (s FixedStatus) CalcStat(st stat.Stat, base float64) float64 { return s.fs.calc(st, base) }

// HP returns the current HP, always the maximum.
func (s FixedStatus) HP() float64 { return s.MaxHPValue() }

// MaxHPValue returns the maximum HP in whole points.
func (s FixedStatus) MaxHPValue() float64 { return float64(s.fs.maxHP) }

// MPValue returns the current MP, always the maximum.
func (s FixedStatus) MPValue() float64 { return s.MaxMPValue() }

// MaxMPValue returns the maximum MP in whole points.
func (s FixedStatus) MaxMPValue() float64 {
	return math.Trunc(s.fs.calc(stat.MaxMP, s.fs.template.MPMax))
}

// PAtk returns the physical attack, truncated to a whole number.
func (s FixedStatus) PAtk() float64 {
	return math.Trunc(s.fs.calc(stat.PowerAttack, s.fs.template.PAtk))
}

// MAtk returns the magic attack, truncated to a whole number.
func (s FixedStatus) MAtk() float64 {
	return math.Trunc(s.fs.calc(stat.MagicAttack, positiveStat(s.fs.template.MAtk)))
}

// PDef returns the physical defence, truncated to a whole number.
func (s FixedStatus) PDef() float64 {
	return math.Trunc(s.fs.calc(stat.PowerDefence, s.fs.template.PDef))
}

// MDef returns the magic defence, truncated to a whole number.
func (s FixedStatus) MDef() float64 {
	return math.Trunc(s.fs.calc(stat.MagicDefence, positiveStat(s.fs.template.MDef)))
}

// Evasion returns the physical evasion.
func (s FixedStatus) Evasion() int { return int(s.fs.calc(stat.EvasionRate, 0)) }

// MoveSpeed returns the speed the stance it was placed in moves at.
func (s FixedStatus) MoveSpeed() float64 { return s.fs.moveSpeed }

// AttackSpeed returns the physical attack speed.
func (s FixedStatus) AttackSpeed() int { return s.fs.pAtkSpd }

// MagicAttackSpeed returns the casting speed.
func (s FixedStatus) MagicAttackSpeed() int { return s.fs.mAtkSpd }

// STR returns the template STR.
func (s FixedStatus) STR() int { return s.fs.template.STR }

// DEX returns the template DEX.
func (s FixedStatus) DEX() int { return s.fs.template.DEX }

// CON returns the template CON.
func (s FixedStatus) CON() int { return s.fs.template.CON }

// INT returns the template INT.
func (s FixedStatus) INT() int { return s.fs.template.INT }

// WIT returns the template WIT.
func (s FixedStatus) WIT() int { return s.fs.template.WIT }

// MEN returns the template MEN.
func (s FixedStatus) MEN() int { return s.fs.template.MEN }
