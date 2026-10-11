package script

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"

// HostileNoDesire runs the no-desire hook of every script bound to h's
// template, in list order: h has nothing left to do.
func (r *Registry) HostileNoDesire(h *npc.Hostile) {
	list := r.scripts(int32(h.NpcID()), EventNoDesire)
	if len(list) == 0 {
		return
	}
	e := NoDesire{NPC: NewNPC(h)}
	for _, s := range list {
		r.run(s, hookNoDesire, func() { s.Hooks.NoDesire(s, e) })
	}
}

// FolkNoDesire runs the no-desire hook of every script bound to f's
// template, in list order: f has nothing left to do.
func (r *Registry) FolkNoDesire(f *npc.Folk) {
	list := r.scripts(int32(f.NpcID()), EventNoDesire)
	if len(list) == 0 {
		return
	}
	e := NoDesire{NPC: NPCOf(f)}
	for _, s := range list {
		r.run(s, hookNoDesire, func() { s.Hooks.NoDesire(s, e) })
	}
}

// HostileMoveToFinished runs the move-finished hook of every script bound
// to h's template, in list order: a walk to a point, or a flight, ended
// with h at x, y, z.
func (r *Registry) HostileMoveToFinished(h *npc.Hostile, x, y, z int32) {
	list := r.scripts(int32(h.NpcID()), EventMoveToFinished)
	if len(list) == 0 {
		return
	}
	e := MoveToFinished{NPC: NewNPC(h), X: x, Y: y, Z: z}
	for _, s := range list {
		r.run(s, hookMoveToFinished, func() { s.Hooks.MoveToFinished(s, e) })
	}
}

// HostileOutOfTerritory runs the out-of-territory hook of every script
// bound to h's template, in list order: h arrived outside its territory.
func (r *Registry) HostileOutOfTerritory(h *npc.Hostile) {
	list := r.scripts(int32(h.NpcID()), EventOutOfTerritory)
	if len(list) == 0 {
		return
	}
	e := OutOfTerritory{NPC: NewNPC(h)}
	for _, s := range list {
		r.run(s, hookOutOfTerritory, func() { s.Hooks.OutOfTerritory(s, e) })
	}
}
