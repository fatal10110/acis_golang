package manager

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// DespawnAll removes every NPC from the world for good, as //unspawnall
// does with SpawnManager.despawn and then World.deleteVisibleNpcSpawns:
//
//   - every spawn slot is dropped and its pending respawn cancelled, so no
//     NPC comes back, not even one whose corpse decays meanwhile;
//   - every database-tracked spawn row of the spawn list is reset to
//     uninitialized (SpawnData.setStatus(-1)), so the next save drops it;
//   - every NPC in the world is deleted: the ones the spawn list placed,
//     the standalone ones, and the ones no spawn placed (item decorations,
//     signet points, NPCs other systems placed). Each leaves on its own
//     queue.
//
// It reports how many NPCs it deleted from the world.
func (n *Npcs) DespawnAll() int {
	n.gate.Lock()
	defer n.gate.Unlock()

	n.mu.Lock()
	for key := range n.slot {
		n.respawn.Cancel(key)
	}
	for _, maker := range n.spawns.Table().Makers() {
		for _, entry := range maker.Entries {
			if entry.DBName == "" {
				continue
			}
			if state, ok := n.spawns.State(entry.DBName); ok {
				state.Status = spawn.StatusUninitialized
			}
		}
	}
	clear(n.slot)
	for _, g := range n.groups {
		clear(g.keys)
	}
	clear(n.live)
	n.liveCount = 0
	n.mu.Unlock()

	deleted := 0
	for _, obj := range n.state.Objects() {
		if n.deleteNpc(obj) {
			deleted++
		}
	}
	return deleted
}

// deleteNpc deletes obj from the world, with no corpse and no respawn, when
// it is an NPC: Npc.deleteMe. It reports false for anything else.
func (n *Npcs) deleteNpc(obj world.Tracked) bool {
	switch o := obj.(type) {
	case *npc.Hostile:
		onQueue(o.Queue(), func() {
			n.forget(o, o.ObjectID())
			o.Decay(n.state, nil)
		})
	case *npc.Folk:
		onQueue(o.Queue(), func() {
			n.forget(o, o.ObjectID())
			o.Decay(n.state, nil)
		})
	case *npc.Decoration:
		// Out of its zones first, then out of the world.
		o.Despawn()
	case *npc.EffectPoint:
		onQueue(o.Queue(), o.Despawn)
	default:
		return false
	}
	return true
}

// forget takes the NPC a, of object id id, off the corpse decay, AI and
// walker tasks.
func (n *Npcs) forget(a interface {
	task.DecayActor
	task.AIActor
}, id int32,
) {
	n.decay.Cancel(a)
	n.ai.Remove(a)
	n.walker.StopRouteByID(id)
}

// onQueue runs fn on q, or at once without a queue.
func onQueue(q *sim.Queue, fn func()) {
	if q == nil {
		fn()
		return
	}
	q.Post(fn)
}

// RespawnAll puts spawns in place of the spawn list and starts every
// on-start maker of it, the makers of the listed spawn events and the
// Seven Signs groups, as SpawnManager.reload does after //respawnall's
// despawn: the database-tracked spawns come back as spawns' rows say. The
// slots of an earlier spawn list are not touched; //respawnall runs
// DespawnAll first, which drops them all. Only the swap holds the gate:
// each NPC then read-holds it while it is placed, and its created hooks
// run with no lock held.
func (n *Npcs) RespawnAll(spawns *Spawns) {
	n.gate.Lock()
	n.mu.Lock()
	n.spawns = spawns
	n.gen++
	gen := n.gen
	n.mu.Unlock()
	n.gate.Unlock()

	n.spawnOnStart(spawns, gen)
}
