package manager

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

func (n *Npcs) RespawnHook(actorID int32) func() {
	n.mu.Lock()
	key, tracked := n.live[actorID]
	if tracked {
		delete(n.live, actorID)
		n.liveCount--
	}
	slot, ok := n.slot[key]
	n.mu.Unlock()
	if !tracked || !ok {
		return nil
	}

	if obj, ok := n.state.Object(actorID); ok {
		if h, ok := obj.(*npc.Hostile); ok {
			n.despawnMinions(h)
			if master := h.Master(); master != nil && slot.entry.RespawnDelay <= 0 {
				master.RemoveMinion(actorID)
				h.SetMaster(nil)
			}
		}
	}
	if obj, ok := n.state.Object(actorID); ok {
		switch a := obj.(type) {
		case *npc.Hostile:
			n.ai.Remove(a)
		case *npc.Folk:
			n.ai.Remove(a)
		}
	}
	n.walker.StopRouteByID(actorID)

	delay := spawn.CalculateRespawnDelay(slot.entry)
	if delay <= 0 {
		n.mu.Lock()
		delete(n.slot, key)
		n.mu.Unlock()
		return nil
	}

	return func() { n.scheduleRespawn(slot, delay) }
}

// Remove takes h out of the world at once, with no corpse and no decay
// wait, and arms its spawn slot's respawn exactly as a decayed corpse does.
// It runs on h's own queue (npc.Hostile.DeleteMe posts it there).
func (n *Npcs) Remove(h *npc.Hostile) {
	if n.decay != nil {
		n.decay.Cancel(h)
	}
	h.Decay(n.state, n.RespawnHook(h.ObjectID()))
}

// scheduleRespawn arms slot's respawn delay from now, unless DespawnAll
// dropped the slot since its NPC decayed: a despawned spawn stays gone.
func (n *Npcs) scheduleRespawn(slot slotInfo, delay time.Duration) {
	now := n.now()
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, ok := n.slot[slot.key]; !ok {
		return
	}
	if slot.dbName != "" {
		if state, ok := n.spawns.State(slot.dbName); ok {
			state.SetRespawn(delay, now)
		}
	}
	n.respawn.Add(slot.key, now.Add(delay))
}

// Respawn implements task.RespawnEffects: it re-instantiates the slot key
// identifies, picking a fresh position (and, for a database-tracked slot,
// resuming through the same persisted-state restore rule used at boot).
// A slot DespawnAll dropped is not respawned.
func (n *Npcs) Respawn(key string) {
	n.gate.RLock()
	defer n.gate.RUnlock()
	n.mu.Lock()
	slot, ok := n.slot[key]
	n.mu.Unlock()
	if !ok {
		return
	}

	tmpl, ok := n.templates.Get(int(slot.entry.NPCID))
	if !ok {
		return
	}
	if slot.masterID != 0 {
		obj, _ := n.state.Object(slot.masterID)
		master, ok := obj.(*npc.Hostile)
		if !ok || master.Dead() {
			n.mu.Lock()
			delete(n.slot, key)
			n.mu.Unlock()
			return
		}
		master.RemoveMinion(slot.liveID)
		n.instantiate(key, slot.entry, tmpl, n.privateSpawnLocation(master, tmpl), master.Heading(), fullHP, fullMP, master)
		return
	}

	if slot.dbName != "" {
		state, ok := n.currentSpawns().State(slot.dbName)
		if !ok {
			state = spawn.NewState(slot.dbName)
		}
		n.spawnPersisted(key, slot.maker, slot.entry, tmpl, state)
		return
	}
	pos, ok := n.pickSpawnPosition(slot.maker, slot.entry)
	if !ok {
		n.deferredCount.Add(1)
		return
	}
	n.spawnFresh(key, slot.entry, tmpl, pos)
}

func (n *Npcs) despawnMinions(master *npc.Hostile) {
	for _, minion := range master.Minions() {
		id := minion.ObjectID()
		n.ai.Remove(minion)
		n.walker.StopRouteByID(id)
		minion.Decay(n.state, nil)
		minion.SetMaster(nil)
		master.RemoveMinion(id)

		n.mu.Lock()
		if _, ok := n.live[id]; ok {
			delete(n.live, id)
			n.liveCount--
		}
		for key, slot := range n.slot {
			if slot.masterID == master.ObjectID() {
				delete(n.slot, key)
			}
		}
		n.mu.Unlock()
	}
}

// SyncPersistedState writes every live database-tracked slot's current HP,
// MP, and position back into its spawn.State row, ready for Spawns.Save. Dead
// rows (mid respawn countdown) are left untouched by State.SetStats itself.
func (n *Npcs) SyncPersistedState() {
	n.mu.Lock()
	live := make(map[int32]string, len(n.live))
	for id, key := range n.live {
		live[id] = key
	}
	n.mu.Unlock()

	for id, key := range live {
		n.mu.Lock()
		slot, ok := n.slot[key]
		n.mu.Unlock()
		if !ok || slot.dbName == "" {
			continue
		}
		obj, ok := n.state.Object(id)
		if !ok {
			continue
		}
		hostile, ok := obj.(*npc.Hostile)
		if !ok {
			continue
		}
		state, ok := n.currentSpawns().State(slot.dbName)
		if !ok {
			continue
		}
		x, y, z := hostile.Position()
		state.SetStats(hostile.CurrentHP(), hostile.CurrentMP(), location.Location{X: x, Y: y, Z: z}, hostile.Heading())
	}
}

// LiveCount returns the number of currently spawned (not decayed) NPCs,
// civilian ones included.
func (n *Npcs) LiveCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.liveCount
}

// DeferredCount returns the number of spawn entries skipped at boot because
// no usable explicit or territory-random position could be chosen.
func (n *Npcs) DeferredCount() int {
	return int(n.deferredCount.Load())
}

// RestoredDeadCount returns the number of database-tracked entries that
// were still dead with a pending respawn deadline at boot.
func (n *Npcs) RestoredDeadCount() int {
	return int(n.restoredDeadCount.Load())
}

// SkippedNonCombatCount returns the number of spawn entries skipped at boot
// for resolving to a non-combat instance type that is not a civilian NPC
// either (castle artifacts, siege flags, towers and the like).
func (n *Npcs) SkippedNonCombatCount() int {
	return int(n.skippedNonCombatCount.Load())
}

// FolkCount returns the number of civilian NPCs spawned, respawns
// included.
func (n *Npcs) FolkCount() int {
	return int(n.folkCount.Load())
}

// pickPosition selects one spawn position from positions. A single entry
// (the "fixed" declaration) is used exactly as declared, heading included.
// Multiple entries (the "chance-weighted" declaration) are chosen by
// rolling a percentage against each Chance in turn — and, for this shape,
// the winning entry's declared heading is discarded in favor of a fresh
// random one. A weight
// table that doesn't sum to 100 falls back to the last entry rather than
// leaving the slot unspawned.
