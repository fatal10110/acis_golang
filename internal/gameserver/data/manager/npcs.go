package manager

import (
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// slotInfo is the static definition of one spawn slot: the entry it was
// declared under, and (when non-empty) the persisted state row backing it.
// A slot with a non-empty dbName is the only kind restored across restarts
// and forced to a single live instance: a database-tracked spawn ignores
// its declared total and only ever has one instance.
type slotInfo struct {
	key      string
	maker    *spawn.Maker
	entry    spawn.Entry
	dbName   string
	masterID int32
	liveID   int32
	// tmpl is the template resolved when the slot was declared. Every
	// respawn of the slot builds from it, so a //reload npc swapping the
	// template table leaves the slot's existing spawn unchanged: only slots
	// declared afterwards (//respawnall, a new //spawn) use the reloaded
	// templates.
	tmpl *npc.Template
	// fixed marks a standalone spawn placed at a point of its own, not
	// declared under a maker: at and heading are that point. It never
	// respawns.
	fixed   bool
	at      location.Location
	heading int
}

// KillRewardConfig carries live reward settings loaded at game-server boot.
type KillRewardConfig struct {
	Rates         item.Rates
	AutoLoot      bool
	AutoLootRaid  bool
	AutoLootHerbs bool
	// MultipleItemDrop drops a non-stackable item rolled with a count
	// above one as that many ground items; false drops just one.
	MultipleItemDrop  bool
	DeepBlueDropRules bool
	PlayerLevels      *player.LevelTable
	PartyRange        int
	// PartyXP are the rules a party shares a kill's exp and sp by.
	PartyXP player.PartyXPRules
	// Parties resolves a rewarded player's party; nil rewards every
	// attacker alone.
	Parties RewardParties
	// RaidKills credits a raid or grand boss kill to the killer's side;
	// nil credits nobody.
	RaidKills RaidKillRecorder
	// Channels resolves the command channel that wins a raid or grand
	// boss's loot rights; nil lets no channel win them.
	Channels LootChannels
	// CursedWeapons rolls a monster kill's cursed weapon drop; nil drops
	// none.
	CursedWeapons CursedWeaponDrops
}

// CursedWeaponDrops rolls whether a monster kill drops a cursed weapon.
type CursedWeaponDrops interface {
	// DropCursedWeapon rolls killer's kill of the monster dropperID, which
	// died at (x, y, z).
	DropCursedWeapon(killer *player.Character, dropperID int32, x, y, z int)
}

// RaidKillRecorder credits a raid or grand boss kill.
type RaidKillRecorder interface {
	// RecordRaidKill credits the kill of the boss, of npc id bossID and
	// level bossLevel, by the player killer acts for.
	RecordRaidKill(killer *player.Character, bossID int32, bossLevel int)
}

// Npcs owns every live NPC instantiated from the spawn table at boot,
// indexed by object id, and drives their decay/respawn/AI lifecycle.
//
// Spawn entries with an explicit "pos" attribute are placed at that fixed
// or chance-weighted coordinate. Entries with no declared positions roll a
// random point inside their maker's territory polygon and resolve the Z
// through geodata. Entry.Privates (child minion spawns declared under one
// spawn position) are not instantiated here; minion fan-out has its own
// master/minion linking concerns.
//
// A combat-capable entry becomes a npc.Hostile with an AI loop, decay and
// respawn. A civilian service NPC (a shop, trainer, gatekeeper, village
// master and the like) becomes a npc.Folk for players to talk to: it
// stands at its spawn point, or walks its route when its template alias
// names one in the walker route data. A mortal one that dies decays and
// respawns through its slot like a hostile. Any other non-combat instance type
// (castle artifacts, siege flags, towers) is counted and skipped.
//
// All exported methods are safe for concurrent use; mu guards slots/live.
type Npcs struct {
	effects        effect.Env
	templates      *npc.Table
	maxBuffsAmount int
	randomWalkRate int
	// maxGeoPathFailCount is each hostile's pathfinding-fail overflow
	// threshold; zero keeps the default.
	maxGeoPathFailCount int
	// raidMultipliers scale every raid-related hostile's base defences and
	// regeneration.
	raidMultipliers npc.RaidMultipliers
	// aiConfig holds every hostile's target-selection switches.
	aiConfig npc.AIConfig
	// events is the SpawnEvents list: the event makers it names spawn
	// after the on-start makers, and it gates the spawn and respawn of
	// event makers.
	events    spawnEvents
	geo       move.Geo
	state     *world.State
	ids       idAllocator
	decay     *task.Decay
	respawn   *task.Respawn
	ai        *task.AI
	positions *task.PositionUpdates
	items     *item.Table
	ground    groundPlacer
	// newSink builds the event sink each spawned NPC reports through; nil
	// leaves spawned NPCs silent.
	newSink func(*npc.Hostile) event.Sink
	// folk places civilian NPCs, walking the route walkers.
	folk    FolkSpawner
	rewards KillRewardConfig
	// spawns is the spawn list the makers come from; RespawnAll replaces
	// it, under mu.
	spawns *Spawns
	now    func() time.Time
	log    zerolog.Logger
	walker *task.Walker
	zones  *zone.Index
	queues Queues

	// castDefs and castEffects wire a live Hostile's cast.AIController at
	// spawn (see newLiveHostile). castDefs is nil-checked so a caller with
	// no skill data loaded (e.g. an existing test harness) still gets a
	// live Hostile with no AI-cast capability, matching a CastController-
	// less ai.Attackable's existing "no skills to cast" contract.
	castDefs    actorcast.Definitions
	castEffects actorcast.EffectHandlers

	// gate orders the whole-population changes against every single spawn:
	// DespawnAll and RespawnAll hold it, Respawn and SpawnFixed read-hold it,
	// so a spawn never lands halfway through a despawn of everything.
	gate sync.RWMutex

	mu   sync.Mutex
	slot map[string]slotInfo
	live map[int32]string
	// gen counts the RespawnAll runs; it suffixes the slot keys after the
	// first, so a respawn armed for a slot of an earlier run never fires
	// on the slot of the same maker entry spawned anew.
	gen int

	// liveCount is guarded by mu, not atomic: every update pairs it with a
	// live map write/delete that must stay consistent with the count.
	liveCount int

	// deferredCount/restoredDeadCount/skippedNonCombatCount are lone
	// increments with no other state to keep in sync, so they're atomic
	// rather than sharing mu.
	deferredCount         atomic.Int64
	restoredDeadCount     atomic.Int64
	skippedNonCombatCount atomic.Int64
	folkCount             atomic.Int64
	// fixedSeq numbers the standalone spawns SpawnFixed places.
	fixedSeq atomic.Int64
}

// NewNpcs walks spawns' loaded table and instantiates every "on start"
// maker's qualifying entries into state, respecting persisted dead/alive
// data for database-tracked entries.
func NewNpcs(spawns *Spawns, templates *npc.Table, geo move.Geo, state *world.State, ids idAllocator, decay *task.Decay, respawnTask *task.Respawn, ai *task.AI, positions *task.PositionUpdates, items *item.Table, ground groundPlacer, rewards KillRewardConfig, now func() time.Time, log zerolog.Logger, castDefs actorcast.Definitions, castEffects actorcast.EffectHandlers, walker *task.Walker, newSink func(*npc.Hostile) event.Sink, effects effect.Env, queues Queues, zoneIndexes ...*zone.Index) (*Npcs, error) {
	return newNpcs(spawns, templates, geo, state, ids, decay, respawnTask, ai, positions, items, ground, rewards, now, log, castDefs, castEffects, walker, newSink, nil, 20, 30, 0, npc.DefaultRaidMultipliers(), npc.DefaultAIConfig(), DefaultSpawnEvents(), effects, queues, zoneIndexes...)
}

// Queues creates the queue one live NPC's work runs on; id names it in logs.
type Queues interface {
	NewQueue(id string) *sim.Queue
}

// NewNpcsWithMaxBuffsAmount builds live NPCs with the configured buff-slot
// base, RandomWalkRate and raid base multipliers, each running its work on a
// queue from queues. newFolkSink builds the sink a civilian NPC shows its
// movement and status through. events is the SpawnEvents list.
func NewNpcsWithMaxBuffsAmount(spawns *Spawns, templates *npc.Table, geo move.Geo, state *world.State, ids idAllocator, decay *task.Decay, respawnTask *task.Respawn, ai *task.AI, positions *task.PositionUpdates, items *item.Table, ground groundPlacer, rewards KillRewardConfig, now func() time.Time, log zerolog.Logger, castDefs actorcast.Definitions, castEffects actorcast.EffectHandlers, walker *task.Walker, newSink func(*npc.Hostile) event.Sink, newFolkSink func(*npc.Folk) event.Sink, maxBuffsAmount, randomWalkRate, maxGeoPathFailCount int, raidMultipliers npc.RaidMultipliers, aiConfig npc.AIConfig, events []string, effects effect.Env, queues Queues, zoneIndexes ...*zone.Index) (*Npcs, error) {
	return newNpcs(spawns, templates, geo, state, ids, decay, respawnTask, ai, positions, items, ground, rewards, now, log, castDefs, castEffects, walker, newSink, newFolkSink, maxBuffsAmount, randomWalkRate, maxGeoPathFailCount, raidMultipliers, aiConfig, events, effects, queues, zoneIndexes...)
}

func newNpcs(spawns *Spawns, templates *npc.Table, geo move.Geo, state *world.State, ids idAllocator, decay *task.Decay, respawnTask *task.Respawn, ai *task.AI, positions *task.PositionUpdates, items *item.Table, ground groundPlacer, rewards KillRewardConfig, now func() time.Time, log zerolog.Logger, castDefs actorcast.Definitions, castEffects actorcast.EffectHandlers, walker *task.Walker, newSink func(*npc.Hostile) event.Sink, newFolkSink func(*npc.Folk) event.Sink, maxBuffsAmount, randomWalkRate, maxGeoPathFailCount int, raidMultipliers npc.RaidMultipliers, aiConfig npc.AIConfig, events []string, effects effect.Env, queues Queues, zoneIndexes ...*zone.Index) (*Npcs, error) {
	if spawns == nil || spawns.Table() == nil {
		return nil, fmt.Errorf("npcs: nil spawn table")
	}
	if templates == nil {
		return nil, fmt.Errorf("npcs: nil npc template table")
	}
	if geo == nil {
		return nil, fmt.Errorf("npcs: nil geo")
	}
	if effects.Activity == nil {
		return nil, fmt.Errorf("npcs: nil effect activity registry")
	}
	if state == nil {
		return nil, fmt.Errorf("npcs: nil world state")
	}
	if queues == nil {
		return nil, fmt.Errorf("npcs: nil queues")
	}
	if newSink == nil {
		// NPCs spawned without a sink factory never reach a client: no
		// attack, status, or death broadcast leaves them and nothing
		// errors. Domain tests boot this way on purpose, so this warns
		// rather than fails, but a production composition root reaching it
		// is a wiring bug.
		log.Warn().Msg("npcs: no NPC event sink factory; spawned NPCs will not broadcast to clients")
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
	if positions == nil {
		return nil, fmt.Errorf("npcs: nil position updates task")
	}
	if items == nil {
		return nil, fmt.Errorf("npcs: nil item table")
	}
	if ground == nil {
		return nil, fmt.Errorf("npcs: nil ground placer")
	}
	if walker == nil {
		return nil, fmt.Errorf("npcs: nil walker task")
	}
	if now == nil {
		now = time.Now
	}
	var zones *zone.Index
	if len(zoneIndexes) != 0 {
		zones = zoneIndexes[0]
	}

	n := &Npcs{
		templates:           templates,
		effects:             effects,
		maxBuffsAmount:      maxBuffsAmount,
		randomWalkRate:      randomWalkRate,
		maxGeoPathFailCount: maxGeoPathFailCount,
		raidMultipliers:     raidMultipliers,
		aiConfig:            aiConfig,
		events:              newSpawnEvents(events),
		geo:                 geo,
		state:               state,
		ids:                 ids,
		decay:               decay,
		respawn:             respawnTask,
		ai:                  ai,
		positions:           positions,
		items:               items,
		ground:              ground,
		rewards:             rewards,
		spawns:              spawns,
		now:                 now,
		log:                 log,
		walker:              walker,
		newSink:             newSink,
		zones:               zones,
		queues:              queues,
		castDefs:            castDefs,
		castEffects:         castEffects,
		slot:                make(map[string]slotInfo),
		live:                make(map[int32]string),
	}
	n.folk = FolkSpawner{
		State:               state,
		Walker:              walker,
		Geo:                 geo,
		Positions:           positions,
		Queues:              queues,
		NewSink:             newFolkSink,
		Zones:               zones,
		Skills:              castDefs,
		CastEffects:         castEffects,
		AI:                  ai,
		Items:               items,
		Decay:               decay,
		Effects:             effects,
		MaxBuffsAmount:      maxBuffsAmount,
		MaxGeoPathFailCount: maxGeoPathFailCount,
		Log:                 log,
	}

	n.spawnOnStart(spawns, 0)
	return n, nil
}

// spawnOnStart spawns every on-start maker of spawns its spawn condition
// allows, then the makers of the listed spawn events, as SpawnManager.spawn
// does at boot, its slots keyed for RespawnAll run gen.
func (n *Npcs) spawnOnStart(spawns *Spawns, gen int) {
	makers := spawns.Table().Makers()
	for _, maker := range makers {
		if isOnStartMaker(maker) && n.events.allows(maker) {
			n.spawnMaker(maker, gen)
		}
	}
	n.spawnEventMakers(makers, gen)
}

// spawnMaker spawns maker's entries within its shared spawn budget, its
// slots keyed for RespawnAll run gen.
func (n *Npcs) spawnMaker(maker *spawn.Maker, gen int) {
	remaining := maker.MaximumNPCs
	for entryIndex, entry := range maker.Entries {
		n.bootSpawnEntry(maker, entryIndex, entry, &remaining, gen)
	}
}

// slotKey is the key of the slot base names in RespawnAll run gen: base
// itself at boot.
func slotKey(base string, gen int) string {
	if gen == 0 {
		return base
	}
	return base + "@" + strconv.Itoa(gen)
}

// currentSpawns returns the spawn list in use.
func (n *Npcs) currentSpawns() *Spawns {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.spawns
}

// Spawns returns the spawn list in use: the one loaded at boot, or the one
// the last RespawnAll put in its place.
func (n *Npcs) Spawns() *Spawns {
	return n.currentSpawns()
}

// isOnStartMaker reports whether maker should be populated at boot: it has
// no event gate and its ai params don't disable the initial spawn. Makers
// with an "ai type" that scripts special spawn selection (random pick
// among candidates, exclusive slots, day/night toggles, etc.) are treated
// the same as the default "spawn every entry up to its total" behavior —
// no scripted maker framework exists in this codebase yet — except that
// spawnEvents.allows gates an event_maker on its EventName.
