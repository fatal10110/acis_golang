package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// pkKillKarmaPlateau is the karma gain once a killer's PK-kill count
// reaches the formula's flat-rate ceiling (the karma formula's default
// branch).
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

// creditKill updates the standing of the player acting through killer
// after it killed c, or c's summon when summonKill is set. A killer
// wielding a cursed weapon never gets here for a player kill: Die has the
// kill feed the weapon instead. Nothing changes when the two players are
// both in a duel, or both in a PvP zone, where a player kill between
// opposite siege sides counts a siege kill and a siege death instead (see
// reportSiegeKill).
//
// A PvP kill (see pvpKill) awards the killer a PvP point and resends its
// UserInfo; a summon kill awards nothing. Any other kill of a karma-free,
// unflagged victim makes the killer a PKer: a player kill adds a PK kill,
// and the karma gain is keyed off the killer's PK count, at the summon
// rate for a summon kill (see publishPKKarma).
func (c *Character) creditKill(killer attackable.Combatant, summonKill bool) {
	pk := actingCharacter(killer)
	if pk == nil || pk == c || duelKillExempt(pk, c) {
		return
	}
	if pk.InPvPZone() && c.InPvPZone() {
		if !summonKill {
			c.reportSiegeKill(pk)
		}
		return
	}
	if pk.pvpKill(c) {
		if summonKill {
			return
		}
		pk.progressionMu.Lock()
		pk.PvPKills++
		pk.progressionMu.Unlock()
		pk.UpdateUserInfo()
		return
	}
	if c.Karma() != 0 || c.PvPFlagState() != task.PvPFlagNone {
		return
	}
	pk.progressionMu.Lock()
	if !summonKill {
		pk.PKKills++
	}
	pk.KarmaPoints += calculateKarmaGain(pk.PKKills, summonKill)
	karma := pk.KarmaPoints
	pk.progressionMu.Unlock()
	pk.publishPKKarma(karma)
}

// pvpKill reports whether c killing victim, or victim's summon, is a PvP
// kill: c holds no karma and victim is flagged without karma; the two
// belong to clans at war with each other, neither in its academy; or
// victim holds karma and c's AwardPKKillPVPPoint option is on.
func (c *Character) pvpKill(victim *Character) bool {
	victimKarma := victim.Karma()
	if c.Karma() == 0 && victimKarma == 0 && victim.PvPFlagState() != task.PvPFlagNone {
		return true
	}
	if c.atWarOutsideAcademy(victim) {
		return true
	}
	c.stateMu.RLock()
	awardPKKillPVPPoint := c.awardPKKillPVPPoint
	c.stateMu.RUnlock()
	return victimKarma > 0 && awardPKKillPVPPoint
}

// reportSiegeKill reports c's death to pk, both in a PvP zone, as a siege
// kill when the two fight a siege on opposite sides.
func (c *Character) reportSiegeKill(pk *Character) {
	own, theirs := c.SiegeState(), pk.SiegeState()
	if own == SiegeStateNone || theirs == SiegeStateNone || own == theirs {
		return
	}
	c.emit(event.SiegeKill{KillerClanID: pk.ClanID(), VictimClanID: c.ClanID()})
}

// AwardSummonKillKarma credits killer for killing c's summon; see
// creditKill. Killing one's own summon awards nothing.
func (c *Character) AwardSummonKillKarma(killer attackable.Combatant) {
	c.creditKill(killer, true)
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
