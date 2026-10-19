package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// The cast and attack state a script reads of a hostile NPC, and the cast it
// starts.

// hpmpGate is the cast controller gate on a cast's MP and HP alone.
type hpmpGate interface {
	MeetsHPMP(target attackable.Combatant, ref skill.Ref) bool
}

// MeetsHPMP reports whether h has the MP and HP its cast of ref at target
// takes, whatever mutes it; false for no target or a skill h's table lacks.
func (h *Hostile) MeetsHPMP(target attackable.Combatant, ref skill.Ref) bool {
	gate, ok := h.brain.CastController().(hpmpGate)
	return ok && gate.MeetsHPMP(target, ref)
}

// MeetsHPMPDisabled is MeetsHPMP that also refuses a skill a mute keeps h
// from casting: a magic one while h is muted, any other while it is
// physically muted.
func (h *Hostile) MeetsHPMPDisabled(target attackable.Combatant, ref skill.Ref) bool {
	cast := h.brain.CastController()
	return cast != nil && cast.MeetsHPMPDisabled(target, ref)
}

// CanAttemptCast reports whether h may try its cast of ref at target as far
// as its skill reuse goes. With no skill waiting for its reuse, it may.
// Otherwise a stun, meditation, sleep, paralysis or fear locks every
// skill, and failing that ref's own reuse wait decides. False for no
// target or a skill h's table lacks.
func (h *Hostile) CanAttemptCast(target attackable.Combatant, ref skill.Ref) bool {
	cast := h.brain.CastController()
	if cast == nil {
		return false
	}
	// The lock is read before ref's own wait, whose check forgets an
	// expired wait: an expired wait still tracked leaves the lock in force.
	if h.skillsLocked() {
		return false
	}
	return cast.CanAttempt(target, ref)
}

// skillsLocked reports whether crowd control keeps h from every skill: it
// holds a stun, meditation, sleep, paralysis or fear while some skill of
// its is tracked as waiting for its reuse, expired or not.
func (h *Hostile) skillsLocked() bool {
	h.skillMu.Lock()
	tracked := len(h.disabledSkills) > 0
	h.skillMu.Unlock()
	return tracked && (h.Stunned() || h.ImmobileUntilAttacked() || h.Sleeping() || h.Paralyzed() || h.Afraid())
}

// AttackingNow reports whether h's attack is in flight.
func (h *Hostile) AttackingNow() bool { return h.brain.AttackingNow() }

// uncheckedCaster is the cast controller start a script's direct cast takes.
type uncheckedCaster interface {
	CastUnchecked(target attackable.Combatant, ref skill.Ref)
}

// CastUnchecked starts h's cast of ref at target at once, past every gate a
// cast's start runs: reuse, MP, HP, mutes, weapon, conditions. A cast h
// already has in flight is replaced. No target, or a skill h's table lacks,
// starts nothing.
func (h *Hostile) CastUnchecked(target attackable.Combatant, ref skill.Ref) {
	if c, ok := h.brain.CastController().(uncheckedCaster); ok && target != nil {
		c.CastUnchecked(target, ref)
	}
}
