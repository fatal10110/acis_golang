package formulas

import "math"

// HealShotScaling selects how a charged spiritshot scales the caster's M.Atk
// term in a heal.
type HealShotScaling uint8

const (
	// HealShotScalingNone keeps the plain M.Atk term under a shot: a
	// fighter-class player.
	HealShotScalingNone HealShotScaling = iota
	// HealShotScalingMage doubles the M.Atk term under a spiritshot and
	// quadruples it under a blessed one: a mage-class player or a summon.
	HealShotScalingMage
	// HealShotScalingNPC quadruples the M.Atk term under either shot.
	HealShotScalingNPC
)

// HealInput is the resolved state of one HEAL or HEAL_STATIC cast.
type HealInput struct {
	Power       float64
	Proficiency float64
	// Static marks a HEAL_STATIC skill: power and proficiency only.
	Static  bool
	MAtk    int
	Scaling HealShotScaling

	Spiritshot        bool
	BlessedSpiritshot bool
	// SpsCorrection is the healSps correction for the skill at MAtk; it only
	// counts while a shot is charged.
	SpsCorrection float64
}

// HealAmount is the HP a heal restores before the target's heal
// effectiveness applies.
func HealAmount(in HealInput) float64 {
	amount := in.Power + in.Proficiency
	if in.Static {
		return amount
	}

	spsPower := 0.
	mAtkMul := 1.
	if in.Spiritshot || in.BlessedSpiritshot {
		spsPower = in.SpsCorrection
		if in.Spiritshot {
			spsPower *= 0.41
		}
		switch in.Scaling {
		case HealShotScalingMage:
			mAtkMul = 2
			if in.BlessedSpiritshot {
				mAtkMul = 4
			}
		case HealShotScalingNPC:
			mAtkMul = 4
		}
	}
	// The shot terms are summed before they join the base amount.
	return amount + (spsPower + math.Sqrt(mAtkMul*float64(in.MAtk)))
}
