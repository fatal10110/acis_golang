package manager

import (
	"fmt"
	"math"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func isOnStartMaker(maker *spawn.Maker) bool {
	if maker.Event != "" {
		return false
	}
	if maker.SpawnTime != "" {
		return false
	}
	if v, ok := maker.AIParams["on_start_spawn"]; ok && v == "0" {
		return false
	}
	return true
}

// bootSpawnEntry instantiates one maker entry's slots at boot: a single
// persisted slot for a database-tracked entry, or up to entry.Total fresh
// slots otherwise. remaining is the maker's shared spawn budget, decremented
// per instance placed and left untouched for a skipped/deferred entry.
func (n *Npcs) bootSpawnEntry(maker *spawn.Maker, entryIndex int, entry spawn.Entry, remaining *int) {
	tmpl, ok := n.templates.Get(int(entry.NPCID))
	if !ok {
		n.log.Warn().Int32("npc_id", entry.NPCID).Str("maker", maker.Name).Msg("spawn entry references unknown npc template")
		return
	}

	if entry.DBName != "" {
		if *remaining <= 0 {
			return
		}
		*remaining--
		n.bootSpawnPersisted(maker, entry.DBName, entry, tmpl)
		return
	}

	for i := 0; i < entry.Total; i++ {
		if *remaining <= 0 {
			return
		}
		pos, ok := n.pickSpawnPosition(maker, entry)
		if !ok {
			n.deferredCount.Add(1)
			return
		}
		*remaining--
		key := fmt.Sprintf("%s#%d#%d", maker.Name, entryIndex, i)
		n.registerSlot(key, maker, entry, "")
		n.spawnFresh(key, entry, tmpl, pos)
	}
}

// fullMP tells instantiate to seed the spawned Hostile at its own calculated
// Max MP rather than a persisted CurrentMP value.
const fullMP = -1

func (n *Npcs) registerSlot(key string, maker *spawn.Maker, entry spawn.Entry, dbName string) {
	n.mu.Lock()
	n.slot[key] = slotInfo{key: key, maker: maker, entry: entry, dbName: dbName}
	n.mu.Unlock()
}

func (n *Npcs) registerPrivateSlot(key string, entry spawn.Entry, masterID int32) {
	n.mu.Lock()
	n.slot[key] = slotInfo{key: key, entry: entry, masterID: masterID}
	n.mu.Unlock()
}

// bootSpawnPersisted restores or freshly spawns a database-tracked entry's
// single slot at boot. A spawn still dead with a pending respawn deadline
// is not instantiated: only its respawn timer is (re)armed, matching the
// persisted-state restore rule.
func (n *Npcs) bootSpawnPersisted(maker *spawn.Maker, dbName string, entry spawn.Entry, tmpl *npc.Template) {
	n.registerSlot(dbName, maker, entry, dbName)

	state, ok := n.spawns.State(dbName)
	if !ok {
		state = spawn.NewState(dbName)
	}

	now := n.now()
	if state.Dead(now) {
		remaining := time.UnixMilli(state.RespawnTime).Sub(now)
		if remaining < 0 {
			remaining = 0
		}
		n.respawn.Add(dbName, now.Add(remaining))
		n.restoredDeadCount.Add(1)
		return
	}

	n.spawnPersisted(dbName, maker, entry, tmpl, state)
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
}

// fullHP tells instantiate to seed the spawned Hostile at its own calculated
// Max HP rather than a persisted CurrentHP value.
const fullHP = -1

// spawnFresh places one non-persisted instance of entry at a freshly rolled
// position, always alive at full HP/MP — the reference server never restores
// HP/MP/position across restarts for a spawn without a database name.
func (n *Npcs) spawnFresh(key string, entry spawn.Entry, tmpl *npc.Template, pos spawn.Position) {
	master := n.instantiate(key, entry, tmpl, pos.Location, pos.Heading, fullHP, fullMP, nil)
	if master != nil {
		n.spawnPrivates(key, entry, master)
	}
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

	if !npc.Attackable(inst) {
		n.skippedNonCombatCount.Add(1)
		return nil
	}

	speed := tmpl.RunSpeed
	if inst.WalkMode {
		speed = tmpl.WalkSpeed
	}
	hostile, walkerRef, err := newLiveHostile(inst, speed, n.geo, n.positions, n.log, n.castDefs, n.castEffects, n.walker, n.maxBuffsAmount, n.zones)
	if err != nil {
		n.log.Warn().Err(err).Int32("npc_id", entry.NPCID).Msg("spawn: cannot build live npc")
		return nil
	}

	if hp == fullHP {
		hp = hostile.MaxHP()
	}
	hostile.SetCurrentHP(hp)
	if mp == fullMP {
		mp = hostile.CurrentMP()
	}
	hostile.SetCurrentMP(mp)
	hostile.SetWorld(n.state)
	hostile.SetFrameBuilder(serverpackets.NpcFrameBuilder{})
	hostile.SetWeapon(n.items)
	hostile.SetRewarder(n.rewarderFor(hostile, tmpl))
	if master != nil {
		hostile.SetMaster(master)
		master.AddMinion(hostile)
	}

	n.state.Spawn(hostile, loc.X, loc.Y, loc.Z, heading)
	n.ai.Add(hostile)
	// Walker only ticks in-region actors — must run after Spawn placed this
	// NPC in world.State, not before.
	startWalkerRoute(n.walker, walkerRef, inst, n.log)

	n.mu.Lock()
	n.live[id] = key
	n.liveCount++
	slot := n.slot[key]
	slot.liveID = id
	n.slot[key] = slot
	n.mu.Unlock()
	return hostile
}

func (n *Npcs) spawnPrivates(key string, entry spawn.Entry, master *npc.Hostile) {
	// Java's generic MonsterBehavior creates spawn-list privates only for Party_Type 2.
	partyType, err := master.Instance.Template.AIParams.GetIntDefault("Party_Type", 0)
	if err != nil || partyType != 2 {
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
		n.registerPrivateSlot(privateKey, privateEntry, master.ObjectID())
		n.instantiate(privateKey, privateEntry, tmpl, n.privateSpawnLocation(master, tmpl), master.Heading(), fullHP, fullMP, master)
	}
}

func (n *Npcs) privateSpawnLocation(master *npc.Hostile, tmpl *npc.Template) location.Location {
	x, y, z := master.Position()
	minOffset := int(master.Instance.Template.CollisionRadius + 30)
	maxOffset := int(100 + master.Instance.Template.CollisionRadius + tmpl.CollisionRadius)
	angle := float64(rnd.Get(360)) * math.Pi / 180
	offset := rnd.GetRange(minOffset, maxOffset)
	targetX := x + int(float64(offset)*math.Cos(angle))
	targetY := y + int(float64(offset)*math.Sin(angle))
	return n.geo.ValidLocation(x, y, z, targetX, targetY, z)
}

// rewarderFor returns the kill-reward hook for a newly spawned hostile.
func (n *Npcs) rewarderFor(hostile *npc.Hostile, tmpl *npc.Template) *deathRewards {
	return &deathRewards{
		hostile:    hostile,
		state:      n.state,
		tmpl:       tmpl,
		categories: tmpl.Drops,
		config:     n.rewards,
		raid:       tmpl.Type == "RaidBoss",
		decay:      n.decay,
		ids:        n.ids,
		items:      n.items,
		ground:     n.ground,
		geo:        n.geo,
	}
}

// NewHostileRewarder builds the production kill-reward hook for a hostile
// spawned outside the spawn table — the behavior-test boot uses it so kills
// pay real experience/SP through the same death chain. Drop categories come
// from the template and corpse-decay scheduling from its CorpseTime, so a
// caller whose templates declare neither never reaches the decay, drop-id,
// or ground-placement hooks this simplified signature leaves out.
func NewHostileRewarder(hostile *npc.Hostile, tmpl *npc.Template, state *world.State, config KillRewardConfig, items *item.Table) creature.Rewarder {
	return &deathRewards{
		hostile:    hostile,
		state:      state,
		tmpl:       tmpl,
		categories: tmpl.Drops,
		config:     config,
		raid:       tmpl.Type == "RaidBoss",
		items:      items,
	}
}

// RespawnHook implements the decay task's per-actor respawn resolution: it
// unregisters actorID from AI ticks and live tracking, and — when its slot
// has a positive respawn delay — returns the closure that arms the next
// respawn. It reports nil when actorID isn't a tracked spawn slot, or when
// the slot's entry has no respawn delay (a permanent, one-shot spawn).
