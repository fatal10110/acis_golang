package manager

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
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
	maker  *spawn.Maker
	entry  spawn.Entry
	dbName string
}

// KillRewardConfig carries live reward settings loaded at game-server boot.
type KillRewardConfig struct {
	Rates             item.Rates
	AutoLoot          bool
	AutoLootRaid      bool
	AutoLootHerbs     bool
	DeepBlueDropRules bool
	PlayerLevels      *player.LevelTable
	PartyRange        int
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
	positions *task.PositionUpdates
	items     *item.Table
	ground    groundPlacer
	rewards   KillRewardConfig
	spawns    *Spawns
	now       func() time.Time
	log       zerolog.Logger

	// castDefs and castEffects wire a live Hostile's cast.AIController at
	// spawn (see newLiveHostile). castDefs is nil-checked so a caller with
	// no skill data loaded (e.g. an existing test harness) still gets a
	// live Hostile with no AI-cast capability, matching a CastController-
	// less ai.Attackable's existing "no skills to cast" contract.
	castDefs    actorcast.Definitions
	castEffects actorcast.EffectHandlers

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
func NewNpcs(spawns *Spawns, templates *npc.Table, geo move.Geo, state *world.State, ids idAllocator, decay *task.Decay, respawnTask *task.Respawn, ai *task.AI, positions *task.PositionUpdates, items *item.Table, ground groundPlacer, rewards KillRewardConfig, now func() time.Time, log zerolog.Logger, castDefs actorcast.Definitions, castEffects actorcast.EffectHandlers) (*Npcs, error) {
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
	if positions == nil {
		return nil, fmt.Errorf("npcs: nil position updates task")
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
		templates:   templates,
		geo:         geo,
		state:       state,
		ids:         ids,
		decay:       decay,
		respawn:     respawnTask,
		ai:          ai,
		positions:   positions,
		items:       items,
		ground:      ground,
		rewards:     rewards,
		spawns:      spawns,
		now:         now,
		log:         log,
		castDefs:    castDefs,
		castEffects: castEffects,
		slot:        make(map[string]slotInfo),
		live:        make(map[int32]string),
	}

	for _, maker := range spawns.Table().Makers() {
		if !isOnStartMaker(maker) {
			continue
		}
		remaining := maker.MaximumNPCs
		for entryIndex, entry := range maker.Entries {
			n.bootSpawnEntry(maker, entryIndex, entry, &remaining)
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
