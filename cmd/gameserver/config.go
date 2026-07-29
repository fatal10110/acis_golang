package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"

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

const generatedHexIDSize = 16

type gameServerConfig struct {
	ListenAddr         string
	LoginAddr          string
	Auth               network.LoginServerAuth
	GeneratedHexID     bool
	HexIDPath          string
	Database           db.Config
	AllowCursedWeapons bool
}

func loadGameServerProperties(paths gameServerPaths) (*config.Properties, error) {
	return config.LoadFile(paths.ConfigPath)
}

// respawnRestoreHP is the fraction of calculated max HP a non-percent
// revive restores, read from players.properties.
type respawnRestoreHP float64

func loadRespawnRestoreHP(paths gameServerPaths) (respawnRestoreHP, error) {
	props, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return 0, err
	}
	return respawnRestoreHP(config.NewFields(props, "respawn restore hp").Float64("RespawnRestoreHP", 0.7)), nil
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
		if errors.Is(err, os.ErrNotExist) {
			return hexIDProperties{}, nil
		}
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
	loginPort, err := serverProps.Int("LoginPort", 9014)
	if err != nil {
		return gameServerConfig{}, err
	}
	requestID, err := serverProps.Int("RequestServerID", 0)
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

	serverID := requestID
	generated := hexProps == nil
	hexID, err := generatedHexID()
	if err != nil {
		return gameServerConfig{}, err
	}
	if hexProps != nil {
		serverID, err = hexProps.Int("ServerID", requestID)
		if err != nil {
			return gameServerConfig{}, err
		}
		hexID, err = model.ParseHexKey(hexProps.String("HexID", "0"))
		if err != nil {
			return gameServerConfig{}, err
		}
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
	ageLimit := int32(serverListAgeLimit)
	testServer := serverProps.Bool("TestServer", false)
	pvpServer := serverProps.Bool("PvpServer", true)
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
		GeneratedHexID: generated,
		HexIDPath:      paths.HexIDPath,
		Database: db.Config{
			URL:      serverProps.String("URL", "jdbc:mariadb://localhost/acis"),
			Login:    serverProps.String("Login", "root"),
			Password: serverProps.String("Password", ""),
		},
		AllowCursedWeapons: serverProps.Bool("AllowCursedWeapons", true),
	}, nil
}

func listenAddress(host string, port int) string {
	if host == "*" {
		host = ""
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func generatedHexID() ([]byte, error) {
	key := make([]byte, generatedHexIDSize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate hexid: %w", err)
	}
	return key, nil
}

func writeHexIDFile(path string, serverID int, hexID []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create hexid directory: %w", err)
	}
	data := fmt.Sprintf("#the hexID to auth into login\nServerID=%d\nHexID=%s\n", serverID, model.HexKeyText(hexID))
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		return fmt.Errorf("write hexid file: %w", err)
	}
	return nil
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
