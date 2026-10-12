package script

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"

// The NPC operations on its party: its master and whether it keeps
// privates. A hostile and a civilian NPC each can be a master and a
// private.

// Master returns the NPC's master, nil for none: an NPC that is no private,
// or a private whose master died, which let its privates go. A handle on
// nothing panics.
func (n *NPC) Master() *NPC {
	if m := memberOf(n, "the master").Master(); m != nil {
		return NPCOf(m)
	}
	return nil
}

// HasMaster reports whether the NPC has a master.
func (n *NPC) HasMaster() bool { return n.Master() != nil }

// IsMaster reports whether the NPC keeps privates: it has created privates
// in this life or an earlier one of its spawn, even ones gone since. A
// handle on nothing panics.
func (n *NPC) IsMaster() bool { return memberOf(n, "the privates").IsMaster() }

// Minions returns the privates of the NPC's party, dead ones included, in
// no particular order: its master's, the NPC among them, when it has a
// master. Otherwise they are its own, and the NPC is a master from then
// on (IsMaster), with no private at first. A handle on nothing panics.
func (n *NPC) Minions() []*NPC {
	members := memberOf(n, "the privates").PartyMinions()
	out := make([]*NPC, 0, len(members))
	for _, m := range members {
		if h := NPCOf(m); h != nil {
			out = append(out, h)
		}
	}
	return out
}

// memberOf returns the NPC n handles, a hostile or a civilian one. A handle
// on nothing panics.
func memberOf(n *NPC, what string) npc.PartyMember {
	m, ok := n.combatant().(npc.PartyMember)
	if !ok {
		panic("script: " + what + " of a handle on nothing")
	}
	return m
}
