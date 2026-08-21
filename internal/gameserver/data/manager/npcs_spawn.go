package manager

import (
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func isOnStartMaker(maker *spawn.Maker) bool {
	if maker.Event != "" {
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

func (n *Npcs) registerSlot(key string, maker *spawn.Maker, entry spawn.Entry, dbName string) {
	n.mu.Lock()
	n.slot[key] = slotInfo{key: key, maker: maker, entry: entry, dbName: dbName}
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

	loc, heading, hp := pos.Location, pos.Heading, fullHP
	if state.CheckAlive(pos.Location, pos.Heading, int(tmpl.HPMax), 0, now) {
		loc, heading, hp = state.Location, state.Heading, state.CurrentHP
	}
	n.instantiate(key, entry, tmpl, loc, heading, hp)
}

// fullHP tells instantiate to seed the spawned Hostile at its own calculated
// Max HP rather than a persisted CurrentHP value.
const fullHP = -1

// spawnFresh places one non-persisted instance of entry at a freshly rolled
// position, always alive at full HP — the reference server never restores
// HP/position across restarts for a spawn without a database name.
func (n *Npcs) spawnFresh(key string, entry spawn.Entry, tmpl *npc.Template, pos spawn.Position) {
	n.instantiate(key, entry, tmpl, pos.Location, pos.Heading, fullHP)
}

// instantiate builds one live Hostile from tmpl and places it in the world
// at (loc, heading) with hp current HP (or fullHP, its calculated Max HP),
// registering it for AI ticks and corpse decay/respawn.
func (n *Npcs) instantiate(key string, entry spawn.Entry, tmpl *npc.Template, loc location.Location, heading, hp int) {
	id, err := n.ids.NextID()
	if err != nil {
		n.log.Warn().Err(err).Int32("npc_id", entry.NPCID).Msg("spawn: id space exhausted")
		return
	}

	inst, err := npc.NewInstance(id, tmpl)
	if err != nil {
		n.log.Warn().Err(err).Int32("npc_id", entry.NPCID).Msg("spawn: cannot build npc instance")
		return
	}
	inst.Home = loc
	inst.HasHome = true

	if !npc.Attackable(inst) {
		n.skippedNonCombatCount.Add(1)
		return
	}

	hostile, err := newLiveHostile(inst, tmpl.RunSpeed, n.geo, n.positions, n.log)
	if err != nil {
		n.log.Warn().Err(err).Int32("npc_id", entry.NPCID).Msg("spawn: cannot build live npc")
		return
	}

	if hp == fullHP {
		hp = hostile.MaxHP()
	}
	hostile.SetCurrentHP(hp)
	hostile.SetWorld(n.state)
	hostile.SetFrameBuilder(serverpackets.NpcFrameBuilder{})
	hostile.SetWeapon(n.items)
	hostile.SetRewarder(n.rewarderFor(hostile, tmpl))

	n.state.Spawn(hostile, loc.X, loc.Y, loc.Z, heading)
	n.ai.Add(hostile)

	n.mu.Lock()
	n.live[id] = key
	n.liveCount++
	n.mu.Unlock()
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
	}
}

// RespawnHook implements the decay task's per-actor respawn resolution: it
// unregisters actorID from AI ticks and live tracking, and — when its slot
// has a positive respawn delay — returns the closure that arms the next
// respawn. It reports nil when actorID isn't a tracked spawn slot, or when
// the slot's entry has no respawn delay (a permanent, one-shot spawn).
