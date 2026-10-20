package manager

import (
	"slices"
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
		if m, ok := obj.(npc.PartyMember); ok {
			// The privates leave with their master only while no behavior
			// is bound to it: a behavior's privates outlive a master that
			// decays or is deleted, as a transforming NPC's successor does.
			if n.scripts == nil || !n.scripts.Behaves(int32(m.NpcID())) {
				n.despawnMinions(m)
			}
			// A one-time private leaves its spawn master's minions, even
			// when that master's death already unlinked it.
			if master := m.SpawnMaster(); master != nil && slot.entry.RespawnDelay <= 0 {
				master.RemoveMinion(m)
				m.SetMaster(nil)
			}
			if m.SpawnedPrivates() {
				n.dropGoneScriptPrivates(m)
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

	if g := slot.group; g != nil {
		return func() { n.groupNPCDeleted(g, slot) }
	}

	if slot.scripted {
		// The script that created the private brings it back, if ever.
		n.dropOrphanScriptSlot(key, slot)
		return nil
	}
	delay := spawn.CalculateRespawnDelay(slot.entry)
	if delay <= 0 || slot.fixed {
		n.mu.Lock()
		n.dropSlotLocked(key)
		n.mu.Unlock()
		return nil
	}

	return func() { n.scheduleRespawn(slot, delay) }
}

// groupNPCDeleted answers the NPC of an npcmaker's slot leaving the world:
// the maker decides whether it respawns, then a database-tracked row
// records a respawn time of its own when the spawn respawns at all, and a
// spawn that never respawns drops its slot.
//
// An NPC that leaves during its slot's first placement, before the maker
// counts it as one of its spawn's NPCs, is not reported to the maker, and
// its slot stays, decayed, whatever the respawn delay. A spawn that never
// respawns drops the slot of an NPC its maker armed a respawn for anyway
// from its list: the slot lives on as an orphan.
func (n *Npcs) groupNPCDeleted(g *makerGroup, slot slotInfo) {
	if slot.orphan {
		n.orphanNPCDeleted(g, slot)
		return
	}
	if !slot.first {
		g.behavior.NPCDeleted(g, groupSpawn{g: g, i: slot.spawnIdx}, groupNPC{n: n, slot: slot})
	}

	now := n.now()
	n.mu.Lock()
	defer n.mu.Unlock()
	if slot.entry.RespawnDelay <= 0 {
		if !slot.first {
			n.unlistGroupSlotLocked(g, slot)
		}
		return
	}
	if slot.dbName == "" {
		return
	}
	if _, ok := n.slot[slot.key]; !ok {
		return
	}
	n.spawns.SetRespawn(slot.dbName, spawn.CalculateRespawnDelay(slot.entry), now)
}

// orphanNPCDeleted answers the NPC of an orphan slot of g leaving the
// world: no maker hook runs, it no longer counts alive, a database-tracked
// row records a respawn time when the spawn respawns at all, and the slot
// goes unless a respawn is armed for it.
func (n *Npcs) orphanNPCDeleted(g *makerGroup, slot slotInfo) {
	now := n.now()
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, ok := n.slot[slot.key]; !ok {
		return
	}
	g.orphansAlive--
	if slot.dbName != "" && slot.entry.RespawnDelay > 0 {
		n.spawns.SetRespawn(slot.dbName, spawn.CalculateRespawnDelay(slot.entry), now)
	}
	if !n.respawn.Tracked(slot.key) {
		n.dropSlotLocked(slot.key)
	}
}

// unlistGroupSlotLocked takes the slot of g off its spawn's list: dropped,
// or kept as an orphan, its armed respawn moved along, when a respawn is
// armed for it. The caller holds n.mu.
func (n *Npcs) unlistGroupSlotLocked(g *makerGroup, slot slotInfo) {
	current, ok := n.slot[slot.key]
	if !ok || current.serial != slot.serial {
		return
	}
	n.dropSlotLocked(slot.key)
	if deadline, armed := n.respawn.Deadline(slot.key); armed {
		n.respawn.Cancel(slot.key)
		if key, ok := n.adoptOrphanLocked(current); ok {
			n.respawn.Add(key, deadline)
		}
	}
	keys := g.keys[slot.spawnIdx]
	if i := slices.Index(keys, slot.key); i >= 0 {
		g.keys[slot.spawnIdx] = slices.Delete(keys, i, i+1)
	}
}

// Remove takes h out of the world at once, with no corpse and no decay
// wait, and arms its spawn slot's respawn exactly as a decayed corpse does.
// It runs on the calling goroutine (npc.Hostile.DeleteNow), so it may race
// the corpse decay on h's own queue: the slot is claimed only by the call
// that wins h's decay.
func (n *Npcs) Remove(h *npc.Hostile) {
	if n.decay != nil {
		n.decay.Cancel(h)
	}
	h.DecayWithRespawn(n.state, n.RespawnHook)
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
		n.spawns.SetRespawn(slot.dbName, delay, now)
	}
	n.respawn.Add(slot.key, now.Add(delay))
}

// Respawn implements task.RespawnEffects: it re-instantiates the slot key
// identifies, picking a fresh position (and, for a database-tracked slot,
// resuming through the same persisted-state restore rule used at boot).
// A slot DespawnAll dropped is not respawned. The NPC is built from the
// template the slot was declared with, not the one the table holds now:
// A spawn keeps its template for life, so //reload npc does not reach the
// respawns of spawns already in place.
func (n *Npcs) Respawn(key string) {
	n.respawnSlot(key)
}

// respawnSlot brings back the NPC of the slot key, a database-tracked one
// with its saved respawn time cleared first. A slot whose NPC is in the
// world, or is being placed, is left alone.
func (n *Npcs) respawnSlot(key string) {
	n.mu.Lock()
	slot, ok := n.slot[key]
	if !ok || slot.spawning || n.liveLocked(key) {
		n.mu.Unlock()
		return
	}
	slot.spawning = true
	n.slot[key] = slot
	n.mu.Unlock()
	defer n.endSpawn(key)

	tmpl := slot.tmpl
	if tmpl == nil {
		return
	}
	if slot.master != nil {
		// A private comes back only while its master lives, in the
		// master's current life: around it, or at the private's own point.
		master := npc.Current(slot.master)
		if master.Dead() {
			n.mu.Lock()
			n.dropSlotLocked(key)
			n.mu.Unlock()
			return
		}
		at, heading := n.privateSpawnLocation(master, tmpl), master.Heading()
		if slot.placed {
			at, heading = slot.at, slot.heading
		}
		n.instantiate(key, slot.entry, tmpl, at, heading, fullHP, fullMP, master)
		return
	}

	if slot.dbName != "" {
		rows := n.currentSpawns()
		rows.CancelRespawn(slot.dbName)
		n.spawnPersisted(key, slot.maker, slot.entry, tmpl, rows, slot.dbName, slot.memory.row)
		return
	}
	pos, ok := n.pickSpawnPosition(slot.maker, slot.entry)
	if !ok {
		n.deferredCount.Add(1)
		return
	}
	n.spawnFresh(key, slot.entry, tmpl, pos)
}

// despawnMinions takes master's minions out of the world with it, each
// decayed on the calling goroutine without waiting for its own queue (the
// npc package's off-queue delete contract), and drops their spawn slots,
// those of the privates spawned for master that are not in the world, and
// those of the gone privates spawned for one of them. A live private spawned
// for any of them sits in another party's set and keeps its spawn. An NPC that is no master takes nothing: a linked
// private's party privates are its master's.
func (n *Npcs) despawnMinions(master npc.PartyMember) {
	if !master.IsMaster() {
		return
	}
	minions := master.Minions()
	keys := make(map[string]bool, len(minions))
	for _, minion := range minions {
		id := minion.ObjectID()
		n.walker.StopRouteByID(id)
		switch o := minion.(type) {
		case *npc.Hostile:
			n.ai.Remove(o)
			o.Decay(n.state, nil)
		case *npc.Folk:
			n.ai.Remove(o)
			o.Decay(n.state, nil)
		}
		minion.SetMaster(nil)
		master.RemoveMinion(minion)

		n.mu.Lock()
		if key, ok := n.live[id]; ok {
			keys[key] = true
			delete(n.live, id)
			n.liveCount--
		}
		n.mu.Unlock()
	}
	if len(minions) == 0 {
		return
	}
	n.dropPrivateSlots(master, minions, keys)
}

// dropPrivateSlots drops, as master's minions are taken with it, the slots
// in keys (the taken minions' own), those of the privates spawned for master
// that are not in the world, and those of the gone privates spawned for one
// of minions.
func (n *Npcs) dropPrivateSlots(master npc.PartyMember, minions []npc.PartyMember, keys map[string]bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for key := range keys {
		n.dropSlotLocked(key)
	}
	for key := range n.privates[master.Scratch()] {
		// A live one was not taken: it sits in another party's set
		// (master's own master's, joined before a death let master go)
		// and stays in the world with its spawn.
		if !n.liveLocked(key) {
			n.dropSlotLocked(key)
		}
	}
	// A taken member's private outside master's set (one it created after
	// master's death let it go) is in the world on its own: only a gone
	// one's slot goes.
	for _, m := range minions {
		for key := range n.privates[m.Scratch()] {
			if !n.slot[key].spawning && !n.liveLocked(key) && !n.respawn.Tracked(key) {
				n.dropSlotLocked(key)
			}
		}
	}
}

// addSlotLocked records slot under its key, a private's indexed under its
// master. The caller holds n.mu.
func (n *Npcs) addSlotLocked(slot slotInfo) {
	n.slot[slot.key] = slot
	if slot.master == nil {
		return
	}
	scratch := slot.master.Scratch()
	keys := n.privates[scratch]
	if keys == nil {
		keys = make(map[string]struct{})
		n.privates[scratch] = keys
	}
	keys[slot.key] = struct{}{}
}

// dropSlotLocked drops the slot key, and a private's from its master's
// index. The caller holds n.mu.
func (n *Npcs) dropSlotLocked(key string) {
	slot, ok := n.slot[key]
	if !ok {
		return
	}
	delete(n.slot, key)
	if slot.master == nil {
		return
	}
	scratch := slot.master.Scratch()
	keys := n.privates[scratch]
	delete(keys, key)
	if len(keys) == 0 {
		delete(n.privates, scratch)
	}
}

// SyncPersistedState writes every live database-tracked slot's current HP,
// MP, and position back into its spawn row, ready for Spawns.Save. Dead rows
// (mid respawn countdown) are left untouched (Spawns.SetStats).
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
		if !ok || slot.dbName == "" || slot.orphan {
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
		x, y, z := hostile.Position()
		n.currentSpawns().SetStats(slot.dbName, hostile.CurrentHP(), hostile.CurrentMP(), location.Location{X: x, Y: y, Z: z}, hostile.Heading())
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
