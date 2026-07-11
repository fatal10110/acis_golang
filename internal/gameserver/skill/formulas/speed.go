package formulas

// TimeBetweenAttacks returns the delay, in milliseconds, before an
// attacker's next physical attack, given pAtkSpd, its already-computed
// P.Atk.Spd stat. The delay never drops below 100ms regardless of how
// high attack speed climbs. pAtkSpd must be positive.
func TimeBetweenAttacks(pAtkSpd int) int {
	d := 500000 / pAtkSpd
	if d < 100 {
		return 100
	}
	return d
}

// AtkSpd returns the delay, in milliseconds, for a skill cast of
// skillTime (its configured base cast time), given the attacker's
// already-computed attack-speed stat: mAtkSpd when magic is true (the
// skill is a magic skill), pAtkSpd otherwise. The selected attack-speed
// stat must be positive.
func AtkSpd(magic bool, mAtkSpd, pAtkSpd int, skillTime float64) int {
	if magic {
		return int(skillTime * 333 / float64(mAtkSpd))
	}
	return int(skillTime * 333 / float64(pAtkSpd))
}
