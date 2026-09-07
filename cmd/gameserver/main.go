// Command gameserver boots the game server process.
package main

import (
	"context"
	"flag"
	"time"

	"go.uber.org/fx"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
)

const (
	// gameServerStopTimeout bounds the whole shutdown sequence, which runs every
	// stop hook in turn: draining connections, persisting ground items and npcs,
	// and finally flushing pending item rows. fx's 15s default leaves that last
	// flush able to be cut short by the process exiting rather than by its own
	// item-instance save timeout budget.
	gameServerStopTimeout = 30 * time.Second
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
			loadHTMLCache,
			loadCrestCache,
			loadGameData,
			gamesql.NewCharacterStore,
			gamesql.NewItemStore,
			gamesql.NewShortcutStore,
			gamesql.NewHennaStore,
			gamesql.NewPetStore,
			provideIDAllocator,
			provideRoster,
			providePvPFlags,
			provideInventoryUpdates,
			provideItemInstances,
			provideWorldState,
			provideTaskEffects,
			provideGroundItemOptions,
			provideGroundItems,
			provideGameClock,
			provideSevenSignsState,
			provideWalker,
			provideWater,
			provideShadowItems,
			provideAutosave,
			provideDecay,
			provideAttackStance,
			provideDoorTask,
			provideWorldObjects,
			provideSpawns,
			provideRespawnTask,
			provideAI,
			providePositionUpdates,
			provideEffects,
			provideNPCRegen,
			provideKillRewardConfig,
			provideSpellbookPolicy,
			provideNpcs,
			network.NewSessionValidator,
			provideLoginLinkState,
			provideSkillPersistence,
			providePlayerClock,
			provideGameClientLink,
		),
		fx.Invoke(wireGameClock, startPvPFlags, startGroundItems, startGroundItemPersistence, startPlayerClock, startGameClock, startSevenSigns, startWalker, startWater, startShadowItems, startAutosave, startDecay, startAttackStance, startDoorTask, startWorldObjects, startRespawnTask, startAI, startPositionUpdates, startInventoryUpdates, startItemInstances, startEffects, startNPCRegen, startNpcs, startNpcPersistence, startDebugHTTP, startGameServer),
	}
}
