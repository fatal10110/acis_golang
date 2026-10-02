package formulas

import "math"

// CubicMAtkModifier is the magic-attack term of a cubic's landing roll for
// a magic skill: the cubic's own fixed M.Atk, quadrupled under its owner's
// blessed spiritshot, against the target's M.Def.
func CubicMAtkModifier(mAtk, mDef float64, bss bool) float64 {
	val := mAtk
	if bss {
		val = mAtk * 4
	}
	return math.Sqrt(val) / mDef * 11
}

// CubicMagicDamageInput is a cubic's magic strike's already-resolved
// inputs. MDef already includes the target's shield bonus. Failure is the
// owner's magic-success outcome; Shield is the block outcome against the
// owner, where a perfect block deals 1.
type CubicMagicDamageInput struct {
	MDef         float64
	SkillPower   float64
	MagicCrit    bool
	Failure      MagicFailure
	ElementalMul float64
	Shield       ShieldDefense
}

// CubicMagicDamage computes a cubic's magic strike damage: 91 / M.Def times
// the skill power, with no attacker M.Atk, shot, or PvP term. A half resist
// halves it and a full resist sets it to 1; only an unresisted strike takes
// the x4 magic critical. The elemental modifier applies last.
func CubicMagicDamage(in CubicMagicDamageInput) float64 {
	if in.Shield == ShieldPerfect {
		return 1
	}
	damage := 91 / in.MDef * in.SkillPower
	switch in.Failure {
	case MagicFailureHalf:
		damage /= 2
	case MagicFailureFull:
		damage = 1
	case MagicFailureNone:
		if in.MagicCrit {
			damage *= 4
		}
	}
	return damage * in.ElementalMul
}
