package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatal10110/acis_golang/internal/config"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/pathfind"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/probe"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/rs/zerolog"
)

type gameData struct {
	Players       *player.TemplateTable
	Levels        *player.LevelTable
	Items         *item.Table
	Skills        *skill.Table
	Trees         *skill.Trees
	Spellbooks    *skill.SpellbookTable
	CursedWeapons *entity.CursedWeaponTable
	Zones         *zone.Index
	Routes        route.WalkerRoutes
	NPCs          *npc.Table
	Doors         *door.Table
	Statics       *staticobject.Table
	Restarts      *restart.Table
	Geo           *engine.Engine
	Finder        *pathfind.Finder
}

type geodata struct {
	Engine        *engine.Engine
	Finder        *pathfind.Finder
	Dir           string
	Type          probe.GeoType
	EngineOptions engine.Options
	Pathfind      pathfind.Options
}

func loadGameData(paths gameServerPaths, cfg gameServerConfig, log zerolog.Logger) (*gameData, error) {
	xmlRoot := filepath.Join(paths.DataRoot, "data", "xml")
	players, err := gamexml.LoadPlayerTemplates(filepath.Join(xmlRoot, "classes"))
	if err != nil {
		return nil, err
	}
	levels, err := gamexml.LoadPlayerLevels(filepath.Join(xmlRoot, "playerLevels.xml"))
	if err != nil {
		return nil, err
	}
	items, err := gamexml.LoadItemTemplates(filepath.Join(xmlRoot, "items"))
	if err != nil {
		return nil, err
	}
	skills, err := gamexml.LoadSkillDefinitions(filepath.Join(xmlRoot, "skills"))
	if err != nil {
		return nil, err
	}
	trees, err := gamexml.LoadSkillTrees(filepath.Join(xmlRoot, "skillstrees"))
	if err != nil {
		return nil, err
	}
	spellbooks, err := gamexml.LoadSpellbooks(filepath.Join(xmlRoot, "spellbooks.xml"))
	if err != nil {
		return nil, err
	}
	var cursedWeapons *entity.CursedWeaponTable
	if cfg.AllowCursedWeapons {
		cursedWeapons, err = gamexml.LoadCursedWeapons(filepath.Join(xmlRoot, "cursedWeapons.xml"), skills)
		if err != nil {
			return nil, err
		}
	}
	zones, err := gamexml.LoadZones(filepath.Join(xmlRoot, "zones"))
	if err != nil {
		return nil, err
	}
	routes, err := gamexml.LoadWalkerRoutes(filepath.Join(xmlRoot, "walkerRoutes.xml"))
	if err != nil {
		return nil, err
	}
	npcs, err := gamexml.LoadNPCTemplates(filepath.Join(xmlRoot, "npcs"), items, log)
	if err != nil {
		return nil, err
	}
	doors, err := gamexml.LoadDoors(filepath.Join(xmlRoot, "doors.xml"))
	if err != nil {
		return nil, err
	}
	statics, err := gamexml.LoadStaticObjects(filepath.Join(xmlRoot, "staticObjects.xml"))
	if err != nil {
		return nil, err
	}
	restarts, err := gamexml.LoadRestartPoints(filepath.Join(xmlRoot, "restartPointAreas.xml"))
	if err != nil {
		return nil, err
	}
	geo, err := loadGeodata(paths)
	if err != nil {
		return nil, err
	}
	log.Info().Str("geodata_dir", geo.Dir).Str("geodata_type", string(geo.Type)).Int("npc_templates", npcs.Len()).Int("skills", skills.Len()).Msg("game data loaded")
	return &gameData{
		Players:       players,
		Levels:        levels,
		Items:         items,
		Skills:        skills,
		Trees:         trees,
		Spellbooks:    spellbooks,
		CursedWeapons: cursedWeapons,
		Zones:         zones,
		Routes:        routes,
		NPCs:          npcs,
		Doors:         doors,
		Statics:       statics,
		Restarts:      restarts,
		Geo:           geo.Engine,
		Finder:        geo.Finder,
	}, nil
}

func loadHTMLCache(paths gameServerPaths) (*datacache.HTML, error) {
	return datacache.LoadHTML(filepath.Join(paths.DataRoot, "data", "html"))
}

func loadCrestCache(paths gameServerPaths) (*datacache.Crests, error) {
	crests, err := datacache.LoadCrests(filepath.Join(paths.DataRoot, "data", "crests"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return datacache.NewCrests(), nil
		}
		return nil, err
	}
	return crests, nil
}

func loadGeodata(paths gameServerPaths) (*geodata, error) {
	props, err := config.LoadFile(paths.GeoConfigPath)
	if err != nil {
		return nil, err
	}

	engineOptions, err := engine.OptionsFromProperties(props)
	if err != nil {
		return nil, err
	}
	pathOptions, err := pathfind.OptionsFromProperties(props)
	if err != nil {
		return nil, err
	}

	geo := &geodata{
		Dir:           resolveGeodataDir(paths.DataRoot, props.String("GeoDataPath", "")),
		Type:          probe.GeoType(props.String("GeoDataType", string(probe.L2OFF))),
		EngineOptions: engineOptions,
		Pathfind:      pathOptions,
	}
	geo.Engine, err = probe.LoadEngine(geo.Dir, geo.Type, geo.EngineOptions)
	if err != nil {
		return nil, err
	}
	geo.Finder = pathfind.New(geo.Engine, geo.Pathfind)
	return geo, nil
}

func resolveGeodataDir(dataRoot, configured string) string {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return filepath.Join(dataRoot, "data", "geodata")
	}
	if filepath.IsAbs(configured) {
		return configured
	}

	clean := filepath.Clean(configured)
	if clean == "data" || strings.HasPrefix(clean, "data"+string(os.PathSeparator)) {
		return filepath.Join(dataRoot, clean)
	}
	return clean
}
