package main

import (
	"context"
	"path/filepath"

	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/rs/zerolog"
)

// provideDataReloads builds the //reload and //respawnall hooks: each reads
// the same files the boot read, and swaps the loaded table into the one the
// server already holds, so every holder sees the new data at once. A file
// that cannot be read leaves the table as it was.
func provideDataReloads(paths gameServerPaths, data *gameData, html *datacache.HTML, crests *datacache.Crests, walker *task.Walker, spawnStore *gamesql.SpawnStore, gameplay gameplayConfig, log zerolog.Logger) network.DataReloads {
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
		// CrestCache.reload never fails: what was read before a bad file
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
		HTML: func() error {
			fresh, err := loadHTMLCache(paths)
			if err != nil {
				return err
			}
			html.Replace(fresh)
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
		// SpawnManager.reload: the save failing is logged and the load
		// goes on, as in the reference.
		SpawnList: func(ctx context.Context, current *manager.Spawns) (*manager.Spawns, error) {
			if err := current.Save(ctx, spawnStore); err != nil {
				log.Warn().Err(err).Msg("save spawn data before the spawn list reload")
			}
			return manager.LoadSpawns(ctx, filepath.Join(xmlRoot, "spawnlist"), spawnStore, log, float64(gameplay.SpawnMultiplier))
		},
	}
}
