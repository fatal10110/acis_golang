package manager

import (
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// fullMP tells instantiate to seed the spawned Hostile at its own calculated
// Max MP rather than a persisted CurrentMP value.
const fullMP = -1

// registerSlot declares the spawn slot key of entry i of g, backed by the
// persisted row dbName when non-empty, with its first spawn under way: the
// caller ends it with endSpawn.
func (n *Npcs) registerSlot(key string, g *makerGroup, i int, dbName string, tmpl *npc.Template) {
	entry := g.def.Entries[i]
	n.mu.Lock()
	n.slot[key] = slotInfo{key: key, maker: g.def, group: g, spawnIdx: i, entry: entry, dbName: dbName, tmpl: tmpl, memory: newSlotMemory(entry), spawning: true}
	g.keys[i] = append(g.keys[i], key)
	n.mu.Unlock()
}

func (n *Npcs) registerPrivateSlot(key string, entry spawn.Entry, masterID int32, tmpl *npc.Template) {
	n.mu.Lock()
	n.slot[key] = slotInfo{key: key, entry: entry, masterID: masterID, tmpl: tmpl, memory: newSlotMemory(entry)}
	n.mu.Unlock()
}

// spawnPersistedSlot declares the database-tracked slot key of entry i of
// g and restores or freshly spawns its NPC. A spawn still dead with a
// pending respawn deadline is not instantiated: only its respawn timer is
// armed, matching the persisted-state restore rule.
func (n *Npcs) spawnPersistedSlot(g *makerGroup, i int, key string, tmpl *npc.Template) {
	entry := g.def.Entries[i]
	dbName := entry.DBName
	n.registerSlot(key, g, i, dbName, tmpl)

	state, ok := n.currentSpawns().State(dbName)
	if !ok {
		state = spawn.NewState(dbName)
	}

	now := n.now()
	if state.Dead(now) {
		remaining := time.UnixMilli(state.RespawnTime).Sub(now)
		if remaining < 0 {
			remaining = 0
		}
		n.respawn.Add(key, now.Add(remaining))
		n.restoredDeadCount.Add(1)
		return
	}

	n.spawnPersisted(key, g.def, entry, tmpl, state)
}

// spawnPersisted places one instance of a database-tracked entry, reusing
// persisted HP and position when the row was still alive, or a freshly
// rolled position at full HP otherwise (CheckAlive's own restore rule).
func (n *Npcs) spawnPersisted(key string, maker *spawn.Maker, entry spawn.Entry, tmpl *npc.Template, state *spawn.State) {
	now := n.now()
	pos, ok := n.pickSpawnPosition(maker, entry)
	if !ok {
		n.deferredCount.Add(1)
		return
	}

	loc, heading, hp, mp := pos.Location, pos.Heading, fullHP, fullMP
	if state.CheckAlive(pos.Location, pos.Heading, int(tmpl.HPMax), int(tmpl.MPMax), now) {
		loc, heading, hp, mp = state.Location, state.Heading, state.CurrentHP, state.CurrentMP
	}
	master := n.instantiate(key, entry, tmpl, loc, heading, hp, mp, nil)
	if master != nil {
		n.spawnPrivates(key, entry, master)
	}
	n.created(key)
}

// fullHP tells instantiate to seed the spawned Hostile at its own calculated
// Max HP rather than a persisted CurrentHP value.
const fullHP = -1

// spawnFresh places one non-persisted instance of entry at a freshly rolled
// position, always alive at full HP/MP — HP/MP/position are never restored
// across restarts for a spawn without a database name.
func (n *Npcs) spawnFresh(key string, entry spawn.Entry, tmpl *npc.Template, pos spawn.Position) {
	master := n.instantiate(key, entry, tmpl, pos.Location, pos.Heading, fullHP, fullMP, nil)
	if master != nil {
		n.spawnPrivates(key, entry, master)
	}
	n.created(key)
}

// instantiate builds one live Hostile from tmpl and places it in the world
// at (loc, heading) with hp current HP and mp current MP (or fullHP/fullMP,
// its calculated Max HP/MP), registering it for AI ticks and corpse
// decay/respawn.
func (n *Npcs) instantiate(key string, entry spawn.Entry, tmpl *npc.Template, loc location.Location, heading, hp, mp int, master *npc.Hostile) *npc.Hostile {
	id, err := n.ids.NextID()
	if err != nil {
		n.log.Warn().Err(err).Int32("npc_id", entry.NPCID).Msg("spawn: id space exhausted")
		return nil
	}

	inst, err := npc.NewInstance(id, tmpl)
	if err != nil {
		n.log.Warn().Err(err).Int32("npc_id", entry.NPCID).Msg("spawn: cannot build npc instance")
		return nil
	}
	inst.Home = loc
	inst.HasHome = true
	inst.SpawnHeading = heading
	inst.WalkMode = walkerWalkModeIDs[entry.NPCID]
	n.mu.Lock()
	info := n.slot[key]
	n.mu.Unlock()
	inst.Maker = info.maker
	// Each life of the slot starts with the script value cleared; its
	// other script memory carries over.
	var slot npc.SpawnSlot
	if info.memory != nil {
		info.memory.scratch.Respawned()
		slot = info.memory
	}

	if npc.FolkKind(inst) {
		n.spawnFolk(key, inst, loc, heading, slot)
		return nil
	}
	if !npc.Attackable(inst) {
		n.skippedNonCombatCount.Add(1)
		return nil
	}

	speed := tmpl.RunSpeed
	if inst.WalkMode {
		speed = tmpl.WalkSpeed
	}
	queue := n.queues.NewQueue(fmt.Sprintf("npc-%d", inst.ObjectID))
	hostile, walkerRef, err := newLiveHostile(inst, speed, n.geo, n.positions, n.log, n.castDefs, n.castEffects, n.walker, n.maxBuffsAmount, n.maxGeoPathFailCount, n.zones, n.effects, queue)
	if err != nil {
		n.log.Warn().Err(err).Int32("npc_id", entry.NPCID).Msg("spawn: cannot build live npc")
		return nil
	}
	hostile.AI().SetRandomWalkRate(n.randomWalkRate)
	hostile.SetRaidMultipliers(n.raidMultipliers)
	hostile.SetAIConfig(n.aiConfig)

	// MP goes first: a saved HP of zero leaves the NPC dead, and a dead
	// NPC's MP no longer changes.
	if mp == fullMP {
		mp = hostile.CurrentMP()
	}
	hostile.SetCurrentMP(mp)
	if hp == fullHP {
		hp = hostile.MaxHP()
	}
	hostile.SetCurrentHP(hp)
	rewards := n.rewarderFor(hostile, tmpl)
	rt := npc.Runtime{World: n.state, Log: n.log, Items: n.items, Rewards: rewards, Remover: n, Slot: slot, Scripts: n.scripts}
	if rewards.rights != nil {
		rt.Hits = rewards.rights
	}
	if los, ok := n.geo.(npc.LineOfSight); ok {
		rt.LOS = los
	}
	if n.newSink != nil {
		rt.Sink = n.newSink(hostile)
	}
	hostile.Attach(rt)
	if master != nil {
		hostile.SetMaster(master)
		master.AddMinion(hostile)
		// MinionSpawn.doSpawn: a Monster-family private of a raid boss
		// joins the raid (no lethal strikes, raid curse, see-through).
		if master.RaidBoss() && hostile.MonsterKind() {
			hostile.SetRaidRelated(true)
		}
	}

	n.state.Spawn(hostile, loc.X, loc.Y, loc.Z, heading)
	hostile.EnterZones()
	n.ai.Add(hostile)
	// Walker only ticks in-region actors — must run after Spawn placed this
	// NPC in world.State, not before.
	startWalkerRoute(n.walker, walkerRef, inst, n.log)

	if !n.trackLive(key, id) {
		n.deleteNpc(hostile)
		return nil
	}
	return hostile
}

// spawnFolk places a civilian NPC built from inst in the world at (loc,
// heading) and tracks it under its spawn slot key, like a hostile: a mortal
// one that dies decays and respawns through the slot. A route walker walks
// its route while it lives.
func (n *Npcs) spawnFolk(key string, inst *npc.Instance, loc location.Location, heading int, slot npc.SpawnSlot) {
	f, err := n.folk.Spawn(inst, loc, heading, slot)
	if err != nil {
		n.log.Warn().Err(err).Int("npc_id", inst.Template.ID).Msg("spawn: cannot build folk npc")
		return
	}
	n.folkCount.Add(1)
	if !n.trackLive(key, inst.ObjectID) {
		n.deleteNpc(f)
	}
}

// endSpawn marks the spawn of the slot key done.
func (n *Npcs) endSpawn(key string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if slot, ok := n.slot[key]; ok {
		slot.spawning = false
		n.slot[key] = slot
	}
}

// trackLive records id as the live NPC of the slot key. It reports false,
// recording nothing, when the slot was dropped while the NPC was being
// placed (its group deleted meanwhile); the caller then deletes the NPC.
func (n *Npcs) trackLive(key string, id int32) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	slot, ok := n.slot[key]
	if !ok {
		return false
	}
	n.live[id] = key
	n.liveCount++
	slot.liveID = id
	n.slot[key] = slot
	return true
}

func (n *Npcs) spawnPrivates(key string, entry spawn.Entry, master *npc.Hostile) {
	// The generic monster behavior creates spawn-list privates only for Party_Type 2.
	if master.AIInt("Party_Type", 0) != 2 {
		return
	}
	for i, private := range entry.Privates {
		tmpl, ok := n.templates.Get(int(private.NPCID))
		if !ok {
			n.log.Warn().Int32("npc_id", private.NPCID).Msg("spawn: private template missing")
			continue
		}
		privateEntry := spawn.Entry{NPCID: private.NPCID, RespawnDelay: private.RespawnDelay}
		privateKey := fmt.Sprintf("%s/private/%d", key, i)
		n.registerPrivateSlot(privateKey, privateEntry, master.ObjectID(), tmpl)
		n.instantiate(privateKey, privateEntry, tmpl, n.privateSpawnLocation(master, tmpl), master.Heading(), fullHP, fullMP, master)
	}
}

func (n *Npcs) privateSpawnLocation(master *npc.Hostile, tmpl *npc.Template) location.Location {
	x, y, z := master.Position()
	minOffset := int(master.Instance.Template.CollisionRadius + 30)
	maxOffset := int(100 + master.Instance.Template.CollisionRadius + tmpl.CollisionRadius)
	dest := location.Location{X: x, Y: y, Z: z}.AddRandomOffsetBetween(minOffset, maxOffset)
	return n.geo.ValidLocation(x, y, z, dest.X, dest.Y, dest.Z)
}

// rewarderFor returns the kill-reward hook for a newly spawned hostile.
func (n *Npcs) rewarderFor(hostile *npc.Hostile, tmpl *npc.Template) *deathRewards {
	return &deathRewards{
		hostile:    hostile,
		state:      n.state,
		tmpl:       tmpl,
		categories: tmpl.Drops,
		config:     n.rewards,
		raid:       hostile.RaidBoss(),
		decay:      n.decay,
		ids:        n.ids,
		items:      n.items,
		ground:     n.ground,
		geo:        n.geo,
		rights:     raidLootRights(hostile, tmpl, n.rewards.Channels),
	}
}

// raidLootRights returns the command channel loot rights of a raid or
// grand boss, or nil for any other NPC or without channels.
func raidLootRights(hostile *npc.Hostile, tmpl *npc.Template, channels LootChannels) *ccLootRights {
	if channels == nil || !hostile.RaidBoss() {
		return nil
	}
	return newCCLootRights(hostile, tmpl.ID, channels)
}

// NewHostileRewarder builds the production kill-reward hook for a hostile
// spawned outside the spawn table — the behavior-test boot uses it so kills
// pay real experience/SP through the same death chain. Drop categories come
// from the template, placed through ids and ground, and corpse-decay
// scheduling from its CorpseTime, so a caller whose templates declare no
// CorpseTime never reaches the decay hook this simplified signature leaves
// out. The hit observer, nil for anything but a raid or grand boss with
// channels configured, watches the hits that win a command channel the
// boss's loot rights.
func NewHostileRewarder(hostile *npc.Hostile, tmpl *npc.Template, state *world.State, config KillRewardConfig, items *item.Table, ids idAllocator, ground groundPlacer) (creature.Rewarder, npc.HitObserver) {
	rights := raidLootRights(hostile, tmpl, config.Channels)
	rewards := &deathRewards{
		hostile:    hostile,
		state:      state,
		tmpl:       tmpl,
		categories: tmpl.Drops,
		config:     config,
		raid:       hostile.RaidBoss(),
		items:      items,
		ids:        ids,
		ground:     ground,
		rights:     rights,
	}
	if rights == nil {
		return rewards, nil
	}
	return rewards, rights
}

// RespawnHook implements the decay task's per-actor respawn resolution: it
// unregisters actorID from AI ticks and live tracking, and — when its slot
// has a positive respawn delay — returns the closure that arms the next
// respawn. It reports nil when actorID isn't a tracked spawn slot, or when
// the slot's entry has no respawn delay (a permanent, one-shot spawn), or
// when its maker's spawn event is not listed.
