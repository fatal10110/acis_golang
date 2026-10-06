package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// ScriptHooks is the script engine as an NPC raises its hooks. Each
// method runs the hooks synchronously on the caller's goroutine, with no
// NPC lock held.
type ScriptHooks interface {
	// Behaves reports whether a behavior is bound to the NPC template id.
	// Such an NPC leaves its reactions to the behavior: the built-in
	// attacked hate, party assist, shot recharge and idle follow and wander
	// stay off.
	Behaves(npcID int32) bool
	// HostileAttacked runs h's attacked hooks: attacker attacked it for
	// damage, with sk when a skill did it (the zero Ref otherwise).
	HostileAttacked(h *Hostile, attacker attackable.Combatant, damage int32, sk skill.Ref)
	// HostilePartyAttacked runs called's party-attacked hooks: caller,
	// attacked by target for damage, called its party member called.
	HostilePartyAttacked(caller, called *Hostile, target attackable.Combatant, damage int32)
	// HostileNoDesire runs h's no-desire hooks: h has nothing left to do.
	HostileNoDesire(h *Hostile)
	// FolkNoDesire runs f's no-desire hooks: f has nothing left to do.
	FolkNoDesire(f *Folk)
	// HostileMoveToFinished runs h's move-finished hooks: a walk to a
	// point, or a flight, ended with h at x, y, z.
	HostileMoveToFinished(h *Hostile, x, y, z int32)
	// HostileOutOfTerritory runs h's out-of-territory hooks: h arrived
	// outside its territory, the first time since it was last inside.
	HostileOutOfTerritory(h *Hostile)
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

// AtHookPoint is where h's AI loop gives the behavior bound to h's template
// its turn, with the AI loop unlocked: the no-desire, move-finished and
// out-of-territory hooks run there. The see-creature point raises nothing
// yet.
func (h *Hostile) AtHookPoint(p ai.HookPoint) {
	if h.scripts == nil {
		return
	}
	switch p {
	case ai.HookNoDesire:
		h.scripts.HostileNoDesire(h)
	case ai.HookMoveFinished:
		x, y, z := h.Position()
		h.scripts.HostileMoveToFinished(h, int32(x), int32(y), int32(z))
	case ai.HookOutOfTerritory:
		h.scripts.HostileOutOfTerritory(h)
	}
}

// AtHookPoint is where f's AI tick gives the behavior bound to f's template
// its turn, on f's queue with no lock held: the tick's state is queue-owned.
// The no-desire hooks run there; the see-creature point raises nothing yet.
func (f *Folk) AtHookPoint(p ai.HookPoint) {
	if f.scripts != nil && p == ai.HookNoDesire {
		f.scripts.FolkNoDesire(f)
	}
}
