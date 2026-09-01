// Command gameserver boots the game server process.
package main

import (
	"flag"
	"time"

	"go.uber.org/fx"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
)

// gameServerStopTimeout bounds the whole shutdown sequence, which runs every
// stop hook in turn: draining connections, persisting ground items and npcs,
// and finally flushing pending item rows. fx's 15s default leaves that last
// flush able to be cut short by the process exiting rather than by its own
// itemInstanceShutdownSaveTimeout budget.
const gameServerStopTimeout = 30 * time.Second

type gameServerPaths struct {
	ConfigPath        string
	LoggingPath       string
	PlayersConfigPath string
	HexIDPath         string
	GeoConfigPath     string
	NpcsConfigPath    string
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
	flag.StringVar(&paths.NpcsConfigPath, "npcs-config", "config/npcs.properties", "npc properties file")
	flag.StringVar(&paths.DataRoot, "data-root", ".", "datapack root containing data/xml")
	flag.StringVar(&paths.LogRoot, "log-root", ".", "root directory for log files")
	flag.Parse()
	return paths
}

func newGameServerApp(paths gameServerPaths) *fx.App {
	return fx.New(
		fx.StopTimeout(gameServerStopTimeout),
		fx.Supply(paths),
		fx.Provide(
			loadGameServerProperties,
			loadPvPFlagOptions,
			loadRespawnRestoreHP,
			loadDeathPenaltyChance,
			loadMaxBuffsAmount,
			loadPerfectShieldBlockRate,
			loadMagicFailures,
			loadCancelLesserEffect,
			loadPlayerSpawnProtection,
			loadSkillEnchantSPBookNeeded,
			loadAutoLearnSkills,
			loadWeightLimitMultiplier,
			loadKarmaPlayerCanTeleport,
			loadAllowDelevel,
			loadRateKarmaExpLost,
			loadCharacterSelectDelay,
			loadServerBypassDelay,
			loadPetConfig,
			loadSpawnMultiplier,
			loadRandomWalkRate,
			loadDisableRaidCurse,
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
		fx.Invoke(wireGameClock, startPvPFlags, startGroundItems, startGroundItemPersistence, startPlayerClock, startGameClock, startSevenSigns, startWalker, startWater, startShadowItems, startAutosave, startDecay, startAttackStance, startDoorTask, startWorldObjects, startRespawnTask, startAI, startPositionUpdates, startInventoryUpdates, startItemInstances, startEffects, startNPCRegen, startNpcs, startNpcPersistence, startGameServer),
	)
}
