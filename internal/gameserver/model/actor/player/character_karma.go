package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

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
// just died) had zero karma and no PvP flag of its own, mirroring the
// reference's onKillUpdatePvPKarma "otherwise, killer is considered as a
// PKer" branch (Player.java:2814: `targetPlayer.getKarma() == 0 &&
// targetPlayer.getPvpFlag() == 0`): a player killing a karma-free,
// non-flagged player gains karma and a PK-kill count. An actively-flagged,
// karma-free victim instead takes the reference's PvP-point branch
// (checkIfPvP); see awardKillerPvPKill.
//
// The reference also routes a kill through other karma-free outcomes when
// either side is dueling, when the kill happens in a PvP/siege zone, when
// the killer wields a cursed weapon, or when the kill is a clan-war kill.
// None of those states are tracked on Character yet, so this only ever
// reproduces the innocent-victim and already-flagged-victim gates; the
// others stay dormant until their owning subsystems land.
func (c *Character) awardKillerPKKarma(killer creature.DeathActor) {
	pk := killerPlayer(killer)
	if pk == nil || pk == c || c.KarmaPoints != 0 || c.PvPFlagState() != task.PvPFlagNone {
		return
	}
	pk.PKKills++
	pk.KarmaPoints += calculatePKKillKarmaGain(pk.PKKills)
	pk.notifyKarmaChanged()
	pk.UpdateUserInfo()
	pk.BroadcastRelations()
}

// awardKillerPvPKill grants the killer a PvP-kill point when both killer
// and victim (c, who just died) are karma-free and c was actively
// PvP-flagged, mirroring the reference's onKillUpdatePvPKarma checkIfPvP
// branch (Player.java:2802-2812, Playable.checkIfPvP at Playable.java:
// 215-227): a karma-free duel-less PvP fight, not a PK. Only the killer's
// own UserInfo is resent (Player.java:2811, "Send UserInfo packet to
// attacker with its Karma and PK Counter"); no karma change and no
// relation broadcast, since neither karma nor PvP flag state changed.
//
// The reference gates the whole method behind cursed-weapon, duel, and
// PvP/siege-zone early returns, and this branch's own condition also
// allows an at-war clan kill or a karma>0 victim under
// Config.KARMA_AWARD_PK_KILL. None of those states are tracked on
// Character yet, matching awardKillerPKKarma's existing dormant-state
// deferral; this only reproduces the plain checkIfPvP case.
func (c *Character) awardKillerPvPKill(killer creature.DeathActor) {
	pk := killerPlayer(killer)
	if pk == nil || pk == c || pk.KarmaPoints != 0 || c.KarmaPoints != 0 || c.PvPFlagState() == task.PvPFlagNone {
		return
	}
	pk.PvPKills++
	pk.UpdateUserInfo()
}

func killerPlayer(killer creature.DeathActor) *Character {
	if summon, ok := killer.(interface{ ActingPlayer() creature.DeathActor }); ok {
		killer = summon.ActingPlayer()
	}
	pk, _ := killer.(*Character)
	return pk
}

// SetKarmaChangeNotifier records the packet-layer hook that tells this
// character's own client its new karma total, mirroring setKarma's
// SystemMessage(YOUR_KARMA_HAS_BEEN_CHANGED_TO_S1) + StatusUpdate(KARMA)
// pair (Player.java:1076-1080).
func (c *Character) SetKarmaChangeNotifier(notify func(karma int)) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.notifyKarmaChange = notify
}

func (c *Character) notifyKarmaChanged() {
	c.stateMu.RLock()
	notify := c.notifyKarmaChange
	c.stateMu.RUnlock()
	if notify != nil {
		notify(c.KarmaPoints)
	}
}
