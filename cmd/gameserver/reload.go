package main

import (
	"context"
	"path/filepath"

	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/rs/zerolog"
)

// provideDataReloads builds the //reload and //respawnall hooks: each reads
// the same files the boot read, and swaps the loaded table into the one the
// server already holds, so every holder sees the new data at once. A file
// that cannot be read leaves the table as it was.
func provideDataReloads(paths gameServerPaths, cfg gameServerConfig, data *gameData, html *datacache.HTML, crests *datacache.Crests, walker *task.Walker, spawnStore *gamesql.SpawnStore, gameplay gameplayConfig, log zerolog.Logger) network.DataReloads {
	xmlRoot := filepath.Join(paths.DataRoot, "data", "xml")
	return network.DataReloads{
		Admin: func() error {
			fresh, err := gamexml.LoadAdminData(xmlRoot)
			if err != nil {
				return err
			}
			data.Admin.Replace(fresh)
			return nil
		},
		Config: func() (network.PlayerConfig, error) {
			return reloadPlayerConfig(paths, data, log)
		},
		BoatRoutes: func() ([]route.BoatItinerary, error) {
			return loadBoatRoutes(paths)
		},
		BuyLists: func() (*buylist.Table, error) {
			return gamexml.LoadBuyLists(filepath.Join(xmlRoot, "buyLists.xml"), data.Items)
		},
		// A crest reload never fails: what was read before a bad file
		// stays loaded, as at boot.
		Crests: func() error {
			fresh, err := loadCrestCache(paths, log)
			if err != nil {
				return err
			}
			crests.Replace(fresh)
			return nil
		},
		CursedWeapons: func() (*entity.CursedWeaponTable, error) {
			return gamexml.LoadCursedWeapons(filepath.Join(xmlRoot, "cursedWeapons.xml"), data.Skills)
		},
		Doors: func() (*door.Table, error) {
			return gamexml.LoadDoors(filepath.Join(xmlRoot, "doors.xml"), log)
		},
		HTML: func() error {
			fresh, err := loadHTMLCache(paths, log)
			if err != nil {
				return err
			}
			html.Replace(fresh)
			return nil
		},
		Items: func() error {
			fresh, err := gamexml.LoadItemTemplates(filepath.Join(xmlRoot, "items"), log)
			if err != nil {
				return err
			}
			data.Items.Replace(fresh)
			return nil
		},
		Multisells: func() error {
			fresh, err := gamexml.LoadMultiSellLists(filepath.Join(xmlRoot, "multisell"), data.Items)
			if err != nil {
				return err
			}
			data.Multisells.Replace(fresh)
			return nil
		},
		NPCs: func() error {
			fresh, err := gamexml.LoadNPCTemplates(filepath.Join(xmlRoot, "npcs"), data.Items, data.Skills, log)
			if err != nil {
				return err
			}
			data.NPCs.Replace(fresh)
			return nil
		},
		Skills: func() error {
			fresh, err := gamexml.LoadSkillDefinitions(filepath.Join(xmlRoot, "skills"), log)
			if err != nil {
				return err
			}
			logUnsupportedSkillEffects(fresh, log)
			data.Skills.Replace(fresh)
			return nil
		},
		WalkerRoutes: func() error {
			fresh, err := gamexml.LoadWalkerRoutes(filepath.Join(xmlRoot, "walkerRoutes.xml"))
			if err != nil {
				return err
			}
			walker.SetRoutes(fresh)
			return nil
		},
		Teleports: func() (travel.TeleportTable, travel.InstantTable, error) {
			teleports, err := gamexml.LoadTeleports(filepath.Join(xmlRoot, "teleports.xml"))
			if err != nil {
				return nil, nil, err
			}
			instants, err := gamexml.LoadInstantTeleports(filepath.Join(xmlRoot, "instantTeleports.xml"))
			if err != nil {
				return nil, nil, err
			}
			return teleports, instants, nil
		},
		// The save failing is logged and the load goes on.
		SpawnList: func(ctx context.Context, current *manager.Spawns) (*manager.Spawns, error) {
			if err := current.Save(ctx, spawnStore); err != nil {
				log.Warn().Err(err).Msg("save spawn data before the spawn list reload")
			}
			return manager.LoadSpawns(ctx, filepath.Join(xmlRoot, "spawnlist"), spawnStore, log, float64(gameplay.SpawnMultiplier))
		},
		Zones: func() (*zone.Index, error) {
			fresh, err := gamexml.LoadZones(filepath.Join(xmlRoot, "zones"))
			if err != nil {
				return nil, err
			}
			applyTownCombatRule(fresh, cfg.TownCombatRule)
			return fresh, nil
		},
	}
}

// reloadPlayerConfig reads the server, hexid, players, NPC, geoengine and
// events properties files anew, failing as boot would on a file or value it cannot
// read, and returns the player settings read from them.
//
// Only the player settings take the reloaded values, each group put in force
// whole: the rules of the characters and summons already in the world, the
// regeneration multipliers the characters, summons, NPCs and doors read, the
// kill reward settings (drop and XP/SP rates, auto-loot, party XP, the party
// range and the multiple item drop rule, shared by the kill rewards, party
// loot, NPC hate randomization, the scripts and item creation), the event
// settings the event quests read (the Four Sepulchers party size and the
// Dimensional Rift settings), the quest drop and reward rates the scripts
// read, the PvP flag times (a flag already running keeps its deadline), the
// MaxBuffsAmount and MaxDriftRange every NPC reads, those already spawned
// included, and ShowNpcLevel (an NpcInfo already sent keeps its title).
//
// These keep their boot value, read once into what the boot built from
// them:
//   - the listener and login server addresses, the login registration
//     (server id, hexid, status flags), the database and the client cipher:
//     the connections and the registration are made with them at boot;
//   - the geodata and the town combat rule the zones were set up with,
//     which //reload zone reapplies as booted;
//   - the calendar systems: the siege, Olympiad, hero, Seven Signs,
//     festival, manor (RateDropManor included), lottery, derby, wedding and
//     fishing championship settings. Each schedules its cycle at boot, and a
//     change in the middle of one could leave the schedule and the stored
//     state disagreeing;
//   - the rest of the long tail, each read when its service or system is
//     built and rarely changed on a running server: AllowCursedWeapons,
//     AllowBoat, DeleteCharAfterDays, NoSpawns (the spawn passes, //respawnall
//     included, keep the boot value), the ground item settings, the pet
//     settings, the spellbook needs, StoreSkillCooltime and
//     CancelLesserEffect, the NPC AI switches, see range and timings, the
//     random walk rate, the path fail count, the raid multipliers, the boss
//     wait times, the raid curse (characters included), the NPC and summon
//     crests (ShowNpcCrest, ShowSummonCrest), free teleport, the merchant
//     and augmentation settings, the chat settings, the GM audit and item
//     logs, the clan, scheme buffer, community board, petition and class
//     master settings, and the spawn multiplier and spawn events
//     (//respawnall reads the spawn list with the boot multiplier).
func reloadPlayerConfig(paths gameServerPaths, data *gameData, log zerolog.Logger) (network.PlayerConfig, error) {
	serverProps, err := loadGameServerProperties(paths, log)
	if err != nil {
		return network.PlayerConfig{}, err
	}
	hexProps, err := loadHexIDProperties(paths)
	if err != nil {
		return network.PlayerConfig{}, err
	}
	cfg, err := gameServerConfigFromLoadedProperties(paths, serverProps, hexProps)
	if err != nil {
		return network.PlayerConfig{}, err
	}
	gameplay, err := loadGameplayConfig(paths, log)
	if err != nil {
		return network.PlayerConfig{}, err
	}
	pvpOptions, err := loadPvPFlagOptions(paths, log)
	if err != nil {
		return network.PlayerConfig{}, err
	}
	rewards, err := loadKillRewardSettings(paths, serverProps)
	if err != nil {
		return network.PlayerConfig{}, err
	}
	events, err := loadEventRules(paths)
	if err != nil {
		return network.PlayerConfig{}, err
	}
	rates, err := loadQuestRates(serverProps)
	if err != nil {
		return network.PlayerConfig{}, err
	}
	settings := newPlayerConfig(cfg, gameplay, pvpOptions)
	settings.KillRewards = &rewards
	settings.Events = &events
	settings.QuestRates = &rates
	pvpTimes := pvpOptions.Times()
	settings.PvPFlagTimes = &pvpTimes
	limits := npcLimits(gameplay)
	settings.NPCLimits = &limits
	return settings, nil
}
