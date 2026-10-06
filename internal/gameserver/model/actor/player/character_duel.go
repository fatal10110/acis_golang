package player

import (
	"sync/atomic"

	"github.com/fatal10110/acis_golang/internal/gameserver/duel"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// duelStanding is the character's place in a duel. The duel's own queue,
// the character's queue and an attacker's queue (a hit interrupting the
// duel) all write it, so every field is atomic.
type duelStanding struct {
	id    atomic.Int32
	state atomic.Int32
	team  atomic.Int32
	// skillsLocked is the lock a duel defeat puts on every skill until a
	// cast stop or the duel's end lifts it.
	skillsLocked atomic.Bool
}

var _ duel.Standing = (*Character)(nil)

// DuelID is the duel the character is in, 0 when it is in none.
func (c *Character) DuelID() int32 { return c.duel.id.Load() }

// InDuel reports whether the character is in a duel.
func (c *Character) InDuel() bool { return c.DuelID() > 0 }

// DuelState is the character's standing in its duel.
func (c *Character) DuelState() duel.State { return duel.State(c.duel.state.Load()) }

// SetDuelState records the character's standing in its duel.
func (c *Character) SetDuelState(s duel.State) { c.duel.state.Store(int32(s)) }

// JoinDuel puts the character in duel id, counting down.
func (c *Character) JoinDuel(id int32) {
	c.duel.state.Store(int32(duel.Countdown))
	c.duel.id.Store(id)
}

// LeaveDuel takes the character out of its duel. A defeated character gets
// its skills back.
func (c *Character) LeaveDuel() {
	if c.DuelState() == duel.Dead {
		c.EnableAllSkills()
	}
	c.duel.state.Store(int32(duel.NoDuel))
	c.duel.id.Store(0)
}

// DuelTeam is the team colour the character shows: a duel side's, or none.
func (c *Character) DuelTeam() int { return int(c.duel.team.Load()) }

// SetDuelTeam sets the team colour the character shows. Callers own the
// client refresh that follows.
func (c *Character) SetDuelTeam(t duel.Team) { c.duel.team.Store(int32(t)) }

// PvPFlagged reports whether the character carries a PvP flag.
func (c *Character) PvPFlagged() bool { return c.PvPFlagState() != task.PvPFlagNone }

// InDuelBlockedZone reports whether the character stands where no duel may
// go on: a peace, siege or PvP zone.
func (c *Character) InDuelBlockedZone() bool {
	return c.InPeaceZone() || c.InSiegeZone() || c.InPvPZone()
}

// InterruptDuel marks the character's duel as interrupted by something from
// outside it; the duel is cancelled on its next check.
func (c *Character) InterruptDuel() { c.SetDuelState(duel.Interrupted) }

// DisableAllSkills locks every skill of the character, as a duel defeat
// does, until EnableAllSkills.
func (c *Character) DisableAllSkills() { c.duel.skillsLocked.Store(true) }

// EnableAllSkills lifts the lock DisableAllSkills put on.
func (c *Character) EnableAllSkills() { c.duel.skillsLocked.Store(false) }

// RestoreDuelVitals puts CP, HP and MP back to the values a duel saved when
// it began, each within its maximum, and reports the change. A dead
// character keeps its values.
func (c *Character) RestoreDuelVitals(cp, hp, mp float64) {
	res := c.ResourceValues()
	c.vitalsMu.Lock()
	if c.dead.Load() {
		c.vitalsMu.Unlock()
		return
	}
	c.curCP = clampVital(cp, res.MaxCP)
	c.writeHPLocked(clampVital(hp, res.MaxHP))
	c.curMP = clampVital(mp, res.MaxMP)
	c.vitalsMu.Unlock()
	c.BroadcastStatus()
}

func clampVital(v, maxV float64) float64 {
	return max(0, min(v, maxV))
}

// StopAllEffects removes every effect on the character, then refreshes its
// view once.
func (c *Character) StopAllEffects() {
	c.EffectList().StopAll()
	c.emit(event.EffectsStripped{})
}

// duelHitAllowed applies a hit from attacker, another creature, to the
// character's duel: a character that already lost or won takes nothing more
// (false), and a hit from outside the duel, or one landing while the
// character does not fight yet, interrupts it.
func (c *Character) duelHitAllowed(attacker attackable.Combatant) bool {
	if !c.InDuel() {
		return true
	}
	state := c.DuelState()
	if state == duel.Dead || state == duel.Winner {
		return false
	}
	other := actingCharacter(attacker)
	if other == nil || other.DuelID() != c.DuelID() || state != duel.Duelling {
		c.InterruptDuel()
	}
	return true
}

// duelKillExempt reports whether a kill between killer and victim, both in
// a duel, leaves karma and PvP counts alone.
func duelKillExempt(killer, victim *Character) bool {
	return killer.InDuel() && victim.InDuel()
}

// actorWithOwner is what resolving an acting player needs: a summon acts
// for its owner, anything else for itself.
type actorWithOwner interface {
	Kind() actor.Kind
	Owner() (attackable.Combatant, bool)
}

// actingStanding returns the duel standing of the player acting through a.
func actingStanding(a actorWithOwner) (duel.Standing, bool) {
	if a == nil {
		return nil, false
	}
	var p any = a
	switch a.Kind() {
	case actor.KindPlayer:
	case actor.KindSummon:
		owner, ok := a.Owner()
		if !ok || owner == nil {
			return nil, false
		}
		p = owner
	default:
		return nil, false
	}
	s, ok := p.(duel.Standing)
	return s, ok
}

// inSameActiveDuel reports whether c and the player acting through a fight
// each other in the same duel.
func (c *Character) inSameActiveDuel(a actorWithOwner) bool {
	other, ok := actingStanding(a)
	return ok && duel.SameActive(c, other)
}
