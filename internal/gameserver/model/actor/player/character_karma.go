package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// pkKillKarmaPlateau is the karma gain once a killer's PK-kill count
// reaches the formula's flat-rate ceiling, matching
// Formulas.calculateKarmaGain's default branch.
const pkKillKarmaPlateau = 14400

// calculateKarmaGain returns the karma a PK kill awards the killer, keyed
// off pkCount. Gain ramps linearly below 100 kills, ramps more slowly from
// 100 up to 180, then plateaus at pkKillKarmaPlateau. Killing a summon
// awards about a quarter of that, with pkCount's two low bits added before
// the division.
func calculateKarmaGain(pkCount int, summon bool) int {
	result := pkKillKarmaPlateau
	switch {
	case pkCount < 100:
		result = int((((float64(pkCount-1) * 0.5) + 1) * 60) * 4)
	case pkCount < 180:
		result = int((((float64(pkCount+1) * 0.125) + 37.5) * 60) * 4)
	}
	if summon {
		result = ((pkCount & 3) + result) >> 2
	}
	return result
}

// awardKillerPKKarma grants killer PK-kill karma when c (the victim who
// just died) had zero karma and no PvP flag of its own: a player killing a
// karma-free, non-flagged player is considered a PKer and gains karma and
// a PK-kill count. An actively-flagged, karma-free victim instead takes
// the PvP-point branch; see awardKillerPvPKill.
//
// A kill between two players who are both in a duel changes nothing. A
// kill also takes other karma-free outcomes when it happens in a PvP/siege
// zone, when the killer wields a cursed weapon, or when the kill is a
// clan-war kill. None of those states are tracked on Character yet, so
// those gates stay dormant until their owning subsystems land.
func (c *Character) awardKillerPKKarma(killer attackable.Combatant) {
	pk := actingCharacter(killer)
	if pk == nil || pk == c || duelKillExempt(pk, c) || c.Karma() != 0 || c.PvPFlagState() != task.PvPFlagNone {
		return
	}
	pk.progressionMu.Lock()
	pk.PKKills++
	pk.KarmaPoints += calculateKarmaGain(pk.PKKills, false)
	karma := pk.KarmaPoints
	pk.progressionMu.Unlock()
	pk.publishPKKarma(karma)
}

// AwardSummonKillKarma grants killer PK karma for killing c's summon when c
// has no karma and no PvP flag, and the kill is not inside a PvP zone for
// both players. A summon kill never counts as a PK kill: the gain uses the
// killer's current PK count at the summon rate, and a PvP point is never
// awarded for it. Killing one's own summon awards nothing.
//
// Nothing is awarded when both players are in a duel. The clan-war
// exemption is not applied: that state is not tracked yet, the same as for
// a player kill (#1301).
func (c *Character) AwardSummonKillKarma(killer attackable.Combatant) {
	pk := actingCharacter(killer)
	if pk == nil || pk == c || duelKillExempt(pk, c) || c.Karma() != 0 || c.PvPFlagState() != task.PvPFlagNone {
		return
	}
	if pk.InPvPZone() && c.InPvPZone() {
		return
	}
	pk.progressionMu.Lock()
	pk.KarmaPoints += calculateKarmaGain(pk.PKKills, true)
	karma := pk.KarmaPoints
	pk.progressionMu.Unlock()
	pk.publishPKKarma(karma)
}

// publishPKKarma reports c's karma after a PK gain to its own client and its
// observers, then has c's equipped items rechecked against their conditions
// and its PvP flag ended, in that order.
func (c *Character) publishPKKarma(karma int) {
	c.notifyKarmaChanged(karma)
	c.UpdateUserInfo()
	c.BroadcastRelations()
	c.emit(event.PKKarmaGained{})
}

// awardKillerPvPKill grants the killer a PvP-kill point for an actively
// PvP-flagged, karma-free victim, or a karma-positive victim when the
// configured AwardPKKillPVPPoint option is enabled: the PvP-kill condition
// of a kill's PvP/karma update. Only the killer's own UserInfo is resent;
// no karma or PvP flag changes.
//
// Nothing is awarded when both players are in a duel. The whole update is
// also gated behind cursed-weapon and PvP/siege-zone early returns, and
// this branch's own condition also allows a mutual clan-war kill between
// non-academy members; the clan registry holds that state, not wired here
// yet (#1301).
func (c *Character) awardKillerPvPKill(killer attackable.Combatant) {
	pk := actingCharacter(killer)
	if pk == nil || pk == c || duelKillExempt(pk, c) {
		return
	}
	pk.stateMu.RLock()
	awardPKKillPVPPoint := pk.awardPKKillPVPPoint
	pk.stateMu.RUnlock()
	victimKarma := c.Karma()
	if victimKarma < 0 || (victimKarma > 0 && !awardPKKillPVPPoint) {
		return
	}
	victimUnflagged := c.PvPFlagState() == task.PvPFlagNone
	pk.progressionMu.Lock()
	if victimKarma == 0 && (pk.KarmaPoints != 0 || victimUnflagged) {
		pk.progressionMu.Unlock()
		return
	}
	pk.PvPKills++
	pk.progressionMu.Unlock()
	pk.UpdateUserInfo()
}

// actingCharacter resolves the player acting through c: c itself, or a
// summon's owner. A live wrapper embedding *Character resolves to its
// model, which is how a summon's owner is held.
func actingCharacter(c attackable.Combatant) *Character {
	if c != nil && c.Kind() == actor.KindSummon {
		c, _ = c.Owner()
	}
	holder, ok := c.(CharacterHolder)
	if !ok {
		return nil
	}
	return holder.PlayerCharacter()
}

func (c *Character) notifyKarmaChanged(karma int) {
	c.emit(event.KarmaChanged{Karma: karma})
}
