package item

// ShotKind identifies one of the charge slots a weapon or summon can carry:
// which flavor of shot is currently "loaded" and ready to apply its bonus
// on the next hit.
type ShotKind uint8

const (
	ShotSoul ShotKind = iota
	ShotSpirit
	ShotBlessedSpirit
	ShotFishSoul
)

// Mask returns the shotsMask bit ShotKind occupies.
func (k ShotKind) Mask() int32 {
	return 1 << uint(k)
}

// ChargedShot reports whether kind is currently charged on inst.
func (inst *Instance) ChargedShot(kind ShotKind) bool {
	return inst.ShotsMask&kind.Mask() == kind.Mask()
}

// SetChargedShot charges or discharges kind on inst.
func (inst *Instance) SetChargedShot(kind ShotKind, charged bool) {
	if charged {
		inst.ShotsMask |= kind.Mask()
	} else {
		inst.ShotsMask &^= kind.Mask()
	}
}

// UnchargeAllShots clears every charged shot on inst, e.g. when it is
// unequipped.
func (inst *Instance) UnchargeAllShots() {
	inst.ShotsMask = 0
}

// EvaluateSoulshot decides whether a soulshot of shotCrystal grade can be
// consumed against a weapon of this detail: the weapon must have soulshot
// capacity, match the shot's crystal grade, and not already be charged. It
// reports the shot count to consume (the weapon's reduced-consumption
// count when reducedRoll, a caller-supplied 0-99 percentile roll, lands
// under the weapon's reduced-soulshot chance) and whether the charge is
// allowed at all. The three distinct rejection reasons (no capacity, grade
// mismatch, already charged) collapse to a single false here: which one
// applies only changes which client message is shown, and that's network
// behavior this package doesn't own.
func (d *WeaponDetail) EvaluateSoulshot(weaponCrystal, shotCrystal CrystalType, alreadyCharged bool, reducedRoll int) (consume int32, ok bool) {
	if d.SoulshotCount == 0 || weaponCrystal != shotCrystal || alreadyCharged {
		return 0, false
	}
	consume = d.SoulshotCount
	if d.ReducedSoulshotCount > 0 && reducedRoll < int(d.ReducedSoulshotChance) {
		consume = d.ReducedSoulshotCount
	}
	return consume, true
}

// EvaluateSpiritshot decides whether a spiritshot (or blessed spiritshot;
// both draw from the same weapon capacity) of shotCrystal grade can be
// consumed against a weapon of this detail, using the same shape as
// EvaluateSoulshot minus the reduced-consumption roll, which spiritshots
// don't have.
func (d *WeaponDetail) EvaluateSpiritshot(weaponCrystal, shotCrystal CrystalType, alreadyCharged bool) (consume int32, ok bool) {
	if d.SpiritshotCount == 0 || weaponCrystal != shotCrystal || alreadyCharged {
		return 0, false
	}
	return d.SpiritshotCount, true
}
