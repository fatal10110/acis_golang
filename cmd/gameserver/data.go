package main

import (
	"path/filepath"

	"github.com/fatal10110/acis_golang/internal/config"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/geodata"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/pathfind"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/armorset"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/augmentation"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/fish"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/multisell"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/observer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/recipe"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
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
	RoutesLoaded  []route.WalkerKey // the order Routes were loaded in
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
	GeoBugs          *engine.BugLog
	Hennas           *henna.Table
	HealSps          *skill.HealSpsTable
	Recipes          *recipe.Table
	BuyLists         *buylist.Table
	Multisells       *multisell.Table
	Augmentations    *augmentation.Table
	ArmorSets        *armorset.Table
	// Fish is the fish a cast fishing line draws from.
	Fish *fish.Table
	// ClanHalls are the clan halls a clan can own.
	ClanHalls *clanhall.Table
	// ClanHallDecos are the levels, fees and decorations of the functions
	// a clan hall can rent.
	ClanHallDecos *clanhall.DecoTable
	// Castles are the castles a clan can own.
	Castles *castledata.Table
	// Manors are the manor seed rows; ManorAreas the areas monsters are
	// sown in.
	Manors     *manor.Table
	ManorAreas *manor.AreaIndex
	// Observers are the broadcasting towers and the viewpoints they offer.
	Observers *observer.Table
	// SoulCrystals are the soul crystals and the monsters that charge
	// them.
	SoulCrystals *item.SoulCrystalTable
	// NewbieBuffs are the buffs the newbie guides cast.
	NewbieBuffs *skill.NewbieBuffTable
}

type loadedGeodata struct {
	Engine        *engine.Engine
	Finder        *pathfind.Finder
	Bugs          *engine.BugLog
	Dir           string
	Type          geodata.GeoType
	EngineOptions engine.Options
	Pathfind      pathfind.Options
}

// loadGameData reads every datapack table the game server needs, in a fixed
// order, failing boot at the first table that does not load.
func loadGameData(paths gameServerPaths, cfg gameServerConfig, log zerolog.Logger) (*gameData, error) {
	xmlRoot := filepath.Join(paths.DataRoot, "data", "xml")
	d := &gameData{}
	if err := loadCharacterData(d, xmlRoot, cfg, log); err != nil {
		return nil, err
	}
	if err := loadWorldData(d, xmlRoot, cfg, log); err != nil {
		return nil, err
	}
	if err := loadEconomyData(d, xmlRoot, log); err != nil {
		return nil, err
	}
	geo, err := loadGeodata(paths, log)
	if err != nil {
		return nil, err
	}
	d.Geo, d.Finder, d.GeoBugs = geo.Engine, geo.Finder, geo.Bugs
	log.Info().Str("geodata_dir", geo.Dir).Str("geodata_type", string(geo.Type)).Int("npc_templates", d.NPCs.Len()).Int("skills", d.Skills.Len()).Int("hennas", d.Hennas.Len()).Int("heal_sps", d.HealSps.Count()).Int("recipes", d.Recipes.Len()).Int("buylists", d.BuyLists.Len()).Int("multisells", d.Multisells.Count()).Int("augmentation_skills", d.Augmentations.SkillCount()).Int("augmentation_stats", d.Augmentations.StatCount()).Int("armor_sets", d.ArmorSets.Len()).Int("fish", d.Fish.Len()).Int("teleports", d.Teleports.Count()).Int("instant_teleports", d.InstantTeleports.Count()).Msg("game data loaded")
	return d, nil
}

// loadCharacterData reads the player, item and skill tables.
func loadCharacterData(d *gameData, xmlRoot string, cfg gameServerConfig, log zerolog.Logger) error {
	var err error
	if d.Players, err = gamexml.LoadPlayerTemplates(filepath.Join(xmlRoot, "classes")); err != nil {
		return err
	}
	if d.Levels, err = gamexml.LoadPlayerLevels(filepath.Join(xmlRoot, "playerLevels.xml")); err != nil {
		return err
	}
	if d.Items, err = gamexml.LoadItemTemplates(filepath.Join(xmlRoot, "items"), log); err != nil {
		return err
	}
	if d.Skills, err = gamexml.LoadSkillDefinitions(filepath.Join(xmlRoot, "skills"), log); err != nil {
		return err
	}
	if d.Trees, err = gamexml.LoadSkillTrees(filepath.Join(xmlRoot, "skillstrees")); err != nil {
		return err
	}
	if d.Spellbooks, err = gamexml.LoadSpellbooks(filepath.Join(xmlRoot, "spellbooks.xml")); err != nil {
		return err
	}
	if cfg.AllowCursedWeapons {
		if d.CursedWeapons, err = gamexml.LoadCursedWeapons(filepath.Join(xmlRoot, "cursedWeapons.xml"), d.Skills); err != nil {
			return err
		}
	}
	return nil
}

// loadWorldData reads the zones, NPCs, doors, teleports and other world
// placement tables; it needs the item and skill tables.
func loadWorldData(d *gameData, xmlRoot string, cfg gameServerConfig, log zerolog.Logger) error {
	var err error
	if d.Zones, err = gamexml.LoadZones(filepath.Join(xmlRoot, "zones")); err != nil {
		return err
	}
	applyTownCombatRule(d.Zones, cfg.TownCombatRule)
	if d.Routes, d.RoutesLoaded, err = gamexml.LoadWalkerRoutes(filepath.Join(xmlRoot, "walkerRoutes.xml")); err != nil {
		return err
	}
	if d.NPCs, err = gamexml.LoadNPCTemplates(filepath.Join(xmlRoot, "npcs"), d.Items, d.Skills, log); err != nil {
		return err
	}
	if d.SummonItems, err = gamexml.LoadSummonItems(filepath.Join(xmlRoot, "summonItems.xml")); err != nil {
		return err
	}
	if d.SoulCrystals, err = gamexml.LoadSoulCrystalData(filepath.Join(xmlRoot, "soulCrystals.xml")); err != nil {
		return err
	}
	if d.NewbieBuffs, err = gamexml.LoadNewbieBuffs(filepath.Join(xmlRoot, "newbieBuffs.xml")); err != nil {
		return err
	}
	if d.Doors, err = gamexml.LoadDoors(filepath.Join(xmlRoot, "doors.xml"), log); err != nil {
		return err
	}
	if d.Statics, err = gamexml.LoadStaticObjects(filepath.Join(xmlRoot, "staticObjects.xml")); err != nil {
		return err
	}
	if d.Restarts, err = gamexml.LoadRestartPoints(filepath.Join(xmlRoot, "restartPointAreas.xml")); err != nil {
		return err
	}
	if d.Teleports, err = gamexml.LoadTeleports(filepath.Join(xmlRoot, "teleports.xml")); err != nil {
		return err
	}
	if d.InstantTeleports, err = gamexml.LoadInstantTeleports(filepath.Join(xmlRoot, "instantTeleports.xml")); err != nil {
		return err
	}
	if d.Admin, err = gamexml.LoadAdminData(xmlRoot); err != nil {
		return err
	}
	return nil
}

// loadEconomyData reads the crafting, trade, residence and manor tables;
// it needs the item table.
func loadEconomyData(d *gameData, xmlRoot string, log zerolog.Logger) error {
	var err error
	if d.Hennas, err = gamexml.LoadHennas(filepath.Join(xmlRoot, "hennas.xml")); err != nil {
		return err
	}
	if d.HealSps, err = gamexml.LoadHealSps(filepath.Join(xmlRoot, "healSps.xml")); err != nil {
		return err
	}
	if d.Recipes, err = gamexml.LoadRecipes(filepath.Join(xmlRoot, "recipes.xml")); err != nil {
		return err
	}
	if d.BuyLists, err = gamexml.LoadBuyLists(filepath.Join(xmlRoot, "buyLists.xml"), d.Items); err != nil {
		return err
	}
	if d.Multisells, err = gamexml.LoadMultiSellLists(filepath.Join(xmlRoot, "multisell"), d.Items); err != nil {
		return err
	}
	if d.Augmentations, err = gamexml.LoadAugmentations(filepath.Join(xmlRoot, "augmentation")); err != nil {
		return err
	}
	if d.ArmorSets, err = gamexml.LoadArmorSets(filepath.Join(xmlRoot, "armorSets.xml")); err != nil {
		return err
	}
	if d.Fish, err = gamexml.LoadFish(filepath.Join(xmlRoot, "fish.xml")); err != nil {
		return err
	}
	if d.ClanHalls, err = gamexml.LoadClanHalls(filepath.Join(xmlRoot, "clanHalls.xml")); err != nil {
		return err
	}
	if d.ClanHallDecos, err = gamexml.LoadClanHallDeco(filepath.Join(xmlRoot, "clanHallDeco.xml")); err != nil {
		return err
	}
	if d.Castles, err = gamexml.LoadCastles(filepath.Join(xmlRoot, "castles.xml")); err != nil {
		return err
	}
	if d.Manors, d.ManorAreas, err = loadManor(xmlRoot, d.Items, log); err != nil {
		return err
	}
	if d.Observers, err = gamexml.LoadObserverGroups(filepath.Join(xmlRoot, "observerGroups.xml")); err != nil {
		return err
	}
	return nil
}

func applyTownCombatRule(index *zone.Index, rule int) {
	for _, town := range zone.OfKind[*zone.Town](index) {
		town.CombatRule = rule
	}
}

// loadHTMLCache loads the datapack HTML pages. A page that is not valid
// UTF-8 is logged at error level and left out, so every send of it shows
// the missing-page notice.
func loadHTMLCache(paths gameServerPaths, log zerolog.Logger) (*datacache.HTML, error) {
	html, undecodable, err := datacache.LoadHTML(filepath.Join(paths.DataRoot, "data", "html"))
	for _, name := range undecodable {
		log.Error().Str("page", name).Msg("error caching html page: not valid UTF-8; it is treated as missing")
	}
	return html, err
}

// loadCrestCache loads the datapack crest images; see loadCrestDir.
func loadCrestCache(paths gameServerPaths, log zerolog.Logger) (*datacache.Crests, error) {
	return loadCrestDir(filepath.Join(paths.DataRoot, "data", "crests"), log)
}

// loadCrestDir loads the crest images in dir. A crest file that cannot be
// loaded is skipped and the server boots on: the skipped files are logged at
// error level, each deleted invalid crest file is warned about, and the
// loaded count is reported. A missing crest directory leaves an empty cache
// that still saves new crests there. Only a directory that exists but cannot
// be read fails.
func loadCrestDir(dir string, log zerolog.Logger) (*datacache.Crests, error) {
	crests, deleted, err := datacache.LoadCrests(dir)
	if crests == nil {
		return nil, err
	}
	for _, name := range deleted {
		log.Warn().Str("crest", name).Msg("crest data is invalid; the crest file has been deleted")
	}
	if err != nil {
		log.Error().Err(err).Bool("partial", crests.Partial()).Msg("error loading crest files")
	}
	log.Info().Int("crests", crests.Len()).Msg("crests loaded")
	return crests, nil
}

// loadGeodata loads exactly the region files geoengine.properties lists by
// their "X_Y" keys; unlisted tiles stay null-block. A listed region that
// fails to load fails boot: the server does not run on partial geodata.
// With a cache path the regions are served from that packed cache file.
func loadGeodata(paths gameServerPaths, log zerolog.Logger) (*loadedGeodata, error) {
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

	geo := &loadedGeodata{
		Dir:           geodata.ResolveDir(paths.DataRoot, props.String("GeoDataPath", "")),
		Type:          geodata.GeoType(props.String("GeoDataType", string(geodata.L2OFF))),
		EngineOptions: engineOptions,
		Pathfind:      pathOptions,
	}
	if paths.GeoCachePath != "" {
		geo.Engine, err = geodata.LoadRegionsCached(geo.Dir, geo.Type, geodata.ListedRegions(props), paths.GeoCachePath, log, geo.EngineOptions)
	} else {
		geo.Engine, err = geodata.LoadRegions(geo.Dir, geo.Type, geodata.ListedRegions(props), log, geo.EngineOptions)
	}
	if err != nil {
		return nil, err
	}
	geo.Finder = pathfind.New(geo.Engine, geo.Pathfind)
	// Game masters' bug reports go next to the geodata.
	geo.Bugs = engine.NewBugLog(geo.Dir)
	return geo, nil
}
