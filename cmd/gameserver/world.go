package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	"github.com/fatal10110/acis_golang/internal/gameserver/boat"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

func provideRoster(cfg gameServerConfig, data *gameData, characters *gamesql.CharacterStore, items *gamesql.ItemStore, shortcuts *gamesql.ShortcutStore, subclasses *gamesql.SubclassStore, ids *idfactory.Allocator) *manager.Roster {
	roster := manager.NewRoster(characters, items, shortcuts, data.Players, data.Items, data.NPCs, ids, cfg.CharacterDeleteAfter, time.Now)
	roster.SetSubclasses(subclasses)
	return roster
}

// provideWorldObjects spawns every door and static object template into
// state at boot, applying closed doors to geodata immediately, and wires the
// door-timer task's late-bound hook to it — manager.WorldObjects needs
// *task.Door to schedule timers with, so that task's own effects can only
// point back at WorldObjects after it exists.
func provideWorldObjects(data *gameData, ids *idfactory.Allocator, state *world.State, doorTimers *task.Door, doorHooks *doorTimerEffects, doorRegen *task.DoorRegen, log zerolog.Logger) (*manager.WorldObjects, error) {
	objs, err := manager.NewWorldObjects(data.Doors, data.Statics, ids, data.Geo, state, doorTimers, doorRegen, network.DoorSinks(state), log)
	if err != nil {
		return nil, err
	}
	doorHooks.SetHook(objs.ToggleDoor)
	return objs, nil
}

// provideBoats spawns one scheduled boat per boatRoutes.xml itinerary, unless
// AllowBoat turns boats off; it then returns a nil fleet.
func provideBoats(cfg gameServerConfig, paths gameServerPaths, ids *idfactory.Allocator, state *world.State, log zerolog.Logger) (*boat.Fleet, error) {
	if !cfg.AllowBoat {
		return nil, nil
	}
	itineraries, err := gamexml.LoadBoatRoutes(filepath.Join(paths.DataRoot, "data", "xml", "boatRoutes.xml"))
	if err != nil {
		return nil, err
	}
	fleet, err := boat.New(itineraries, ids, state, network.BoatSinks(state))
	if err != nil {
		return nil, err
	}
	log.Info().Int("itineraries", len(itineraries)).Msg("boat itineraries loaded")
	return fleet, nil
}

// startBoats runs the boats' schedules and movement while the server runs.
func startBoats(lc fx.Lifecycle, fleet *boat.Fleet, log zerolog.Logger) {
	if fleet == nil {
		return
	}
	startTicker(lc, log, fleet.Start)
}

func startWorldObjects(objs *manager.WorldObjects, log zerolog.Logger) {
	log.Info().Int("doors", len(objs.Doors())).Int("static_objects", len(objs.StaticObjects())).Msg("world objects spawned")
}

// provideSpawns loads the spawnlist XML and restores dynamic spawn_data
// rows, returning the store alongside so it can be reused to persist state
// back at shutdown.
func provideSpawns(ctx bootContext, paths gameServerPaths, pool *sql.DB, log zerolog.Logger, gameplay gameplayConfig) (*manager.Spawns, *gamesql.SpawnStore, error) {
	store := gamesql.NewSpawnStore(pool)
	dir := filepath.Join(paths.DataRoot, "data", "xml", "spawnlist")
	spawns, err := manager.LoadSpawns(ctx, dir, store, log, float64(gameplay.SpawnMultiplier))
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
func provideNpcs(spawns *manager.Spawns, data *gameData, state *world.State, ids *idfactory.Allocator, decay *task.Decay, decayHooks *worldDecayEffects, respawnTask *task.Respawn, respawnHooks *npcRespawnEffects, ai *task.AI, positions *task.PositionUpdates, ground *task.GroundItems, rewards manager.KillRewardConfig, gameplay gameplayConfig, log zerolog.Logger, walker *task.Walker, link *network.GameClientLink, attackStance *task.AttackStance, effects effect.Env, pool *sim.Pool, makers *script.Makers, scripts *script.Registry) (*manager.Npcs, error) {
	rewards.Parties = link
	rewards.RaidKills = link
	rewards.Channels = link
	rewards.CursedWeapons = link
	npcs, err := manager.NewNpcsWithMaxBuffsAmount(spawns, data.NPCs, move.NewGeo(data.Geo, data.Finder), state, ids, decay, respawnTask, ai, positions, data.Items, ground, rewards, time.Now, log,
		data.Skills, link.HostileCastEffects(), walker, network.HostileSinks(state, attackStance), link.FolkSinks(state, attackStance), int(gameplay.MaxBuffsAmount), int(gameplay.RandomWalkRate), int(gameplay.MaxGeoPathFailCount), gameplay.RaidMultipliers, gameplay.NpcAI, gameplay.SpawnEvents, effects, pool, makers, scripts, data.Zones)
	if err != nil {
		return nil, err
	}
	decayHooks.SetRespawnHook(npcs.RespawnHook)
	respawnHooks.SetHook(npcs.Respawn)
	link.SetNpcSpawns(npcs)
	return npcs, nil
}

func startNpcs(npcs *manager.Npcs, log zerolog.Logger) {
	log.Info().
		Int("live_npcs", npcs.LiveCount()).
		Int("folk_npcs", npcs.FolkCount()).
		Int("deferred_territory_spawns", npcs.DeferredCount()).
		Int("restored_dead_spawns", npcs.RestoredDeadCount()).
		Int("skipped_non_combat_spawns", npcs.SkippedNonCombatCount()).
		Msg("npc spawns loaded")
}

// startNpcPersistence syncs every live database-tracked spawn's current
// HP/position into its spawn.State row and saves spawn_data at shutdown,
// from the spawn list in use (a //respawnall replaces the boot one). The
// save runs under its own budget: it stops ahead of the final item flush, and on a
// slow database it would otherwise spend the rest of the stop budget.
func startNpcPersistence(lc fx.Lifecycle, npcs *manager.Npcs, store *gamesql.SpawnStore, log zerolog.Logger) {
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			npcs.SyncPersistedState()
			saveOnStop(ctx, shutdownSaveTimeout, log, "save spawn data", func(ctx context.Context) error {
				return npcs.Spawns().Save(ctx, store)
			})
			return nil
		},
	})
}

// saveOnStop runs one shutdown save under at most budget of the stop
// context, and logs a failure rather than returning it: the stop hooks after
// it still have to run.
func saveOnStop(ctx context.Context, budget time.Duration, log zerolog.Logger, what string, save func(context.Context) error) {
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	if err := save(ctx); err != nil {
		log.Warn().Err(err).Msg(what)
	}
}
