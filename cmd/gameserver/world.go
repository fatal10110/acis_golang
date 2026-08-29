package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

func provideRoster(cfg gameServerConfig, data *gameData, characters *gamesql.CharacterStore, items *gamesql.ItemStore, shortcuts *gamesql.ShortcutStore, ids *idfactory.Allocator) *manager.Roster {
	return manager.NewRoster(characters, items, shortcuts, data.Players, data.Items, data.NPCs, ids, manager.DefaultDeleteAfter, time.Now)
}

// provideWorldObjects spawns every door and static object template into
// state at boot, applying closed doors to geodata immediately, and wires the
// door-timer task's late-bound hook to it — manager.WorldObjects needs
// *task.Door to schedule timers with, so that task's own effects can only
// point back at WorldObjects after it exists.
func provideWorldObjects(data *gameData, ids *idfactory.Allocator, state *world.State, doorTimers *task.Door, doorHooks *doorTimerEffects, log zerolog.Logger) (*manager.WorldObjects, error) {
	objs, err := manager.NewWorldObjects(data.Doors, data.Statics, ids, data.Geo, state, doorTimers, log)
	if err != nil {
		return nil, err
	}
	doorHooks.SetHook(objs.ToggleDoor)
	return objs, nil
}

func startWorldObjects(objs *manager.WorldObjects, log zerolog.Logger) {
	log.Info().Int("doors", len(objs.Doors())).Int("static_objects", len(objs.StaticObjects())).Msg("world objects spawned")
}

// provideSpawns loads the spawnlist XML and restores dynamic spawn_data
// rows, returning the store alongside so it can be reused to persist state
// back at shutdown.
func provideSpawns(paths gameServerPaths, pool *sql.DB, log zerolog.Logger, multiplier spawnMultiplier) (*manager.Spawns, *gamesql.SpawnStore, error) {
	store := gamesql.NewSpawnStore(pool)
	dir := filepath.Join(paths.DataRoot, "data", "xml", "spawnlist")
	spawns, err := manager.LoadSpawns(context.Background(), dir, store, log, float64(multiplier))
	if err != nil {
		return nil, nil, err
	}
	log.Info().
		Int("spawn_makers", spawns.Table().MakerCount()).
		Int("spawn_entries", spawns.Table().SpawnCount()).
		Int("persisted_spawn_rows", spawns.StateCount()).
		Msg("spawn list loaded")
	return spawns, store, nil
}

// provideNpcs instantiates every "on start" spawn entry into state at boot,
// then wires the decay/respawn tasks' late-bound hooks to it — manager.Npcs
// needs *task.Decay and *task.Respawn to register actors with, so those
// tasks' own effects can only point back at Npcs after it exists.
func provideNpcs(spawns *manager.Spawns, data *gameData, state *world.State, ids *idfactory.Allocator, decay *task.Decay, decayHooks *worldDecayEffects, respawnTask *task.Respawn, respawnHooks *npcRespawnEffects, ai *task.AI, positions *task.PositionUpdates, ground *task.GroundItems, rewards manager.KillRewardConfig, log zerolog.Logger, walker *task.Walker) (*manager.Npcs, error) {
	// castTargets/castHandlers are a boot-owned instance for the hostile-NPC
	// AI cast seam (issue #1612), built the same way NewGameClientLink builds
	// its own per-connection instance — NPCs are spawned before any client
	// connects, so they cannot share that one.
	castTargets := skilltarget.NewRegistry(skilltarget.WorldKnown{State: state})
	castHandlers := handlerskill.NewDefaultRegistryWithSignet(data.Skills, handlerskill.SignetDeps{
		Templates: data.NPCs,
		IDs:       ids,
		World:     state,
		Log:       log,
	})
	npcs, err := manager.NewNpcs(spawns, data.NPCs, move.NewGeo(data.Geo, data.Finder), state, ids, decay, respawnTask, ai, positions, data.Items, ground, rewards, time.Now, log,
		data.Skills, actorcast.EffectHandlers{Targets: castTargets, Skills: castHandlers}, walker)
	if err != nil {
		return nil, err
	}
	decayHooks.SetRespawnHook(npcs.RespawnHook)
	respawnHooks.SetHook(npcs.Respawn)
	return npcs, nil
}

func startNpcs(npcs *manager.Npcs, log zerolog.Logger) {
	log.Info().
		Int("live_npcs", npcs.LiveCount()).
		Int("deferred_territory_spawns", npcs.DeferredCount()).
		Int("restored_dead_spawns", npcs.RestoredDeadCount()).
		Int("skipped_non_combat_spawns", npcs.SkippedNonCombatCount()).
		Msg("npc spawns loaded")
}

// startNpcPersistence syncs every live database-tracked spawn's current
// HP/position into its spawn.State row and saves spawn_data at shutdown,
// mirroring the reference server's own save-on-shutdown behavior.
func startNpcPersistence(lc fx.Lifecycle, npcs *manager.Npcs, spawns *manager.Spawns, store *gamesql.SpawnStore, log zerolog.Logger) {
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			npcs.SyncPersistedState()
			if err := spawns.Save(ctx, store); err != nil {
				log.Warn().Err(err).Msg("save spawn data")
			}
			return nil
		},
	})
}
