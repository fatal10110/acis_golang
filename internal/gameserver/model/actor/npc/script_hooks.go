package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// ScriptHooks is the script engine as a hostile NPC raises its hooks. Each
// method runs the hooks synchronously on the caller's goroutine, with no
// NPC lock held.
type ScriptHooks interface {
	// Behaves reports whether a behavior is bound to the NPC template id.
	// Such an NPC leaves its reactions to the behavior: the built-in
	// attacked hate, party assist and shot recharge stay off.
	Behaves(npcID int32) bool
	// HostileAttacked runs h's attacked hooks: attacker attacked it for
	// damage, with sk when a skill did it (the zero Ref otherwise).
	HostileAttacked(h *Hostile, attacker attackable.Combatant, damage int32, sk skill.Ref)
	// HostilePartyAttacked runs called's party-attacked hooks: caller,
	// attacked by target for damage, called its party member called.
	HostilePartyAttacked(caller, called *Hostile, target attackable.Combatant, damage int32)
	// HostileDecayed runs once h has decayed, before it leaves the world:
	// the behavior timers bound to h stop.
	HostileDecayed(h *Hostile)
}

// behaves reports whether a behavior is bound to h's template id.
func (h *Hostile) behaves() bool {
	return h.scripts != nil && h.scripts.Behaves(int32(h.NpcID()))
}

// raiseAttacked runs h's attacked hooks.
func (h *Hostile) raiseAttacked(attacker attackable.Combatant, damage int32, sk skill.Ref) {
	if h.scripts != nil {
		h.scripts.HostileAttacked(h, attacker, damage, sk)
	}
}

// partyAttacked tells h that caller, its party member, was attacked by
// target for damage: h's party-attacked hooks run, then, when no behavior
// is bound to h, its built-in party assist. assist is false where the
// attack has no built-in assist at all (a skill's attacked call).
func (h *Hostile) partyAttacked(caller *Hostile, target attackable.Combatant, damage int, assist bool) {
	if h.scripts != nil {
		h.scripts.HostilePartyAttacked(caller, h, target, int32(damage))
	}
	if assist && !h.behaves() {
		h.reactPartyAttacked(caller, target, damage)
	}
}

// raiseDecayed tells the script engine that h has decayed.
func (h *Hostile) raiseDecayed() {
	if h.scripts != nil {
		h.scripts.HostileDecayed(h)
	}
}
