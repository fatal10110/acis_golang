package script

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// dyingDelay is how long after its death an NPC's dying hooks run.
const dyingDelay = 3 * time.Second

// HostileCreated runs the created hook of every script bound to h's
// template, in list order: h has just entered the world.
func (r *Registry) HostileCreated(h *npc.Hostile) {
	r.created(int32(h.NpcID()), h)
}

// FolkCreated is HostileCreated for the civilian NPC f.
func (r *Registry) FolkCreated(f *npc.Folk) {
	r.created(int32(f.NpcID()), f)
}

func (r *Registry) created(npcID int32, self attackable.Combatant) {
	list := r.scripts(npcID, EventCreated)
	if len(list) == 0 {
		return
	}
	e := Created{NPC: NPCOf(self)}
	for _, s := range list {
		r.run(s, hookCreated, func() { s.Hooks.Created(s, e) })
	}
}

// HostileDecayed runs the decayed hook of every script bound to h's
// template, in list order: h has decayed or was deleted and has not left
// the world yet.
func (r *Registry) HostileDecayed(h *npc.Hostile) {
	r.decayed(int32(h.NpcID()), h)
}

// FolkDecayed is HostileDecayed for the civilian NPC f.
func (r *Registry) FolkDecayed(f *npc.Folk) {
	r.decayed(int32(f.NpcID()), f)
}

func (r *Registry) decayed(npcID int32, self attackable.Combatant) {
	list := r.scripts(npcID, EventDecayed)
	if len(list) == 0 {
		return
	}
	e := Decayed{NPC: NPCOf(self)}
	for _, s := range list {
		r.run(s, hookDecayed, func() { s.Hooks.Decayed(s, e) })
	}
}

// HostileDying schedules the dying hook of every script bound to h's
// template: three seconds after killer killed h, they run in list order on
// the engine queue, whatever h has become by then.
func (r *Registry) HostileDying(h *npc.Hostile, killer attackable.Combatant) {
	r.dying(int32(h.NpcID()), h, killer)
}

// FolkDying is HostileDying for the civilian NPC f.
func (r *Registry) FolkDying(f *npc.Folk, killer attackable.Combatant) {
	r.dying(int32(f.NpcID()), f, killer)
}

func (r *Registry) dying(npcID int32, self, killer attackable.Combatant) {
	list := r.scripts(npcID, EventMyDying)
	if len(list) == 0 {
		return
	}
	if r.queue == nil {
		r.log.Error().Int32("npc_id", npcID).Msg("script: no engine queue to run the dying hooks on")
		return
	}
	e := MyDying{NPC: NPCOf(self), Killer: creatureOf(killer)}
	r.queue.After(dyingDelay, func() {
		for _, s := range list {
			r.run(s, hookMyDying, func() { s.Hooks.MyDying(s, e) })
		}
	})
}
