package script

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"

// The NPC operations on its party: its master and whether it keeps
// privates. Only a hostile NPC is in a party; a civilian NPC has no master
// and keeps no privates.

// Master returns the NPC's master, nil for none: an NPC that is no private,
// or a private whose master died, which let its privates go. A handle on
// nothing panics.
func (n *NPC) Master() *NPC {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		if m := o.Master(); m != nil {
			return NewNPC(m)
		}
		return nil
	case *npc.Folk:
		return nil
	}
	panic("script: the master of a handle on nothing")
}

// HasMaster reports whether the NPC has a master.
func (n *NPC) HasMaster() bool { return n.Master() != nil }

// IsMaster reports whether the NPC keeps privates: it has created privates
// in this life, even ones gone since. A handle on nothing panics.
func (n *NPC) IsMaster() bool {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		return o.IsMaster()
	case *npc.Folk:
		return false
	}
	panic("script: the privates of a handle on nothing")
}
