// Command gameserver boots the game server process.
package main

import (
	"flag"

	"go.uber.org/fx"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
)

type gameServerPaths struct {
	ConfigPath        string
	LoggingPath       string
	PlayersConfigPath string
	HexIDPath         string
	GeoConfigPath     string
	DataRoot          string
	LogRoot           string
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
	flag.StringVar(&paths.DataRoot, "data-root", ".", "datapack root containing data/xml")
	flag.StringVar(&paths.LogRoot, "log-root", ".", "root directory for log files")
	flag.Parse()
	return paths
}

func newGameServerApp(paths gameServerPaths) *fx.App {
	return fx.New(
		fx.Supply(paths),
		fx.Provide(
			loadGameServerProperties,
			loadPvPFlagOptions,
			loadRespawnRestoreHP,
			loadSkillEnchantSPBookNeeded,
			loadKarmaPlayerCanTeleport,
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
			provideIDAllocator,
			provideRoster,
			providePvPFlags,
			provideInventoryUpdates,
			provideWorldState,
			provideGroundItemOptions,
			provideGroundItems,
			provideGameClock,
			provideWalker,
			provideWater,
			provideShadowItems,
			provideDecay,
			provideAttackStance,
			provideWorldObjects,
			provideSpawns,
			provideRespawnTask,
			provideAI,
			providePositionUpdates,
			provideEffects,
			provideKillRewardConfig,
			provideSpellbookPolicy,
			provideNpcs,
			network.NewSessionValidator,
			provideLoginLinkState,
			provideSkillPersistence,
			providePlayerClock,
			provideGameClientLink,
		),
		fx.Invoke(startPvPFlags, startGroundItems, startGroundItemPersistence, startPlayerClock, startGameClock, startWalker, startWater, startShadowItems, startDecay, startAttackStance, startWorldObjects, startRespawnTask, startAI, startPositionUpdates, startInventoryUpdates, startEffects, startNpcs, startNpcPersistence, startGameServer),
	)
}
