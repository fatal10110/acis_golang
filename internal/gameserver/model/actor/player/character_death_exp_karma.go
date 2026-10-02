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
// of this character's death. It is a no-op for an environmental death
// (killer == nil); any other death first clears the previous
// death's exp snapshot, so a later resurrection restores nothing unless this
// death takes exp. The loss is skipped while the delevel gate is closed: the
// AllowDelevel config off, or the Lucky skill still active below level 10.
//
// Inside a PvP zone two deaths cost nothing: in a siege zone, one while a
// Charm of Courage is held, which uses the charm up; in any other PvP zone
// (an arena), one to a player or its summon. A player killed inside a siege
// zone without the charm still loses exp, at a quarter of the normal rate,
// as does one killed by a player (or its summon) when either of their clans
// has declared war on the other.
//
// The siege-zone and festival-participant reductions are wired: InSiegeZone
// and FestivalParticipant are live accessors (FestivalParticipant is a
// permanent false stub pending #223, so its branch stays dormant, not
// approximated).
func (c *Character) applyDeathExpKarmaLoss(killer attackable.Combatant) {
	if killer == nil {
		return
	}
	pk := actingCharacter(killer)

	c.stateMu.RLock()
	allow := c.allowDelevel
	c.stateMu.RUnlock()
	lucky := c.HasSkill(int(skill.LuckySkillID))
	loss := c.deathLossTerms(pk != nil, c.clanWarDeath(pk))

	var hooks progressionHooks
	defer func() { hooks.run() }()
	c.progressionMu.Lock()
	defer c.progressionMu.Unlock()
	c.ExpBeforeDeath = 0
	if loss.table == nil || !allow || (lucky && c.CharLevel <= 9) {
		return
	}
	c.applyDeathPenaltyLocked(loss, &hooks)
}

// ApplyDeathPenalty takes a full death's experience and karma loss from
// this character without a death: a clan's surrender costs its leader
// that, and a personal surrender the member. The delevel gate does not apply; the PvP-zone exemptions do, as
// for a death no playable caused.
func (c *Character) ApplyDeathPenalty() {
	loss := c.deathLossTerms(false, false)
	if loss.table == nil {
		return
	}
	var hooks progressionHooks
	c.progressionMu.Lock()
	c.applyDeathPenaltyLocked(loss, &hooks)
	c.progressionMu.Unlock()
	hooks.run()
}

// deathLoss is what a death's experience loss is computed from, read
// before progressionMu is taken.
type deathLoss struct {
	table            *LevelTable
	rate             float64
	reducedLoss      bool
	exempt, useCharm bool
}

// deathLossTerms reads the level table, the karma rate and the zone and
// effect state a death's experience loss depends on; atWar reports a death
// between clans at war, which costs a quarter of the normal loss.
func (c *Character) deathLossTerms(killedByPlayable, atWar bool) deathLoss {
	c.stateMu.RLock()
	loss := deathLoss{table: c.levelTable, rate: c.rateKarmaExpLost}
	c.stateMu.RUnlock()
	loss.reducedLoss = c.FestivalParticipant() || atWar || c.InSiegeZone()
	loss.exempt, loss.useCharm = deathLossExemption(c.InPvPZone(), c.InSiegeZone(),
		c.EffectList().IsAffected(effect.FlagCharmOfCourage), killedByPlayable)
	return loss
}

// applyDeathPenaltyLocked takes the experience and karma one death costs;
// the caller holds progressionMu.
func (c *Character) applyDeathPenaltyLocked(loss deathLoss, hooks *progressionHooks) {
	table := loss.table
	if loss.exempt {
		if loss.useCharm {
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
		percentLost *= loss.rate
	}
	if loss.reducedLoss {
		percentLost /= 4.0
	}

	span := table.ExpSpanAtLevel(c.CharLevel)
	lostExp := int64(math.Round(float64(span) * percentLost / 100))

	// Snapshot the pre-loss exp for a later resurrection to restore from.
	c.ExpBeforeDeath = c.Exp

	c.updateKarmaLoss(table, lostExp, hooks)
	// The loss is a negative experience add, not a removal: one that would
	// take experience below zero is dropped rather than floored, no loss
	// message goes out, and UserInfo is sent either way.
	c.addExp(table, c.template(), -lostExp, hooks)
	hooks.add(c.UpdateUserInfo)
}

// RestoreExp restores restorePercent (0-100) of the experience lost in this
// character's last death.
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
// (gained from a kill or lost to a death), through the karma-lost formula.
// A cursed-weapon holder keeps its karma; that
// gate stays dormant until cursed weapons are modeled (#225).
//
// The caller holds progressionMu; the karma announcement, UserInfo and
// relation broadcast run from hooks, in the karma-change order.
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
