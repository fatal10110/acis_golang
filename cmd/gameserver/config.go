package main

import (
	"fmt"
	"math"
	"net"
	"strconv"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/db"
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
)

type gameServerConfig struct {
	ListenAddr         string
	LoginAddr          string
	Auth               network.LoginServerAuth
	Database           db.Config
	AllowCursedWeapons bool
	AllowWater         bool
	UseBlowfishCipher  bool
	TownCombatRule     int
}

func loadGameServerProperties(paths gameServerPaths) (*config.Properties, error) {
	return config.LoadFile(paths.ConfigPath)
}

// respawnRestoreHP is the fraction of calculated max HP a non-percent
// revive restores, read from players.properties.
type respawnRestoreHP float64

type deathPenaltyChance int

type playerSpawnProtection time.Duration

func loadPlayerSpawnProtection(paths gameServerPaths) (playerSpawnProtection, error) {
	props, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return 0, err
	}
	return playerSpawnProtection(time.Duration(config.NewFields(props, "player spawn protection").Int("PlayerSpawnProtection", 0)) * time.Second), nil
}

func loadRespawnRestoreHP(paths gameServerPaths) (respawnRestoreHP, error) {
	props, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return 0, err
	}
	return respawnRestoreHP(config.NewFields(props, "respawn restore hp").Float64("RespawnRestoreHP", 0.7)), nil
}

func loadDeathPenaltyChance(paths gameServerPaths) (deathPenaltyChance, error) {
	props, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return 0, err
	}
	return deathPenaltyChance(config.NewFields(props, "death penalty chance").Int("DeathPenaltyChance", 20)), nil
}

type maxBuffsAmount int

func loadMaxBuffsAmount(paths gameServerPaths) (maxBuffsAmount, error) {
	props, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return 0, err
	}
	return maxBuffsAmount(config.NewFields(props, "max buffs amount").Int("MaxBuffsAmount", 20)), nil
}

// perfectShieldBlockRate is the roll threshold (out of 100) below which a
// successful shield block upgrades to a perfect block, read from
// players.properties (Config.java:338,873).
type perfectShieldBlockRate int

func loadPerfectShieldBlockRate(paths gameServerPaths) (perfectShieldBlockRate, error) {
	props, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return 0, err
	}
	return perfectShieldBlockRate(config.NewFields(props, "perfect shield block rate").Int("PerfectShieldBlockRate", 5)), nil
}

// magicFailures is the players.properties MagicFailures switch: when true,
// magic-damage casts roll resist and may halve or flatten damage.
type magicFailures bool

func loadMagicFailures(paths gameServerPaths) (magicFailures, error) {
	props, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return false, err
	}
	return magicFailures(config.NewFields(props, "magic failures").Bool("MagicFailures", true)), nil
}

// cancelLesserEffect is the players.properties CancelLesserEffect switch:
// when true, a newly stacked non-herb effect removes the lower-priority
// effect it displaces.
type cancelLesserEffect bool

func loadCancelLesserEffect(paths gameServerPaths) (cancelLesserEffect, error) {
	props, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return false, err
	}
	return cancelLesserEffect(config.NewFields(props, "cancel lesser effect").Bool("CancelLesserEffect", true)), nil
}

// skillEnchantSPBookNeeded controls whether enchanting a skill above level
// 76 also consumes the tree's configured spellbook item, read from
// players.properties.
type skillEnchantSPBookNeeded bool

func loadSkillEnchantSPBookNeeded(paths gameServerPaths) (skillEnchantSPBookNeeded, error) {
	props, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return false, err
	}
	return skillEnchantSPBookNeeded(config.NewFields(props, "skill enchant sp book needed").Bool("EnchantSkillSpBookNeeded", true)), nil
}

type autoLearnSkills bool

type weightLimitMultiplier float64

func loadWeightLimitMultiplier(paths gameServerPaths) (weightLimitMultiplier, error) {
	props, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return 0, err
	}
	return weightLimitMultiplier(config.NewFields(props, "weight limit").Float64("WeightLimit", 1)), nil
}

func loadAutoLearnSkills(paths gameServerPaths) (autoLearnSkills, error) {
	props, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return false, err
	}
	return autoLearnSkills(config.NewFields(props, "auto learn skills").Bool("AutoLearnSkills", false)), nil
}

// karmaPlayerCanTeleport controls whether a karma-carrying player may use a
// TELEPORT/RECALL-type skill, direct or item-attached, read from
// players.properties.
type karmaPlayerCanTeleport bool

func loadKarmaPlayerCanTeleport(paths gameServerPaths) (karmaPlayerCanTeleport, error) {
	props, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return false, err
	}
	return karmaPlayerCanTeleport(config.NewFields(props, "karma player can teleport").Bool("KarmaPlayerCanTeleport", true)), nil
}

// allowDelevel controls whether a player death may cost experience/karma at
// all, read from players.properties.
type allowDelevel bool

func loadAllowDelevel(paths gameServerPaths) (allowDelevel, error) {
	props, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return false, err
	}
	return allowDelevel(config.NewFields(props, "allow delevel").Bool("AllowDelevel", true)), nil
}

// rateKarmaExpLost scales the death exp-loss percentage while the dying
// player carries positive karma, read from server.properties.
type rateKarmaExpLost float64

func loadRateKarmaExpLost(paths gameServerPaths) (rateKarmaExpLost, error) {
	props, err := config.LoadFile(paths.ConfigPath)
	if err != nil {
		return 0, err
	}
	return rateKarmaExpLost(config.NewFields(props, "rate karma exp lost").Float64("RateKarmaExpLost", 1)), nil
}

// characterSelectDelay is the reuse delay shared by one client's
// character-list actions (delete, restore, select), read from
// server.properties.
type characterSelectDelay time.Duration

func loadCharacterSelectDelay(paths gameServerPaths) (characterSelectDelay, error) {
	props, err := config.LoadFile(paths.ConfigPath)
	if err != nil {
		return 0, err
	}
	return characterSelectDelay(time.Duration(config.NewFields(props, "character select reuse delay").Int("CharacterSelectTime", 3000)) * time.Millisecond), nil
}

// serverBypassDelay is the reuse delay between two bypass commands on one
// client session, read from server.properties.
type serverBypassDelay time.Duration

func loadServerBypassDelay(paths gameServerPaths) (serverBypassDelay, error) {
	props, err := config.LoadFile(paths.ConfigPath)
	if err != nil {
		return 0, err
	}
	return serverBypassDelay(time.Duration(config.NewFields(props, "server bypass reuse delay").Int("ServerBypassTime", 100)) * time.Millisecond), nil
}

func loadPetConfig(paths gameServerPaths) (pet.Config, error) {
	serverProps, err := config.LoadFile(paths.ConfigPath)
	if err != nil {
		return pet.Config{}, err
	}
	playersProps, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return pet.Config{}, err
	}
	return pet.ConfigFromProperties(serverProps, playersProps)
}

// spawnMultiplier is Config.SPAWN_MULTIPLIER (Config.java:715), read from
// npcs.properties.
type spawnMultiplier float64

func loadSpawnMultiplier(paths gameServerPaths) (spawnMultiplier, error) {
	props, err := config.LoadFile(paths.NpcsConfigPath)
	if err != nil {
		return 0, err
	}
	return spawnMultiplier(config.NewFields(props, "spawn multiplier").Float64("SpawnMultiplier", 1)), nil
}

// randomWalkRate is Config.RANDOM_WALK_RATE (Config.java:753), read from
// npcs.properties RandomWalkRate.
type randomWalkRate int

func loadRandomWalkRate(paths gameServerPaths) (randomWalkRate, error) {
	props, err := config.LoadFile(paths.NpcsConfigPath)
	if err != nil {
		return 0, err
	}
	return randomWalkRate(config.NewFields(props, "random walk rate").Int("RandomWalkRate", 30)), nil
}

// raidCursesDisabled is Config.RAID_DISABLE_CURSE (Config.java:746), read from
// npcs.properties DisableRaidCurse.
type raidCursesDisabled bool

func loadDisableRaidCurse(paths gameServerPaths) (raidCursesDisabled, error) {
	props, err := config.LoadFile(paths.NpcsConfigPath)
	if err != nil {
		return false, err
	}
	return raidCursesDisabled(config.NewFields(props, "disable raid curse").Bool("DisableRaidCurse", false)), nil
}

func loadPvPFlagOptions(paths gameServerPaths) (task.PvPFlagOptions, error) {
	props, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return task.PvPFlagOptions{}, err
	}
	return task.PvPFlagOptionsFromProperties(props)
}

type hexIDProperties struct {
	Props *config.Properties
}

func loadHexIDProperties(paths gameServerPaths) (hexIDProperties, error) {
	props, err := config.LoadFile(paths.HexIDPath)
	if err != nil {
		return hexIDProperties{}, err
	}
	return hexIDProperties{Props: props}, nil
}

func gameServerConfigFromLoadedProperties(paths gameServerPaths, serverProps *config.Properties, hexProps hexIDProperties) (gameServerConfig, error) {
	return gameServerConfigFromProperties(paths, serverProps, hexProps.Props)
}

func gameServerConfigFromProperties(paths gameServerPaths, serverProps, hexProps *config.Properties) (gameServerConfig, error) {
	listenPort, err := serverProps.Int("GameserverPort", 7777)
	if err != nil {
		return gameServerConfig{}, err
	}
	if listenPort < 0 || listenPort > math.MaxUint16 {
		return gameServerConfig{}, fmt.Errorf("GameserverPort %d outside uint16 range", listenPort)
	}
	loginPort, err := serverProps.Int("LoginPort", 9014)
	if err != nil {
		return gameServerConfig{}, err
	}
	maxPlayers, err := serverProps.Int64("MaximumOnlineUsers", 100)
	if err != nil {
		return gameServerConfig{}, err
	}
	if maxPlayers < math.MinInt32 || maxPlayers > math.MaxInt32 {
		return gameServerConfig{}, fmt.Errorf("MaximumOnlineUsers %d outside int32 range", maxPlayers)
	}

	if hexProps == nil {
		return gameServerConfig{}, fmt.Errorf("missing hexid properties")
	}
	serverIDText, ok := hexProps.Lookup("ServerID")
	if !ok {
		return gameServerConfig{}, fmt.Errorf("missing ServerID in hexid file")
	}
	serverID, err := strconv.Atoi(serverIDText)
	if err != nil {
		return gameServerConfig{}, err
	}
	hexIDText, ok := hexProps.Lookup("HexID")
	if !ok {
		return gameServerConfig{}, fmt.Errorf("missing HexID in hexid file")
	}
	hexID, err := model.ParseHexKey(hexIDText)
	if err != nil {
		return gameServerConfig{}, err
	}

	host := serverProps.String("Hostname", "*")
	statusType := link.ServerTypeAuto
	if serverProps.Bool("ServerGMOnly", false) {
		statusType = link.ServerTypeGMOnly
	}
	showClock := serverProps.Bool("ServerListClock", false)
	showBrackets := serverProps.Bool("ServerListBrackets", false)
	serverListAgeLimit, err := serverProps.Int("ServerListAgeLimit", 0)
	if err != nil {
		return gameServerConfig{}, err
	}
	if serverListAgeLimit < math.MinInt32 || serverListAgeLimit > math.MaxInt32 {
		return gameServerConfig{}, fmt.Errorf("ServerListAgeLimit %d outside int32 range", serverListAgeLimit)
	}
	ageLimit := int32(serverListAgeLimit)
	testServer := serverProps.Bool("TestServer", false)
	pvpServer := serverProps.Bool("PvpServer", true)
	townCombatRule, err := serverProps.Int("ZoneTown", 0)
	if err != nil {
		return gameServerConfig{}, err
	}
	return gameServerConfig{
		ListenAddr: listenAddress(serverProps.String("GameserverHostname", "*"), listenPort),
		LoginAddr:  net.JoinHostPort(serverProps.String("LoginHost", "127.0.0.1"), strconv.Itoa(loginPort)),
		Auth: network.LoginServerAuth{
			ServerID:          serverID,
			AcceptAlternateID: serverProps.Bool("AcceptAlternateID", true),
			HexID:             hexID,
			HostName:          host,
			Port:              uint16(listenPort),
			MaxPlayers:        int32(maxPlayers),
			InitialStatus: link.ServerStatus{
				Status:       &statusType,
				ShowClock:    &showClock,
				ShowBrackets: &showBrackets,
				AgeLimit:     &ageLimit,
				TestServer:   &testServer,
				Pvp:          &pvpServer,
			},
		},
		Database: db.Config{
			URL:      serverProps.String("URL", "jdbc:mariadb://localhost/acis"),
			Login:    serverProps.String("Login", "root"),
			Password: serverProps.String("Password", ""),
		},
		AllowCursedWeapons: serverProps.Bool("AllowCursedWeapons", true),
		AllowWater:         serverProps.Bool("AllowWater", true),
		UseBlowfishCipher:  serverProps.Bool("UseBlowfishCipher", true),
		TownCombatRule:     townCombatRule,
	}, nil
}

func listenAddress(host string, port int) string {
	if host == "*" {
		host = ""
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func provideGroundItemOptions(props *config.Properties) (task.GroundItemOptions, error) {
	return task.GroundItemOptionsFromProperties(props)
}

func provideKillRewardConfig(paths gameServerPaths, serverProps *config.Properties, data *gameData) (manager.KillRewardConfig, error) {
	playersProps, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return manager.KillRewardConfig{}, err
	}

	currencyKey := "RateDropCurrency"
	if _, ok := serverProps.Lookup(currencyKey); !ok {
		currencyKey = "RateDropAdena"
	}
	currency, err := serverProps.Float64(currencyKey, 1)
	if err != nil {
		return manager.KillRewardConfig{}, err
	}
	spoil, err := serverProps.Float64("RateDropSpoil", 1)
	if err != nil {
		return manager.KillRewardConfig{}, err
	}
	items, err := serverProps.Float64("RateDropItems", 1)
	if err != nil {
		return manager.KillRewardConfig{}, err
	}
	raidItems, err := serverProps.Float64("RateRaidDropItems", 1)
	if err != nil {
		return manager.KillRewardConfig{}, err
	}
	herbs, err := serverProps.Float64("RateDropHerbs", 1)
	if err != nil {
		return manager.KillRewardConfig{}, err
	}
	partyRange, err := playersProps.Int("PartyRange", 1500)
	if err != nil {
		return manager.KillRewardConfig{}, err
	}

	return manager.KillRewardConfig{
		Rates: item.Rates{
			Spoil:    spoil,
			Currency: currency,
			Item:     items,
			ItemRaid: raidItems,
			Herb:     herbs,
		},
		AutoLoot:          serverProps.Bool("AutoLoot", false),
		AutoLootRaid:      serverProps.Bool("AutoLootRaid", false),
		AutoLootHerbs:     serverProps.Bool("AutoLootHerbs", false),
		DeepBlueDropRules: playersProps.Bool("UseDeepBlueDropRules", true),
		PlayerLevels:      data.Levels,
		PartyRange:        partyRange,
	}, nil
}

func provideSpellbookPolicy(paths gameServerPaths, data *gameData) (skill.BookPolicy, error) {
	props, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return skill.BookPolicy{}, err
	}
	return skill.BookPolicy{
		Table:            data.Spellbooks,
		SPBookNeeded:     props.Bool("SpBookNeeded", true),
		DivineBookNeeded: props.Bool("DivineInspirationSpBookNeeded", true),
	}, nil
}
