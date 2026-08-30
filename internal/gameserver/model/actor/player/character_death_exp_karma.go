package player

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// SetLevelTable records the level table consulted for the exp-loss- and
// karma-loss-at-death calculations.
func (c *Character) SetLevelTable(table *LevelTable) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.levelTable = table
}

// SetAllowDelevel records the players.properties AllowDelevel gate: whether
// a death may cost experience/karma at all, matching applyDeathPenalty's
// caller-side guard (Player.java:2650: `Config.ALLOW_DELEVEL &&
// (!hasSkill(SKILL_LUCKY) || getStatus().getLevel() > 9)`).
func (c *Character) SetAllowDelevel(allow bool) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.allowDelevel = allow
}

// SetRateKarmaExpLost records the server.properties RateKarmaExpLost
// multiplier applied to the death exp-loss percentage while karma is
// positive (Player.java:2904, Config.java:968).
func (c *Character) SetRateKarmaExpLost(rate float64) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.rateKarmaExpLost = rate
}

// applyDeathExpKarmaLoss computes and applies the experience and karma cost
// of this character's death, mirroring Player.applyDeathPenalty and
// updateKarmaLoss (Player.java:2649-2651, 2874-2926, 2749-2757). It is a
// no-op for an environmental death (killer == nil, Player.java:2615's
// `if (killer != null)` guard) and while the delevel gate is closed: the
// AllowDelevel config off, or the Lucky skill still active below level 10
// (Player.java:2650).
//
// Deferred pending owning subsystems, tracked so this stays linked rather
// than silently approximated (issue #1737):
//   - the PvP/siege-zone early return and its Charm of Courage stop, and the
//     killed-by-playable arena exemption (Player.java:2881-2895) — the
//     charm's effect flag isn't exported outside the effect package, and the
//     arena/olympiad state those branches matter for isn't tracked on
//     Character yet (#1302, #217, #215);
//   - the mutual-clan-war halving of percentLost (Player.java:2906,
//     `atWar`) — clan-war state isn't tracked yet (#149);
//   - the victim's-own cursed-weapon exemption in updateKarmaLoss
//     (Player.java:2751) — cursed-weapon-equipped state isn't tracked on
//     Character yet (#225).
//
// The siege-zone halving (Player.java:2906) and the festival-participant
// halving are wired: InSiegeZone and FestivalParticipant are live
// accessors (FestivalParticipant is a permanent false stub pending #223, so
// its branch stays dormant, not approximated).
func (c *Character) applyDeathExpKarmaLoss(killer creature.DeathActor) {
	if killer == nil {
		return
	}

	c.stateMu.RLock()
	table := c.levelTable
	allow := c.allowDelevel
	rate := c.rateKarmaExpLost
	c.stateMu.RUnlock()
	if table == nil || !allow {
		return
	}
	if c.HasSkill(int(skill.LuckySkillID)) && c.CharLevel <= 9 {
		return
	}

	level, ok := table.Level(c.CharLevel)
	if !ok {
		return
	}

	percentLost := level.ExpLossAtDeath
	if c.KarmaPoints > 0 {
		percentLost *= rate
	}
	if c.FestivalParticipant() || c.InSiegeZone() {
		percentLost /= 4.0
	}

	span := table.ExpSpanAtLevel(c.CharLevel)
	lostExp := int64(math.Round(float64(span) * percentLost / 100))

	// Snapshot the pre-loss exp for a later resurrection to restore from
	// (Player.java:2919, `setExpBeforeDeath(getStatus().getExp())`).
	c.progressionMu.Lock()
	c.ExpBeforeDeath = c.Exp
	c.progressionMu.Unlock()

	c.updateKarmaLoss(lostExp)
	c.RemoveExpAndSp(table, c.runtimeTemplate, lostExp, 0)
}

// RestoreExp restores restorePercent (0-100) of the experience lost in this
// character's last death, matching Player.restoreExp (Player.java:2865-2872).
// It is a no-op unless a death has left ExpBeforeDeath set, and always
// clears ExpBeforeDeath afterward.
func (c *Character) RestoreExp(restorePercent float64) {
	c.progressionMu.Lock()
	defer c.progressionMu.Unlock()
	if c.ExpBeforeDeath <= 0 {
		return
	}
	c.stateMu.RLock()
	table := c.levelTable
	c.stateMu.RUnlock()
	if table == nil {
		return
	}

	restored := int64(math.Round(float64(c.ExpBeforeDeath-c.Exp) * restorePercent / 100))
	c.ExpBeforeDeath = 0
	c.addExpAndSp(table, c.runtimeTemplate, restored, -1)
}

// updateKarmaLoss reduces this character's karma for a death that cost
// lostExp experience, matching Player.updateKarmaLoss (Player.java:2749-2757)
// and Formulas.calculateKarmaLost (Formulas.java:1267-1270). The victim's-own
// cursed-weapon exemption is deferred (see applyDeathExpKarmaLoss); until
// #225 lands this always evaluates as not-equipped, which is the common
// case Java itself reproduces for every non-cursed-weapon death.
func (c *Character) updateKarmaLoss(lostExp int64) {
	if c.KarmaPoints <= 0 {
		return
	}
	c.stateMu.RLock()
	table := c.levelTable
	c.stateMu.RUnlock()
	if table == nil {
		return
	}
	level, ok := table.Level(c.CharLevel)
	if !ok || level.KarmaModifier == 0 {
		return
	}

	karmaLost := int(float64(lostExp) / level.KarmaModifier / 15)
	if karmaLost <= 0 {
		return
	}

	newKarma := max(0, c.KarmaPoints-karmaLost)
	if newKarma == c.KarmaPoints {
		return
	}
	c.KarmaPoints = newKarma
	c.notifyKarmaChanged()
	c.UpdateUserInfo()
}
