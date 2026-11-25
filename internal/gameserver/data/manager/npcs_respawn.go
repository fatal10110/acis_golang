package manager

import (
	"slices"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/task"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

func (n *Npcs) RespawnHook(actorID int32) func() {
	// The look the NPC leaves in is the one it comes back in.
	var (
		form *npc.Template
		left leftNPC
	)
	if obj, ok := n.state.Object(actorID); ok {
		if m, ok := obj.(interface{ Polymorph() *npc.Template }); ok {
			form = m.Polymorph()
		}
		switch a := obj.(type) {
		case *npc.Hostile:
			left.name = a.Instance.Name()
		case *npc.Folk:
			left.name = a.Instance.Name()
		}
	}
	left.index, left.reverse, _ = n.walker.Walking(actorID)
	n.mu.Lock()
	key, tracked := n.live[actorID]
	slot, ok := n.slot[key]
	// The NPC of an npcmaker's slot counts alive until its group takes
	// its leave, in turn (see leaveGroup).
	grouped := tracked && ok && slot.group != nil
	if tracked && !grouped {
		delete(n.live, actorID)
		n.liveCount--
		if ok {
			slot.form, slot.left, slot.leftID = form, left, actorID
			n.slot[key] = slot
		}
	}
	n.mu.Unlock()
	if !tracked || !ok {
		n.walker.StopRouteByID(actorID)
		return nil
	}

	if obj, ok := n.state.Object(actorID); ok {
		if m, ok := obj.(npc.PartyMember); ok {
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

	if grouped {
		leave := groupLeave{id: actorID, key: key, form: form, left: left}
		return func() { n.leaveGroup(slot.group, leave) }
	}

	if slot.scripted {
		// The script that created the private brings it back, if ever.
		n.dropOrphanScriptSlot(key, slot)
		return nil
	}
	delay := spawn.CalculateRespawnDelay(*slot.entry)
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
// spawn that never respawns drops its slot. It runs in its group's turn
// (see leaveGroup), slot as the NPC left it.
//
// An NPC that leaves during its slot's first placement, before the maker
// counts it as one of its spawn's NPCs, is not reported to the maker, and
// its slot stays, decayed, whatever the respawn delay. A spawn that never
// respawns drops the slot of an NPC its maker armed a respawn for anyway
// from its list: the slot lives on as an orphan.
func (n *Npcs) groupNPCDeleted(g *makerGroup, slot slotInfo) {
	if slot.orphan {
		n.orphanNPCDeleted(slot)
		return
	}
	if !slot.first {
		scope := &hookScope{deleted: g}
		g.behavior.NPCDeleted(g.in(scope), groupSpawn{g: g, i: slot.spawnIdx, scope: scope}, groupNPC{n: n, slot: slot, scope: scope})
		n.mu.Lock()
		scope.deleted = nil
		n.mu.Unlock()
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
	n.spawns.SetRespawn(slot.dbName, spawn.CalculateRespawnDelay(*slot.entry), now)
}

// orphanNPCDeleted answers the NPC of an orphan slot leaving the world:
// no maker hook runs, it no longer counts alive (its group took it off as
// it took its leave), a database-tracked row records a respawn time when
// the spawn respawns at all, and the slot goes unless a respawn is armed
// for it.
func (n *Npcs) orphanNPCDeleted(slot slotInfo) {
	now := n.now()
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, ok := n.slot[slot.key]; !ok {
		return
	}
	if slot.dbName != "" && slot.entry.RespawnDelay > 0 {
		n.spawns.SetRespawn(slot.dbName, spawn.CalculateRespawnDelay(*slot.entry), now)
	}
	if !n.respawn.Tracked(slot.key) {
		n.dropSlotLocked(slot.key)
	}
}

// groupLeave is an NPC of an npcmaker's slot that left the world, waiting
// for its group to take its leave: the slot key it held and the look and
// walk it left in.
type groupLeave struct {
	id   int32
	key  task.SlotID
	form *npc.Template
	left leftNPC
}

// leaveGroup has g take the leave of the NPC that left, after every NPC of
// g that left before it, one at a time: an NPC is out of g's alive count,
// and its slot's maker hook and bookkeeping run, only once those of the
// earlier ones are done, the later ones still counted alive, as decays
// that run one after the other leave them.
func (n *Npcs) leaveGroup(g *makerGroup, leave groupLeave) {
	n.mu.Lock()
	g.leaving = append(g.leaving, leave)
	n.mu.Unlock()
	n.drainLeaves(g)
}

// drainLeaves takes the leaves g has waiting, in order, unless another
// goroutine is taking them: that one takes the new ones too. No caller
// waits on another, so a hook that deletes an NPC of its own group never
// waits on itself; an NPC-deleted hook runs the hooks of the NPCs it
// deletes itself nested in it (see leaveNowIn).
func (n *Npcs) drainLeaves(g *makerGroup) {
	n.mu.Lock()
	if g.draining {
		n.mu.Unlock()
		return
	}
	g.draining = true
	done := false
	defer func() {
		// A hook that panicked leaves the rest to the next leave.
		if !done {
			n.mu.Lock()
			g.draining = false
			n.mu.Unlock()
		}
	}()
	for len(g.leaving) > 0 {
		next := g.leaving[0]
		g.leaving = slices.Delete(g.leaving, 0, 1)
		slot, ok := n.takeLeaveLocked(g, next)
		n.mu.Unlock()
		if ok {
			n.groupNPCDeleted(g, slot)
		}
		n.mu.Lock()
	}
	g.draining = false
	done = true
	n.mu.Unlock()
}

// leaveNowIn takes the leave of g's NPC id, which a hook running in scope
// just deleted, at once when that hook is g's NPC-deleted hook: that hook
// runs in g's turn, so the NPC's own hook runs nested in it, before it
// carries on. Any other caller has g take its leaves in turn.
func (n *Npcs) leaveNowIn(g *makerGroup, id int32, scope *hookScope) {
	n.mu.Lock()
	i := slices.IndexFunc(g.leaving, func(l groupLeave) bool { return l.id == id })
	if i < 0 || scope == nil || scope.deleted != g {
		n.mu.Unlock()
		n.drainLeaves(g)
		return
	}
	leave := g.leaving[i]
	g.leaving = slices.Delete(g.leaving, i, i+1)
	slot, ok := n.takeLeaveLocked(g, leave)
	n.mu.Unlock()
	if ok {
		n.groupNPCDeleted(g, slot)
	}
}

// takeLeaveLocked takes leave's NPC out of g's alive count, recording on
// its slot the look and walk it left in, and returns that slot; none when
// its slot no longer counts it alive (a DeleteAll, DeleteNPCs or
// DespawnAll took it since, its maker hook never to run). The caller holds
// n.mu.
func (n *Npcs) takeLeaveLocked(g *makerGroup, leave groupLeave) (slotInfo, bool) {
	if key, ok := n.live[leave.id]; !ok || key != leave.key {
		return slotInfo{}, false
	}
	delete(n.live, leave.id)
	n.liveCount--
	slot, ok := n.slot[leave.key]
	if !ok {
		return slotInfo{}, false
	}
	slot.form, slot.left, slot.leftID = leave.form, leave.left, leave.id
	n.slot[leave.key] = slot
	if slot.orphan {
		g.orphansAlive--
	}
	return slot, true
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
func (n *Npcs) Respawn(key task.SlotID) {
	n.respawnSlot(key)
}

// respawnSlot brings back the NPC of the slot key, a database-tracked one
// with its saved respawn time cleared first. A slot whose NPC is in the
// world, or is being placed, is left alone.
func (n *Npcs) respawnSlot(key task.SlotID) {
	n.mu.Lock()
	slot, ok := n.slot[key]
	if !ok || slot.spawning || n.liveLocked(key) {
		n.mu.Unlock()
		return
	}
	if slot.group != nil && slot.group.retired {
		// DespawnAll or RespawnAll retired its npcmaker since.
		n.dropSlotLocked(key)
		n.mu.Unlock()
		return
	}
	slot.spawning = true
	leftID := slot.leftID
	slot.leftID = 0
	n.slot[key] = slot
	n.mu.Unlock()
	defer n.endSpawn(key)
	// The NPC that left the slot gives its object id back as the slot
	// respawns, whether or not the respawn places one.
	n.releaseID(leftID)

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
		n.instantiate(key, *slot.entry, tmpl, at, heading, fullHP, fullMP, master)
		return
	}

	if slot.dbName != "" {
		rows := n.currentSpawns()
		rows.CancelRespawn(slot.dbName)
		n.spawnPersisted(key, slot.maker, *slot.entry, tmpl, rows, slot.dbName, slot.memory.row)
		return
	}
	pos, ok := n.pickSpawnPosition(slot.maker, *slot.entry)
	if !ok {
		n.deferredCount.Add(1)
		return
	}
	n.spawnFresh(key, *slot.entry, tmpl, pos)
}

// releaseID gives the object id id back to the id allocator; nothing for
// id 0 or an allocator that takes none back.
func (n *Npcs) releaseID(id int32) {
	if id == 0 {
		return
	}
	// accepted: releasing ids is an optional allocator capability.
	if r, ok := n.ids.(interface{ ReleaseID(int32) }); ok {
		r.ReleaseID(id)
	}
}

// despawnMinions takes master's minions out of the world as its spawn is
// deleted (a fixed spawn's delete, a maker's), each decayed on the calling
// goroutine without waiting for its own queue (the npc package's off-queue
// delete contract), and drops their spawn slots, those of the privates
// spawned for master that are not in the world, and those of the gone
// privates spawned for one of them. A live private spawned for any of them
// sits in another party's set and keeps its spawn. An NPC that is no master
// takes nothing: a linked private's party privates are its master's. A
// master's decay alone takes none of them.
func (n *Npcs) despawnMinions(master npc.PartyMember) {
	if !master.IsMaster() {
		return
	}
	minions := master.Minions()
	keys := make(map[task.SlotID]bool, len(minions))
	for _, minion := range minions {
		id := minion.ObjectID()
		n.walker.StopRouteByID(id)
		// A private a script is still placing is not in the world yet, and
		// a decay now would not keep it out: once placed, it is taken by
		// its creator, which finds master gone (#5156).
		_, placed := n.state.Object(id)
		switch o := minion.(type) {
		case *npc.Hostile:
			n.ai.Remove(o)
			if placed {
				o.Decay(n.state, nil)
			}
		case *npc.Folk:
			n.ai.Remove(o)
			if placed {
				o.Decay(n.state, nil)
			}
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
func (n *Npcs) dropPrivateSlots(master npc.PartyMember, minions []npc.PartyMember, keys map[task.SlotID]bool) {
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
		keys = make(map[task.SlotID]struct{})
		n.privates[scratch] = keys
	}
	keys[slot.key] = struct{}{}
}

// dropSlotLocked drops the slot key, and a private's from its master's
// index. The caller holds n.mu.
func (n *Npcs) dropSlotLocked(key task.SlotID) {
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
	live := make(map[int32]task.SlotID, len(n.live))
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
