package formulas

import "math"

// clampDamage enforces the floor every damage formula in this file shares:
// never negative, and never below 1 once an attack actually connects.
func clampDamage(damage float64) float64 {
	if damage < 0 {
		return 0
	}
	if damage < 1 {
		return 1
	}
	return damage
}

// PhysicalAttackInput is a normal (non-skill) physical attack's
// already-resolved inputs. Defence must already include the target's
// shield bonus when the block succeeded; a caller whose shield roll came
// back ShieldPerfect should return a flat 1 damage without calling
// PhysicalAttackDamage at all.
//
// The Crit* fields are only read when Crit is true; a non-critical attack
// can leave them at their zero value.
type PhysicalAttackInput struct {
	AttackPower float64
	Defence     float64

	Crit     bool
	SoulShot bool

	PosMul        float64 // PosMul(behind, inFront, crit)
	ElementalMul  float64
	RandomMul     float64
	RaceMul       float64
	WeaponVulnMul float64
	PvPMul        float64

	CritDamageMul     float64
	CritDamagePosMul  float64
	CritVulnMul       float64
	CritDamageAddBase float64 // raw critical-damage-add stat, pre 77/Defence scaling
}

// PhysicalAttackDamage computes a normal physical attack's damage.
func PhysicalAttackDamage(in PhysicalAttackInput) float64 {
	addCritPower := 0.0
	critDamMul, critDamPosMul, critVuln := 1.0, 1.0, 1.0
	if in.Crit {
		critDamMul = in.CritDamageMul
		critDamPosMul = in.CritDamagePosMul
		critVuln = in.CritVulnMul
		// The additive critical-damage stat is intentionally scaled by
		// defence here, then scaled again with the full critical bracket
		// below; that double scaling is part of the damage contract.
		addCritPower = in.CritDamageAddBase * 77. / in.Defence
	}

	var damage float64
	if in.Crit {
		damage = (in.AttackPower*2.*critDamMul*critDamPosMul*critVuln*in.PosMul*in.RandomMul*in.RaceMul*in.PvPMul*in.ElementalMul*in.WeaponVulnMul + addCritPower) * 77. / in.Defence
	} else {
		damage = (in.AttackPower * in.PosMul * in.RandomMul * in.RaceMul * in.PvPMul * in.ElementalMul * in.WeaponVulnMul) * 77. / in.Defence
	}

	if in.SoulShot {
		// Soulshot doubles the finished physical-hit damage after all
		// bracket multipliers and defence scaling have already applied.
		damage *= 2.
	}

	return clampDamage(damage)
}

// PhysicalSkillInput is a physical skill's already-resolved inputs.
// Defence must already include the target's shield bonus, same as
// PhysicalAttackInput.
type PhysicalSkillInput struct {
	AttackPower float64
	SkillPower  float64 // includes any per-skill SoulShot power boost when SoulShot is true
	Defence     float64

	Crit     bool
	SoulShot bool

	// RandomMul is the attacker's random damage multiplier; leave it at 1
	// for a charge-damage skill, which does not use random variance.
	RandomMul float64

	ElementalMul  float64
	RaceMul       float64
	WeaponVulnMul float64
	PvPMul        float64
}

// PhysicalSkillDamage computes a physical skill's damage.
func PhysicalSkillDamage(in PhysicalSkillInput) float64 {
	ssMul := 1.0
	if in.SoulShot {
		ssMul = 2.04
	}

	damage := ((in.AttackPower*ssMul + in.SkillPower) * in.RandomMul * in.RaceMul * in.PvPMul * in.ElementalMul * in.WeaponVulnMul) * 77. / in.Defence
	if in.Crit {
		damage *= 2.
	}

	return clampDamage(damage)
}

// BlowInput is a blow-type skill's already-resolved inputs. A blow is
// always a critical hit, so — unlike PhysicalAttackInput — the crit-family
// multipliers here are always read.
type BlowInput struct {
	AttackPower float64
	SkillPower  float64 // includes any per-skill SoulShot power boost when SoulShot is true
	Defence     float64

	SoulShot bool
	IsPvP    bool

	// RandomMul is a fresh 95-105% roll (independent of the attacker's
	// general random damage multiplier); PosMul should be computed with
	// crit=true.
	RandomMul float64
	PosMul    float64

	PvPMul            float64 // only meaningful when IsPvP
	CritDamageMul     float64
	CritDamagePosMul  float64 // caller passes the blow-adjusted crit-position multiplier, not the raw stat
	CritVulnMul       float64
	DaggerVulnMul     float64
	CritDamageAddBase float64 // raw critical-damage-add stat, pre ×6 scaling
}

// BlowDamage computes a blow-type skill's damage. Unlike the other damage
// formulas, a successful PvP blow divides by 70 instead of 77, and the
// critical-damage-add contribution is scaled by a flat ×6 instead of by
// 77/Defence.
func BlowDamage(in BlowInput) float64 {
	attackPower := in.AttackPower
	if in.SoulShot {
		attackPower *= 2.
	}

	pvpMul := 1.0
	divisor := 77.
	if in.IsPvP {
		pvpMul = in.PvPMul
		divisor = 70.
	}

	addCritPower := in.CritDamageAddBase * 6

	damage := ((attackPower+in.SkillPower)*in.CritDamageMul*in.RandomMul*in.CritDamagePosMul*in.PosMul*pvpMul*in.CritVulnMul*in.DaggerVulnMul + addCritPower) * divisor / in.Defence

	return math.Max(1, damage)
}

// MagicDamageInput is a magic skill's already-resolved inputs. MDef must
// already include the target's shield bonus, same as PhysicalAttackInput's
// Defence. Resist-driven damage reduction (a target fully or partially
// resisting the spell) is the cast pipeline's job once skill data exists —
// this computes only the pre-resist damage.
type MagicDamageInput struct {
	MAtk       float64
	MDef       float64
	SkillPower float64

	SoulShot        bool
	BlessedSoulShot bool
	MagicCrit       bool // callers must pass false for resisted casts

	PvPMul       float64
	ElementalMul float64
}

// MagicDamage computes a magic skill's pre-resist damage.
func MagicDamage(in MagicDamageInput) float64 {
	mAtk := in.MAtk
	if in.BlessedSoulShot {
		mAtk *= 4
	} else if in.SoulShot {
		mAtk *= 2
	}

	damage := 91 * math.Sqrt(mAtk) / in.MDef * in.SkillPower
	if in.MagicCrit {
		damage *= 4
	}

	damage *= in.PvPMul
	damage *= in.ElementalMul

	return damage
}

// ManaDamageInput is a mana-drain skill's already-resolved inputs.
type ManaDamageInput struct {
	MAtk        float64
	MDef        float64
	SkillPower  float64
	TargetMaxMp float64

	SoulShot        bool
	BlessedSoulShot bool

	// VulnMul is the target's skill-type vulnerability multiplier (e.g.
	// DRAIN); pass 1 when not applicable.
	VulnMul float64
}

// ManaDamage computes a mana-drain skill's damage.
func ManaDamage(in ManaDamageInput) float64 {
	mAtk := in.MAtk
	if in.BlessedSoulShot {
		mAtk *= 4
	} else if in.SoulShot {
		mAtk *= 2
	}

	damage := (math.Sqrt(mAtk) * in.SkillPower * (in.TargetMaxMp / 97)) / in.MDef
	damage *= in.VulnMul

	return damage
}
