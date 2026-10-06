// Command gameserver boots the game server process.
package main

import (
	"context"
	"flag"
	"time"

	"go.uber.org/fx"

	"github.com/fatal10110/acis_golang/internal/gameserver/castlemanor"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/fishchamp"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

const (
	// shutdownSaveTimeout bounds each single-table save a stop hook runs
	// (spawn_data, items_on_ground), so a slow database spends at most this
	// much of gameServerStopTimeout on it.
	shutdownSaveTimeout = 10 * time.Second
	// debugHTTPStopTimeout bounds the debug listener's graceful stop, so an
	// in-flight profile request cannot hold up the shutdown.
	debugHTTPStopTimeout = 2 * time.Second
	// simPoolStopTimeout bounds the wait for the actor pool's workers to
	// finish their queued, in-memory tasks.
	simPoolStopTimeout = 5 * time.Second
	// persistCloseTimeout bounds the persistence worker's last close, which
	// gives lanes the item drain's own close left backed up a little longer.
	// The ground-item save and the database close still follow it.
	persistCloseTimeout = 5 * time.Second
	// gameServerStopSlack covers the stop hooks that neither wait on the
	// database nor on other goroutines' work: in-memory tickers finishing a
	// tick, stopping a timer, closing the logger. Closing the database pool
	// also waits for queries already running, but it is the last database
	// step, so running past the deadline there loses nothing.
	gameServerStopSlack = 5 * time.Second
	// gameServerStopTimeout bounds the whole shutdown sequence. fx runs every
	// stop hook in turn under this one deadline; once it expires fx skips
	// every hook it has not reached, and Run exits the process with any
	// running hook cut off. A step's own detached budget therefore cannot
	// carry it past this deadline, so the deadline is the sum of every
	// step's worst case, in stop order. A slow database then cannot spend
	// the budget before the final item flush and the ground-item save run.
	// TestGameServerStopTimeoutCoversEveryStopStep checks every registered
	// stop hook against this sum.
	gameServerStopTimeout = network.ShutdownCloseGrace + // listener: connections flushing their ServerClose before the rest are force-closed
		network.LivePlayerPersistWait + // listener: each connection's exit waits for its player's saves, in parallel
		debugHTTPStopTimeout +
		shutdownSaveTimeout + // spawn_data
		shutdownSaveTimeout + // character_relations
		shutdownSaveTimeout + // petition, petition_message
		shutdownSaveTimeout + // seven_signs, seven_signs_status
		simPoolStopTimeout +
		task.ItemInstanceSaveTimeout + // item ticker: its stop cancels an in-flight save, so this is headroom, not task.ItemInstanceTickBudget
		3*task.ItemInstanceSaveTimeout + // drainItemInstances: save, persistence-worker drain, save
		persistCloseTimeout +
		shutdownSaveTimeout + // items_on_ground
		2*olympiad.TaskTimeout + // the Olympiad's queued olympiad_nobles and server_memo writes, then its final save
		shutdownSaveTimeout + // grandboss_list
		shutdownSaveTimeout + // buffer_schemes
		shutdownSaveTimeout + // mods_wedding
		fishchamp.TaskTimeout + shutdownSaveTimeout + // fishing_championship and its server_memo end, behind a save already running
		castlemanor.TaskTimeout + shutdownSaveTimeout + // castle_manor_production and castle_manor_procure, behind a save already running
		gameServerStopSlack
	// gameServerBootTimeout bounds constructor-time DB I/O (id scan, ground-item
	// restore, spawn-state load). These run inside fx.New's constructor graph,
	// before fx.StartTimeout applies and before Run installs signal handling,
	// so with context.Background() a stuck database hung the process with no
	// way to interrupt it. idfactory.New alone runs six full-table scans plus
	// dozens of orphan-cleanup DELETEs against unindexed owner columns, so the
	// budget is minutes rather than seconds: generous enough that only a
	// genuinely hung database ever trips it, not a large but healthy one.
	gameServerBootTimeout = 5 * time.Minute
	// gameServerStartTimeout bounds the OnStart phase, which fx runs with one
	// shared context across every hook: pool.PingContext, sevensigns.State's
	// Restore, clearing the previous shutdown's items_on_ground snapshot
	// (startGroundItems), and the game listener bind. fx's 15s default was
	// sized before any of these touched the database; explicit and separate
	// from gameServerBootTimeout so this budget can be tuned on its own.
	gameServerStartTimeout = 30 * time.Second
)

// bootContext is the constructor-time I/O deadline. Named so fx cannot inject
// it into an unrelated context.Context parameter.
type bootContext struct{ context.Context }

func provideBootContext(lc fx.Lifecycle) bootContext {
	ctx, cancel := context.WithTimeout(context.Background(), gameServerBootTimeout)
	lc.Append(fx.Hook{OnStop: func(context.Context) error {
		cancel()
		return nil
	}})
	return bootContext{ctx}
}

type gameServerPaths struct {
	ConfigPath        string
	LoggingPath       string
	PlayersConfigPath string
	HexIDPath         string
	GeoConfigPath     string
	NpcsConfigPath    string
	ClansConfigPath   string
	EventsConfigPath  string
	SiegeConfigPath   string
	DataRoot          string
	LogRoot           string
	DebugAddr         string
}

func main() {
	paths := parseGameServerFlags()
	newGameServerApp(paths).Run()
}

func parseGameServerFlags() gameServerPaths {
	var paths gameServerPaths
	flag.StringVar(&paths.ConfigPath, "config", "config/server.properties", "game server properties file")
	flag.StringVar(&paths.LoggingPath, "logging", "config/logging.properties", "logging properties file")
	flag.StringVar(&paths.PlayersConfigPath, "players-config", "config/players.properties", "player properties file")
	flag.StringVar(&paths.HexIDPath, "hexid", "config/hexid.txt", "game server hexid properties file")
	flag.StringVar(&paths.GeoConfigPath, "geo-config", "config/geoengine.properties", "geoengine properties file")
	flag.StringVar(&paths.NpcsConfigPath, "npcs-config", "config/npcs.properties", "npc properties file")
	flag.StringVar(&paths.ClansConfigPath, "clans-config", "config/clans.properties", "clan properties file")
	flag.StringVar(&paths.EventsConfigPath, "events-config", "config/events.properties", "events properties file")
	flag.StringVar(&paths.SiegeConfigPath, "siege-config", "config/siege.properties", "siege properties file")
	flag.StringVar(&paths.DataRoot, "data-root", ".", "datapack root containing data/xml")
	flag.StringVar(&paths.LogRoot, "log-root", ".", "root directory for log files")
	flag.StringVar(&paths.DebugAddr, "debug-addr", "", "optional host:port serving pprof and expvar")
	flag.Parse()
	return paths
}

func newGameServerApp(paths gameServerPaths) *fx.App {
	return fx.New(newGameServerAppOptions(paths)...)
}

// newGameServerAppOptions is the fx option list for the game server's
// constructor graph, split out from newGameServerApp so fx.ValidateApp can
// check it resolves without a database (see TestGameServerGraphValidates).
func newGameServerAppOptions(paths gameServerPaths) []fx.Option {
	return []fx.Option{
		fx.StopTimeout(gameServerStopTimeout),
		fx.StartTimeout(gameServerStartTimeout),
		fx.Supply(paths),
		fx.Provide(
			provideBootContext,
			loadGameServerProperties,
			loadGameplayConfig,
			loadPvPFlagOptions,
			loadPetConfig,
			loadHexIDProperties,
			gameServerConfigFromLoadedProperties,
			provideGameServerLogger,
			provideGameServerDatabase,
			providePersist,
			provideItemWriteOrder,
			loadHTMLCache,
			loadCrestCache,
			loadGameData,
			gamesql.NewCharacterStore,
			gamesql.NewItemStore,
			gamesql.NewShortcutStore,
			gamesql.NewHennaStore,
			gamesql.NewRecipeBookStore,
			gamesql.NewMacroStore,
			gamesql.NewRecommendationStore,
			gamesql.NewQuestStore,
			gamesql.NewMemoStore,
			gamesql.NewSubclassStore,
			gamesql.NewPetStore,
			provideIDAllocator,
			loadClanConfig,
			provideClans,
			provideCastles,
			provideSieges,
			provideRoster,
			providePvPFlags,
			provideInventoryUpdates,
			provideItemInstances,
			provideWorldState,
			provideSimPool,
			provideTaskEffects,
			provideGroundItemOptions,
			provideGroundItems,
			provideGameClock,
			provideSevenSignsState,
			provideFestival,
			loadSignsPriestConfig,
			provideHeroes,
			provideOlympiad,
			provideRaidPoints,
			provideCursedWeapons,
			provideWalker,
			provideWater,
			provideShadowItems,
			provideAutosave,
			provideDecay,
			provideAttackStance,
			provideDoorTask,
			provideDoorRegen,
			provideWorldObjects,
			provideBoats,
			provideSpawns,
			provideRespawnTask,
			provideAI,
			providePositionUpdates,
			provideEffects,
			provideEffectEnv,
			provideNPCRegen,
			provideKillRewardConfig,
			provideSpellbookPolicy,
			provideNpcs,
			provideScripts,
			provideQuestJournals,
			provideMakers,
			network.NewSessionValidator,
			provideLoginLinkState,
			provideSkillPersistence,
			providePlayerClock,
			provideMerchant,
			provideRelations,
			providePetitions,
			provideCommunityBoard,
			provideAnnouncements,
			provideSchemeBuffer,
			provideWedding,
			provideLottery,
			provideFishingChampionship,
			provideDerbyTrack,
			provideClanHallFunctions,
			provideClanHalls,
			provideCastleManor,
			provideDataReloads,
			provideGameClientLink,
		),
		fx.Invoke(buildScripts),
		fx.Invoke(startClanDissolutions, startClanHallFunctions, startClanHalls, startSieges, startCastleManor),
		fx.Invoke(startPvPFlags, startGroundItems, startGroundItemPersistence, startPlayerClock, startGameClock, startSevenSigns, startWalker, startWater, startShadowItems, startAutosave, startDecay, startAttackStance, startDoorTask, startDoorRegen, startWorldObjects, startBoats, startRespawnTask, startAI, startPositionUpdates, startInventoryUpdates, startItemInstances, startBuyListRestock, startSimPool, startEffects, startNPCRegen, startNpcs, startObserverTowers, startNpcPersistence, startRelationPersistence, startPetitionPersistence, startAnnouncements, startHeroes, startOlympiad, startRaidPoints, startCursedWeapons, startBossZones, startSchemeBuffer, startWedding, startLottery, startFishingChampionship, startDerbyTrack, startDebugHTTP, startSchedule, startGameServer),
	}
}
