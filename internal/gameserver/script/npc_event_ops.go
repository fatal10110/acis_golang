package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// The NPC operations a behavior and a quest make on the players around an
// NPC and on the hooks of its party (#3609, #3597).

// skillAnimator is an NPC that shows a cast animation around itself.
type skillAnimator interface {
	BroadcastSkillAnimation(casterID int32, casterAt location.Location, targetID int32, targetAt location.Location, skillID, level int32, hitTime, reuseDelay int)
}

// BroadcastMagicSkillUse shows every player around the NPC the cast
// animation of skillID at level from caster to target, with hitTime and
// reuseDelay in milliseconds (Creature.broadcastPacket of a MagicSkillUse).
// It is only shown: no skill is used, and caster and target may be any
// creatures. Each stands where it is at the call. A handle on nothing
// panics.
func (n *NPC) BroadcastMagicSkillUse(caster, target Creature, skillID, level int32, hitTime, reuseDelay int) {
	a, ok := n.combatant().(skillAnimator)
	if !ok {
		panic("script: a cast animation around a handle on nothing")
	}
	from, to := locOf(caster), locOf(target)
	a.BroadcastSkillAnimation(
		caster.ObjectID(), location.Location{X: from.X, Y: from.Y, Z: from.Z},
		target.ObjectID(), location.Location{X: to.X, Y: to.Y, Z: to.Z},
		skillID, level, hitTime, reuseDelay,
	)
}

// RunAttacked runs the script's own attacked hook, as if attacker had
// attacked the NPC for damage with no skill: a direct call of the hook on
// the caller's goroutine, inside the caller's invocation. Nothing is
// checked and no other script is called.
func (s *Script) RunAttacked(n *NPC, attacker Creature, damage int32) {
	s.Hooks.Attacked(s, Attacked{NPC: n, Attacker: attacker, Damage: damage})
}

// RaisePartyAttacked runs, before it returns, the party-attacked hook of
// every script bound to called's template, in list order: caller, attacked
// by target for damage, called its party member called. Each hook is one
// invocation, run on the caller's goroutine. Nothing is checked: called may
// be dead or caller itself. A civilian NPC is bound to no party-attacked
// hook, so it is called for nothing; neither is a handle on nothing.
func (s *Script) RaisePartyAttacked(caller, called *NPC, target Creature, damage int32) {
	h, ok := called.combatant().(*npc.Hostile)
	if !ok {
		return
	}
	s.registry.raisePartyAttacked(h, func() PartyAttacked {
		return PartyAttacked{Caller: caller, Called: called, Target: target, Damage: damage}
	})
}
