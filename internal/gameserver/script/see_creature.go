package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// SeesCreatures reports whether a script reacts to the NPC with id npcID
// seeing a creature; a nil registry binds none.
func (r *Registry) SeesCreatures(npcID int32) bool {
	return len(r.scripts(npcID, EventSeeCreature)) > 0
}

// HostileSeeCreature runs the see-creature hook of every script bound to
// h's template, in list order: h noticed c.
func (r *Registry) HostileSeeCreature(h *npc.Hostile, c attackable.Combatant) {
	list := r.scripts(int32(h.NpcID()), EventSeeCreature)
	if len(list) == 0 {
		return
	}
	e := SeeCreature{NPC: NewNPC(h), Creature: creatureOf(c)}
	for _, s := range list {
		r.run(s, hookSeeCreature, func() { s.Hooks.SeeCreature(s, e) })
	}
}

// FolkSeeCreature runs the see-creature hook of every script bound to f's
// template, in list order: f noticed c.
func (r *Registry) FolkSeeCreature(f *npc.Folk, c attackable.Combatant) {
	list := r.scripts(int32(f.NpcID()), EventSeeCreature)
	if len(list) == 0 {
		return
	}
	e := SeeCreature{NPC: NPCOf(f), Creature: creatureOf(c)}
	for _, s := range list {
		r.run(s, hookSeeCreature, func() { s.Hooks.SeeCreature(s, e) })
	}
}

// LookNeighbor makes the NPC tell its see-creature scripts about every
// creature around it, at most once per 30 s: a call sooner after its last
// look does nothing. A handle on nothing does nothing.
func (n *NPC) LookNeighbor() {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		o.LookNeighbor()
	case *npc.Folk:
		o.LookNeighbor()
	}
}
