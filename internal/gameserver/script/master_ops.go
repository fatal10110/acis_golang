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
// in this life, even ones gone since. A handle on nothing panics.
func (n *NPC) IsMaster() bool { return memberOf(n, "the privates").IsMaster() }

// memberOf returns the NPC n handles, a hostile or a civilian one. A handle
// on nothing panics.
func memberOf(n *NPC, what string) npc.PartyMember {
	m, ok := n.combatant().(npc.PartyMember)
	if !ok {
		panic("script: " + what + " of a handle on nothing")
	}
	return m
}
