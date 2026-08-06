package player

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"

// pkKillKarmaPlateau is the karma gain once a killer's PK-kill count
// reaches the formula's flat-rate ceiling, matching
// Formulas.calculateKarmaGain's default branch.
const pkKillKarmaPlateau = 14400

// calculatePKKillKarmaGain returns the karma a PK kill awards the killer,
// keyed off the killer's PK-kill count after this kill has incremented it
// (so the first PK uses pkKills == 1). Gain ramps linearly below 100
// kills, ramps more slowly from 100 up to 180, then plateaus at
// pkKillKarmaPlateau — matching Formulas.calculateKarmaGain(pkCount,
// isSummon=false); the isSummon halving branch doesn't apply here since
// this path only handles a player killing another player.
func calculatePKKillKarmaGain(pkKills int) int {
	switch {
	case pkKills < 100:
		return int((((float64(pkKills-1) * 0.5) + 1) * 60) * 4)
	case pkKills < 180:
		return int((((float64(pkKills+1) * 0.125) + 37.5) * 60) * 4)
	default:
		return pkKillKarmaPlateau
	}
}

// awardKillerPKKarma grants killer PK-kill karma when c (the victim who
// just died) had zero karma of its own, mirroring the reference's
// onKillUpdatePvPKarma innocent-victim branch: a player killing a
// karma-free, non-flagged player gains karma and a PK-kill count.
//
// The reference also routes a kill through a different, karma-free
// outcome when the victim carries karma or PvP flag, when either side is
// dueling, when the kill happens in a PvP/siege zone, when the killer
// wields a cursed weapon, or when the kill is a clan-war kill. None of
// those states are tracked on Character yet — c.KarmaPoints == 0 is the
// only gate this port can evaluate — so this only ever reproduces the
// innocent-victim branch; the others stay dormant until their owning
// subsystems land.
func (c *Character) awardKillerPKKarma(killer creature.DeathActor) {
	pk, ok := killer.(*Character)
	if !ok || pk == c || c.KarmaPoints != 0 {
		return
	}
	pk.PKKills++
	pk.KarmaPoints += calculatePKKillKarmaGain(pk.PKKills)
	pk.UpdateUserInfo()
}
