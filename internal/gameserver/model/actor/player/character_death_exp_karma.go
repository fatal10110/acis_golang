package player

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// deathLossExemption reports whether a death inside a PvP zone costs no
// experience or karma, and whether that exemption uses up a Charm of
// Courage. In a siege zone only the charm exempts the death, whoever the
// killer; in any other PvP zone a death to a player or its summon is exempt.
func deathLossExemption(inPvP, inSiege, charmOfCourage, killedByPlayable bool) (exempt, useCharm bool) {
	switch {
	case !inPvP:
		return false, false
	case inSiege:
		return charmOfCourage, charmOfCourage
	default:
		return killedByPlayable, false
	}
}

// applyDeathExpKarmaLoss computes and applies the experience and karma cost
// of this character's death, mirroring Player.applyDeathPenalty and
// updateKarmaLoss (Player.java:2649-2651, 2874-2926, 2749-2757). It is a
// no-op for an environmental death (killer == nil, Player.java:2615's
// `if (killer != null)` guard); any other death first clears the previous
// death's exp snapshot, so a later resurrection restores nothing unless this
// death takes exp. The loss is skipped while the delevel gate is closed: the
// AllowDelevel config off, or the Lucky skill still active below level 10
// (Player.java:2650).
//
// Inside a PvP zone two deaths cost nothing: in a siege zone, one while a
// Charm of Courage is held, which uses the charm up; in any other PvP zone
// (an arena), one to a player or its summon. A player killed inside a siege
// zone without the charm still loses exp, at a quarter of the normal rate.
//
// Deferred pending owning subsystems: the mutual-clan-war halving of
// percentLost (Player.java:2906, `atWar`) — clan-war state isn't tracked yet
// (#149).
//
// The siege-zone and festival-participant reductions are wired: InSiegeZone
// and FestivalParticipant are live accessors (FestivalParticipant is a
// permanent false stub pending #223, so its branch stays dormant, not
// approximated).
func (c *Character) applyDeathExpKarmaLoss(killer attackable.Combatant) {
	if killer == nil {
		return
	}
	killedByPlayable := actingCharacter(killer) != nil

	c.stateMu.RLock()
	table := c.levelTable
	allow := c.allowDelevel
	rate := c.rateKarmaExpLost
	c.stateMu.RUnlock()
	lucky := c.HasSkill(int(skill.LuckySkillID))
	reducedLoss := c.FestivalParticipant() || c.InSiegeZone()
	exempt, useCharm := deathLossExemption(c.InPvPZone(), c.InSiegeZone(),
		c.EffectList().IsAffected(effect.FlagCharmOfCourage), killedByPlayable)

	var hooks progressionHooks
	defer func() { hooks.run() }()
	c.progressionMu.Lock()
	defer c.progressionMu.Unlock()
	c.ExpBeforeDeath = 0
	if table == nil || !allow || (lucky && c.CharLevel <= 9) {
		return
	}
	if exempt {
		if useCharm {
			// The charm's end broadcasts the status window with the charm
			// cleared; it runs after progressionMu is released.
			hooks.add(func() { c.EffectList().StopByType(effect.TypeCharmOfCourage) })
		}
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
	if reducedLoss {
		percentLost /= 4.0
	}

	span := table.ExpSpanAtLevel(c.CharLevel)
	lostExp := int64(math.Round(float64(span) * percentLost / 100))

	// Snapshot the pre-loss exp for a later resurrection to restore from
	// (Player.java:2919, `setExpBeforeDeath(getStatus().getExp())`).
	c.ExpBeforeDeath = c.Exp

	c.updateKarmaLoss(table, lostExp, &hooks)
	// The loss is a negative experience add, not a removal: one that would
	// take experience below zero is dropped rather than floored, no loss
	// message goes out, and UserInfo is sent either way.
	c.addExp(table, c.template(), -lostExp, &hooks)
	hooks.add(c.UpdateUserInfo)
}

// RestoreExp restores restorePercent (0-100) of the experience lost in this
// character's last death, matching Player.restoreExp (Player.java:2865-2872).
// It is a no-op unless a death has left ExpBeforeDeath set, and always
// clears ExpBeforeDeath afterward.
func (c *Character) RestoreExp(restorePercent float64) {
	c.stateMu.RLock()
	table := c.levelTable
	c.stateMu.RUnlock()

	c.progressionMu.Lock()
	if c.ExpBeforeDeath <= 0 || table == nil {
		c.progressionMu.Unlock()
		return
	}
	restored := int64(math.Round(float64(c.ExpBeforeDeath-c.Exp) * restorePercent / 100))
	c.ExpBeforeDeath = 0
	// A bare experience add: UserInfo, but no reward message.
	var hooks progressionHooks
	c.addExp(table, c.template(), restored, &hooks)
	c.progressionMu.Unlock()
	hooks.run()
	c.UpdateUserInfo()
}

// UpdateKarmaLoss reduces this character's karma for exp experience earned
// from a kill, announcing the new total when it changes.
func (c *Character) UpdateKarmaLoss(table *LevelTable, exp int64) {
	if table == nil {
		return
	}
	var hooks progressionHooks
	c.progressionMu.Lock()
	c.updateKarmaLoss(table, exp, &hooks)
	c.progressionMu.Unlock()
	hooks.run()
}

// updateKarmaLoss reduces this character's karma by an experience amount
// (gained from a kill or lost to a death), matching Player.updateKarmaLoss
// (Player.java:2749-2757) and Formulas.calculateKarmaLost
// (Formulas.java:1267-1270). A cursed-weapon holder keeps its karma; that
// gate stays dormant until cursed weapons are modeled (#225).
//
// The caller holds progressionMu; the karma announcement, UserInfo and
// relation broadcast run from hooks, in Player.setKarma's order.
func (c *Character) updateKarmaLoss(table *LevelTable, lostExp int64, hooks *progressionHooks) {
	if c.KarmaPoints <= 0 || c.CursedWeaponEquipped() {
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
	hooks.add(func() { c.notifyKarmaChanged(newKarma) })
	hooks.add(c.UpdateUserInfo)
	hooks.add(c.BroadcastRelations)
}
