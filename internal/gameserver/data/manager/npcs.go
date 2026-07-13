package manager

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// slotInfo is the static definition of one spawn slot: the entry it was
// declared under, and (when non-empty) the persisted state row backing it.
// A slot with a non-empty dbName is the only kind restored across restarts
// and forced to a single live instance, matching the reference server's
// "a database-tracked spawn ignores its total and only ever has one
// instance" rule.
type slotInfo struct {
	key    string
	entry  spawn.Entry
	dbName string
}

// Npcs owns every live NPC instantiated from the spawn table at boot,
// indexed by object id, and drives their decay/respawn/AI lifecycle.
//
// Only spawn entries with an explicit "pos" attribute (fixed or
// chance-weighted) are placed. Entries with no declared positions rely on
// picking a random point inside their maker's territory polygon, which
// needs a triangulation/random-point-in-polygon routine and a geodata
// height query that don't exist in this codebase yet (tracked under the
// geometry/Territory epic) — those entries are counted and skipped rather
// than guessed at. Entry.Privates (child minion spawns declared under one
// spawn position) are likewise not instantiated here; the base spawn loop
// is already a large unit on its own, and minion fan-out has its own
// master/minion linking concerns.
//
// Every instantiated NPC becomes a npc.Hostile: an entry whose template
// resolves to a non-combat instance type (a shop, trainer, gatekeeper,
// village master, and similar service NPCs — confirmed against the full
// shipped spawn list, roughly a quarter of positioned entries) is counted
// and skipped rather than given some other live representation. Those NPCs
// need dialog/HTML/shop interaction, not combat, which is its own later
// system (the dialog pipeline epic) — this type only builds the
// combat-capable half of "every spawn entry becomes a live NPC".
//
// All exported methods are safe for concurrent use; mu guards slots/live.
type Npcs struct {
	templates *npc.Table
	geo       move.Geo
	state     *world.State
	ids       idAllocator
	decay     *task.Decay
	respawn   *task.Respawn
	ai        *task.AI
	items     *item.Table
	ground    groundPlacer
	rates     item.Rates
	spawns    *Spawns
	now       func() time.Time
	log       zerolog.Logger

	mu   sync.Mutex
	slot map[string]slotInfo
	live map[int32]string

	// liveCount is guarded by mu, not atomic: every update pairs it with a
	// live map write/delete that must stay consistent with the count.
	liveCount int

	// deferredCount/restoredDeadCount/skippedNonCombatCount are lone
	// increments with no other state to keep in sync, so they're atomic
	// rather than sharing mu.
	deferredCount         atomic.Int64
	restoredDeadCount     atomic.Int64
	skippedNonCombatCount atomic.Int64
}

// NewNpcs walks spawns' loaded table and instantiates every "on start"
// maker's qualifying entries into state, respecting persisted dead/alive
// data for database-tracked entries.
func NewNpcs(spawns *Spawns, templates *npc.Table, geo move.Geo, state *world.State, ids idAllocator, decay *task.Decay, respawnTask *task.Respawn, ai *task.AI, items *item.Table, ground groundPlacer, rates item.Rates, now func() time.Time, log zerolog.Logger) (*Npcs, error) {
	if spawns == nil || spawns.Table() == nil {
		return nil, fmt.Errorf("npcs: nil spawn table")
	}
	if templates == nil {
		return nil, fmt.Errorf("npcs: nil npc template table")
	}
	if geo == nil {
		return nil, fmt.Errorf("npcs: nil geo")
	}
	if state == nil {
		return nil, fmt.Errorf("npcs: nil world state")
	}
	if ids == nil {
		return nil, fmt.Errorf("npcs: nil id allocator")
	}
	if decay == nil {
		return nil, fmt.Errorf("npcs: nil decay task")
	}
	if respawnTask == nil {
		return nil, fmt.Errorf("npcs: nil respawn task")
	}
	if ai == nil {
		return nil, fmt.Errorf("npcs: nil ai task")
	}
	if items == nil {
		return nil, fmt.Errorf("npcs: nil item table")
	}
	if ground == nil {
		return nil, fmt.Errorf("npcs: nil ground placer")
	}
	if now == nil {
		now = time.Now
	}

	n := &Npcs{
		templates: templates,
		geo:       geo,
		state:     state,
		ids:       ids,
		decay:     decay,
		respawn:   respawnTask,
		ai:        ai,
		items:     items,
		ground:    ground,
		rates:     rates,
		spawns:    spawns,
		now:       now,
		log:       log,
		slot:      make(map[string]slotInfo),
		live:      make(map[int32]string),
	}

	for _, maker := range spawns.Table().Makers() {
		if !isOnStartMaker(maker) {
			continue
		}
		remaining := maker.MaximumNPCs
		for entryIndex, entry := range maker.Entries {
			n.bootSpawnEntry(maker.Name, entryIndex, entry, &remaining)
		}
	}

	return n, nil
}

// isOnStartMaker reports whether maker should be populated at boot: it has
// no event gate and its ai params don't disable the initial spawn. Makers
// with an "ai type" that scripts special spawn selection (random pick
// among candidates, exclusive slots, day/night toggles, etc.) are treated
// the same as the default "spawn every entry up to its total" behavior —
// no scripted maker framework exists in this codebase yet.
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
func (n *Npcs) bootSpawnEntry(makerName string, entryIndex int, entry spawn.Entry, remaining *int) {
	if len(entry.Positions) == 0 {
		n.deferredCount.Add(1)
		return
	}

	tmpl, ok := n.templates.Get(int(entry.NPCID))
	if !ok {
		n.log.Warn().Int32("npc_id", entry.NPCID).Str("maker", makerName).Msg("spawn entry references unknown npc template")
		return
	}

	if entry.DBName != "" {
		if *remaining <= 0 {
			return
		}
		*remaining--
		n.bootSpawnPersisted(entry.DBName, entry, tmpl)
		return
	}

	for i := 0; i < entry.Total; i++ {
		if *remaining <= 0 {
			return
		}
		*remaining--
		key := fmt.Sprintf("%s#%d#%d", makerName, entryIndex, i)
		n.registerSlot(key, entry, "")
		n.spawnFresh(key, entry, tmpl)
	}
}

func (n *Npcs) registerSlot(key string, entry spawn.Entry, dbName string) {
	n.mu.Lock()
	n.slot[key] = slotInfo{key: key, entry: entry, dbName: dbName}
	n.mu.Unlock()
}

// bootSpawnPersisted restores or freshly spawns a database-tracked entry's
// single slot at boot. A spawn still dead with a pending respawn deadline
// is not instantiated: only its respawn timer is (re)armed, matching the
// persisted-state restore rule.
func (n *Npcs) bootSpawnPersisted(dbName string, entry spawn.Entry, tmpl *npc.Template) {
	n.registerSlot(dbName, entry, dbName)

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

	n.spawnPersisted(dbName, entry, tmpl, state)
}

// spawnPersisted places one instance of a database-tracked entry, reusing
// persisted HP and position when the row was still alive, or a freshly
// rolled position at full HP otherwise (CheckAlive's own restore rule).
func (n *Npcs) spawnPersisted(key string, entry spawn.Entry, tmpl *npc.Template, state *spawn.State) {
	now := n.now()
	pos := pickPosition(entry.Positions)

	loc, heading, hp := pos.Location, pos.Heading, int(tmpl.HPMax)
	if state.CheckAlive(pos.Location, pos.Heading, int(tmpl.HPMax), 0, now) {
		loc, heading, hp = state.Location, state.Heading, state.CurrentHP
	}
	n.instantiate(key, entry, tmpl, loc, heading, hp)
}

// spawnFresh places one non-persisted instance of entry at a freshly rolled
// position, always alive at full HP — the reference server never restores
// HP/position across restarts for a spawn without a database name.
func (n *Npcs) spawnFresh(key string, entry spawn.Entry, tmpl *npc.Template) {
	pos := pickPosition(entry.Positions)
	n.instantiate(key, entry, tmpl, pos.Location, pos.Heading, int(tmpl.HPMax))
}

// instantiate builds one live Hostile from tmpl and places it in the world
// at (loc, heading) with hp current HP, registering it for AI ticks and
// corpse decay/respawn.
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

	hostile, err := newLiveHostile(inst, tmpl.RunSpeed, n.geo)
	if err != nil {
		n.log.Warn().Err(err).Int32("npc_id", entry.NPCID).Msg("spawn: cannot build live npc")
		return
	}

	hostile.SetCurrentHP(hp)
	hostile.SetWorld(n.state)
	hostile.SetRewarder(n.rewarderFor(hostile, tmpl))

	n.state.Spawn(hostile, loc.X, loc.Y, loc.Z, heading)
	n.ai.Add(hostile)
	if tmpl.CorpseTime > 0 {
		n.decay.Add(hostile, time.Duration(tmpl.CorpseTime)*time.Second)
	}

	n.mu.Lock()
	n.live[id] = key
	n.liveCount++
	n.mu.Unlock()
}

// rewarderFor returns the kill-reward hook for a newly spawned hostile, or
// nil when its template has no drop table at all. Experience/SP are not
// granted here: that formula (player.KillRewardExpAndSp) needs a live
// player actor to credit, and player-side combat isn't wired to a live
// actor yet (tracked separately) — only item/spoil/herb drops are reachable
// from a real kill at this point.
func (n *Npcs) rewarderFor(hostile *npc.Hostile, tmpl *npc.Template) *deathDrops {
	if len(tmpl.Drops) == 0 {
		return nil
	}
	return &deathDrops{
		hostile:    hostile,
		categories: tmpl.Drops,
		rates:      n.rates,
		raid:       tmpl.Type == "RaidBoss",
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
func (n *Npcs) RespawnHook(actorID int32) func() {
	if obj, ok := n.state.Object(actorID); ok {
		if h, ok := obj.(*npc.Hostile); ok {
			n.ai.Remove(h)
		}
	}

	n.mu.Lock()
	key, tracked := n.live[actorID]
	if tracked {
		delete(n.live, actorID)
		n.liveCount--
	}
	n.mu.Unlock()
	if !tracked {
		return nil
	}

	n.mu.Lock()
	slot, ok := n.slot[key]
	n.mu.Unlock()
	if !ok {
		return nil
	}

	delay := spawn.CalculateRespawnDelay(slot.entry)
	if delay <= 0 {
		n.mu.Lock()
		delete(n.slot, key)
		n.mu.Unlock()
		return nil
	}

	return func() { n.scheduleRespawn(slot, delay) }
}

func (n *Npcs) scheduleRespawn(slot slotInfo, delay time.Duration) {
	now := n.now()
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
func (n *Npcs) Respawn(key string) {
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

	if slot.dbName != "" {
		state, ok := n.spawns.State(slot.dbName)
		if !ok {
			state = spawn.NewState(slot.dbName)
		}
		n.spawnPersisted(key, slot.entry, tmpl, state)
		return
	}
	n.spawnFresh(key, slot.entry, tmpl)
}

// SyncPersistedState writes every live database-tracked slot's current HP
// and position back into its spawn.State row, ready for Spawns.Save. Dead
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
		state, ok := n.spawns.State(slot.dbName)
		if !ok {
			continue
		}
		x, y, z := hostile.Position()
		state.SetStats(hostile.CurrentHP(), 0, location.Location{X: x, Y: y, Z: z}, hostile.Heading())
	}
}

// LiveCount returns the number of currently spawned (not decayed) NPCs.
func (n *Npcs) LiveCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.liveCount
}

// DeferredCount returns the number of spawn entries skipped at boot for
// lacking an explicit position (territory-random placement).
func (n *Npcs) DeferredCount() int {
	return int(n.deferredCount.Load())
}

// RestoredDeadCount returns the number of database-tracked entries that
// were still dead with a pending respawn deadline at boot.
func (n *Npcs) RestoredDeadCount() int {
	return int(n.restoredDeadCount.Load())
}

// SkippedNonCombatCount returns the number of spawn entries skipped at boot
// for resolving to a non-combat instance type (shops, trainers, and similar
// service NPCs the dialog pipeline doesn't support yet).
func (n *Npcs) SkippedNonCombatCount() int {
	return int(n.skippedNonCombatCount.Load())
}

// pickPosition selects one spawn position from positions. A single entry
// (the "fixed" declaration) is used exactly as declared, heading included.
// Multiple entries (the "chance-weighted" declaration) are chosen by
// rolling a percentage against each Chance in turn — and, matching the
// reference server's own behavior for this shape, the winning entry's
// declared heading is discarded in favor of a fresh random one. A weight
// table that doesn't sum to 100 falls back to the last entry rather than
// leaving the slot unspawned.
func pickPosition(positions []spawn.Position) spawn.Position {
	if len(positions) == 1 {
		return positions[0]
	}

	chance := rnd.Get(100)
	for _, pos := range positions {
		chance -= pos.Chance
		if chance < 0 {
			pos.Heading = rnd.Get(65536)
			return pos
		}
	}
	last := positions[len(positions)-1]
	last.Heading = rnd.Get(65536)
	return last
}

// locatedRef and creatureActorRef are forward references that break the
// construction cycle between a live NPC and the movement/attack
// controllers it owns: the controllers need the NPC's position/combat
// surface, but the NPC's own constructor needs the controllers already
// built. Each embeds its target interface unset, is handed to the
// controller constructors, and is pointed at the real NPC immediately
// after — before anything can call through it.
type locatedRef struct{ move.Located }
type creatureActorRef struct{ attack.CreatureActor }

// newLiveHostile builds a live Hostile for inst, wiring a real movement
// controller (over a fresh CreatureMove seeded at inst.Home) and a real
// attack controller, resolving their mutual construction-order dependency
// on the finished Hostile via locatedRef/creatureActorRef.
func newLiveHostile(inst *npc.Instance, speed float64, geo move.Geo) (*npc.Hostile, error) {
	cm, err := move.NewCreatureMove(inst.Home, speed, geo)
	if err != nil {
		return nil, err
	}

	locRef := &locatedRef{}
	moveCtl, err := move.NewController(cm, locRef)
	if err != nil {
		return nil, err
	}

	actorRef := &creatureActorRef{}
	attackCtl := attack.NewAttackable(actorRef)

	hostile, err := npc.NewHostile(inst, moveCtl, attackCtl)
	if err != nil {
		return nil, err
	}

	locRef.Located = hostile
	actorRef.CreatureActor = hostile
	return hostile, nil
}

// deathDrops rolls one victim's item/spoil/herb rewards at its position at
// the moment of death, rather than a position fixed when it spawned —
// hostile NPCs can move (offensive follow) between spawning and dying.
type deathDrops struct {
	hostile    *npc.Hostile
	categories []item.DropCategory
	rates      item.Rates
	raid       bool
	ids        idAllocator
	items      *item.Table
	ground     groundPlacer
}

// CalculateRewards implements creature.Rewarder.
func (d *deathDrops) CalculateRewards(killer creature.DeathActor) {
	x, y, z := d.hostile.Position()
	heading := d.hostile.Heading()
	// pool is nil: the spoil mechanic (marking a monster spoiled via a
	// skill cast) isn't wired to a live actor yet, so RollKillReward's own
	// nil-pool handling (skip spoil rolls) is the correct behavior here,
	// not a workaround. levelMultiplier is 1 (no penalty) and rates are
	// all neutral: the drop-rate config surface (RateDropAdena and
	// friends) and the killer-level resolution needed for
	// item.LevelPenaltyMultiplier aren't loaded/wired anywhere yet either.
	NewKillReward(d.categories, nil, 1, d.raid, d.rates, false, d.ids, d.items, d.ground, x, y, z, heading).CalculateRewards(killer)
}
