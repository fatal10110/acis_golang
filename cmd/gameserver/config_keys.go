package main

import (
	"fmt"
	"math"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/config"
)

// dropItemDelay is the reuse delay between two item drops on one client
// session, read from server.properties.
type dropItemDelay time.Duration

func loadDropItemDelay(paths gameServerPaths) (dropItemDelay, error) {
	props, err := config.LoadFile(paths.ConfigPath)
	if err != nil {
		return 0, err
	}
	fields := config.NewFields(props, "drop item reuse delay")
	millis := fields.Int("DropItemTime", 1000)
	if err := fields.Err(); err != nil {
		return 0, err
	}
	if millis < math.MinInt32 || millis > math.MaxInt32 {
		return 0, fmt.Errorf("DropItemTime %d outside int32 range", millis)
	}
	return dropItemDelay(time.Duration(millis) * time.Millisecond), nil
}

// warehouseAllowed is server.properties AllowWarehouse: when false, a
// warehouse deposit request is ignored.
type warehouseAllowed bool

func loadWarehouseAllowed(paths gameServerPaths) (warehouseAllowed, error) {
	props, err := config.LoadFile(paths.ConfigPath)
	if err != nil {
		return false, err
	}
	return warehouseAllowed(config.NewFields(props, "allow warehouse").Bool("AllowWarehouse", true)), nil
}

// noSpawns is server.properties NoSpawns: when true, the spawn pass at boot
// and on //respawnall spawns nothing.
type noSpawns bool

func loadNoSpawns(paths gameServerPaths) (noSpawns, error) {
	props, err := config.LoadFile(paths.ConfigPath)
	if err != nil {
		return false, err
	}
	return noSpawns(config.NewFields(props, "no spawns").Bool("NoSpawns", false)), nil
}

// fakeDeathUpProtection is how long, after a Fake Death ends, hostile NPCs
// do not aggro the player: players.properties PlayerFakeDeathUpProtection.
type fakeDeathUpProtection time.Duration

func loadFakeDeathUpProtection(paths gameServerPaths) (fakeDeathUpProtection, error) {
	props, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return 0, err
	}
	fields := config.NewFields(props, "fake death up protection")
	seconds := fields.Int("PlayerFakeDeathUpProtection", 5)
	if err := fields.Err(); err != nil {
		return 0, err
	}
	return fakeDeathUpProtectionFromSeconds(seconds)
}

// fakeDeathUpProtectionFromSeconds converts the configured seconds to the
// grace. The milliseconds are an int32 product, so a value past 2147483
// seconds wraps, possibly negative, which leaves no grace at all; a value
// outside int32 is malformed.
func fakeDeathUpProtectionFromSeconds(seconds int) (fakeDeathUpProtection, error) {
	if seconds < math.MinInt32 || seconds > math.MaxInt32 {
		return 0, fmt.Errorf("PlayerFakeDeathUpProtection %d outside int32 range", seconds)
	}
	millis := int32(seconds) * 1000
	return fakeDeathUpProtection(time.Duration(millis) * time.Millisecond), nil
}

// summonCrestShown is npcs.properties ShowSummonCrest: when true, a
// summon's NpcInfo shows its owner's clan and alliance crests, and its owner
// is sent that NpcInfo too when the summon spawns or lands from a teleport.
type summonCrestShown bool

func loadShowSummonCrest(paths gameServerPaths) (summonCrestShown, error) {
	props, err := config.LoadFile(paths.NpcsConfigPath)
	if err != nil {
		return false, err
	}
	return summonCrestShown(config.NewFields(props, "show summon crest").Bool("ShowSummonCrest", false)), nil
}

// reportConfigKeys logs, once per key, every key of the server's
// properties files that it does not read (config.ReportUnreadKeys).
func reportConfigKeys(paths gameServerPaths, log zerolog.Logger) error {
	for _, f := range []struct{ name, path string }{
		{"clans.properties", paths.ClansConfigPath},
		{"events.properties", paths.EventsConfigPath},
		{"geoengine.properties", paths.GeoConfigPath},
		{"npcs.properties", paths.NpcsConfigPath},
		{"players.properties", paths.PlayersConfigPath},
		{"server.properties", paths.ConfigPath},
		{"siege.properties", paths.SiegeConfigPath},
	} {
		props, err := config.LoadFile(f.path)
		if err != nil {
			return err
		}
		config.ReportUnreadKeys(log, config.GameServer, f.name, props)
	}
	return nil
}
