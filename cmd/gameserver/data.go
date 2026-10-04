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
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/armorset"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/augmentation"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/multisell"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/recipe"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/rs/zerolog"
)

// logUnsupportedSkillEffects warns once per shipped skill effect template
// whose name effect.New doesn't recognize, so a coreKinds gap (#1517) is
// visible at boot instead of silently degrading a cast to a no-op the way
// an unlogged effect.New rejection does at apply/restore time. A name
// starting with "#" is the unresolved-<table> loader defect tracked by
// #1516, not a coreKinds gap, so it doesn't warn here.
func logUnsupportedSkillEffects(table *skill.Table, log zerolog.Logger) {
	check := func(def skill.Definition, effects []skill.EffectTemplate, kind string) {
		for _, eff := range effects {
			if strings.HasPrefix(eff.Name, "#") {
				continue
			}
			if _, err := effect.New(effect.SkillFromDefinition(def), eff); err != nil && errors.Is(err, effect.ErrUnsupportedCoreEffect) {
				log.Warn().
					Int("skill", int(def.ID)).
					Int("level", def.Level).
					Str("skillName", def.Name).
					Str("effect", eff.Name).
					Str("kind", kind).
					Msg("skill effect template names an unsupported core effect kind; casts using it apply nothing")
			}
		}
	}
	for _, def := range table.All() {
		check(def, def.Effects, "effect")
		check(def, def.SelfEffects, "self-effect")
	}
}

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
	SummonItems   *item.SummonItemTable
	Doors         *door.Table
	Statics       *staticobject.Table
	Restarts      *restart.Table
	// Teleports and InstantTeleports are the destinations civilian NPCs
	// offer.
	Teleports        travel.TeleportTable
	InstantTeleports travel.InstantTable
	Admin            *admin.Data
	Geo              *engine.Engine
	Finder           *pathfind.Finder
	Hennas           *henna.Table
	HealSps          *skill.HealSpsTable
	Recipes          *recipe.Table
	BuyLists         *buylist.Table
	Multisells       *multisell.Table
	Augmentations    *augmentation.Table
	ArmorSets        *armorset.Table
	// ClanHalls are the clan halls a clan can own.
	ClanHalls *clanhall.Table
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
	items, err := gamexml.LoadItemTemplates(filepath.Join(xmlRoot, "items"), log)
	if err != nil {
		return nil, err
	}
	skills, err := gamexml.LoadSkillDefinitions(filepath.Join(xmlRoot, "skills"), log)
	if err != nil {
		return nil, err
	}
	logUnsupportedSkillEffects(skills, log)
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
	applyTownCombatRule(zones, cfg.TownCombatRule)
	routes, err := gamexml.LoadWalkerRoutes(filepath.Join(xmlRoot, "walkerRoutes.xml"))
	if err != nil {
		return nil, err
	}
	npcs, err := gamexml.LoadNPCTemplates(filepath.Join(xmlRoot, "npcs"), items, skills, log)
	if err != nil {
		return nil, err
	}
	summonItems, err := gamexml.LoadSummonItems(filepath.Join(xmlRoot, "summonItems.xml"))
	if err != nil {
		return nil, err
	}
	doors, err := gamexml.LoadDoors(filepath.Join(xmlRoot, "doors.xml"), log)
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
	teleports, err := gamexml.LoadTeleports(filepath.Join(xmlRoot, "teleports.xml"))
	if err != nil {
		return nil, err
	}
	instantTeleports, err := gamexml.LoadInstantTeleports(filepath.Join(xmlRoot, "instantTeleports.xml"))
	if err != nil {
		return nil, err
	}
	adminData, err := gamexml.LoadAdminData(xmlRoot)
	if err != nil {
		return nil, err
	}
	hennas, err := gamexml.LoadHennas(filepath.Join(xmlRoot, "hennas.xml"))
	if err != nil {
		return nil, err
	}
	healSps, err := gamexml.LoadHealSps(filepath.Join(xmlRoot, "healSps.xml"))
	if err != nil {
		return nil, err
	}
	recipes, err := gamexml.LoadRecipes(filepath.Join(xmlRoot, "recipes.xml"))
	if err != nil {
		return nil, err
	}
	buyLists, err := gamexml.LoadBuyLists(filepath.Join(xmlRoot, "buyLists.xml"), items)
	if err != nil {
		return nil, err
	}
	multisells, err := gamexml.LoadMultiSellLists(filepath.Join(xmlRoot, "multisell"), items)
	if err != nil {
		return nil, err
	}
	augmentations, err := gamexml.LoadAugmentations(filepath.Join(xmlRoot, "augmentation"))
	if err != nil {
		return nil, err
	}
	armorSets, err := gamexml.LoadArmorSets(filepath.Join(xmlRoot, "armorSets.xml"))
	if err != nil {
		return nil, err
	}
	clanHalls, err := gamexml.LoadClanHalls(filepath.Join(xmlRoot, "clanHalls.xml"))
	if err != nil {
		return nil, err
	}
	geo, err := loadGeodata(paths, log)
	if err != nil {
		return nil, err
	}
	log.Info().Str("geodata_dir", geo.Dir).Str("geodata_type", string(geo.Type)).Int("npc_templates", npcs.Len()).Int("skills", skills.Len()).Int("hennas", hennas.Len()).Int("heal_sps", healSps.Count()).Int("recipes", recipes.Len()).Int("buylists", buyLists.Len()).Int("multisells", multisells.Count()).Int("augmentation_skills", augmentations.SkillCount()).Int("augmentation_stats", augmentations.StatCount()).Int("armor_sets", armorSets.Len()).Int("teleports", teleports.Count()).Int("instant_teleports", instantTeleports.Count()).Msg("game data loaded")
	return &gameData{
		Players:          players,
		Levels:           levels,
		Items:            items,
		Skills:           skills,
		Trees:            trees,
		Spellbooks:       spellbooks,
		CursedWeapons:    cursedWeapons,
		Zones:            zones,
		Routes:           routes,
		NPCs:             npcs,
		SummonItems:      summonItems,
		Doors:            doors,
		Statics:          statics,
		Restarts:         restarts,
		Teleports:        teleports,
		InstantTeleports: instantTeleports,
		Admin:            adminData,
		Geo:              geo.Engine,
		Finder:           geo.Finder,
		Hennas:           hennas,
		HealSps:          healSps,
		Recipes:          recipes,
		BuyLists:         buyLists,
		Multisells:       multisells,
		Augmentations:    augmentations,
		ArmorSets:        armorSets,
		ClanHalls:        clanHalls,
	}, nil
}

func applyTownCombatRule(index *zone.Index, rule int) {
	for _, town := range zone.OfKind[*zone.Town](index) {
		town.CombatRule = rule
	}
}

func loadHTMLCache(paths gameServerPaths) (*datacache.HTML, error) {
	return datacache.LoadHTML(filepath.Join(paths.DataRoot, "data", "html"))
}

// loadCrestCache loads the datapack crest images. As in the reference, a
// crest load that stops early keeps the crests read so far and the server
// boots on: the failure is logged at error level, each deleted invalid
// crest file is warned about, and the loaded count is reported. A missing
// crest directory leaves an empty cache that still saves new crests there.
func loadCrestCache(paths gameServerPaths, log zerolog.Logger) (*datacache.Crests, error) {
	dir := filepath.Join(paths.DataRoot, "data", "crests")
	crests, deleted, err := datacache.LoadCrests(dir)
	for _, name := range deleted {
		log.Warn().Str("crest", name).Msg("crest data is invalid; the crest file has been deleted")
	}
	if err != nil {
		if crests == nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		log.Error().Err(err).Msg("error loading crest files; clans whose crest was not loaded lose it")
		if crests == nil {
			crests = datacache.NewCrestsIn(dir)
		}
	}
	log.Info().Int("crests", crests.Len()).Msg("crests loaded")
	return crests, nil
}

func loadGeodata(paths gameServerPaths, log zerolog.Logger) (*geodata, error) {
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
	geo.Engine, err = probe.LoadEngine(geo.Dir, geo.Type, log, geo.EngineOptions)
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
