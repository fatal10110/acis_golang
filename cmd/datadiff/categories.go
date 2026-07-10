package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/datadiff"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/multisell"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
)

// category wires one loader's output into the dump format.
type category struct {
	// load reads every record of this category from the aCis_datapack
	// checkout rooted at root. It does not need to sort the result;
	// datadiff.WriteDump and datadiff.Compare both do that themselves.
	load func(root string) ([]datadiff.Record, error)
}

// categories lists every data category this command can dump or compare
// today.
var categories = map[string]category{
	"boatroute":       {load: loadBoatRouteRecords},
	"castle":          {load: loadCastleRecords},
	"clanhall":        {load: loadClanHallRecords},
	"classtemplate":   {load: loadClassTemplateRecords},
	"door":            {load: loadDoorRecords},
	"html":            {load: loadHTMLRecords},
	"instantteleport": {load: loadInstantTeleportRecords},
	"item":            {load: loadItemRecords},
	"multisell":       {load: loadMultisellRecords},
	"npc":             {load: loadNPCRecords},
	"playerlevels":    {load: loadPlayerLevelRecords},
	"restart":         {load: loadRestartRecords},
	"skill":           {load: loadSkillRecords},
	"staticobject":    {load: loadStaticObjectRecords},
	"teleport":        {load: loadTeleportRecords},
	"walkerroute":     {load: loadWalkerRouteRecords},
}

// sortedCategoryNames returns every registered category name, sorted.
func sortedCategoryNames() []string {
	names := make([]string, 0, len(categories))
	for name := range categories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func xmlPath(root string, elems ...string) string {
	parts := append([]string{root, "data", "xml"}, elems...)
	return filepath.Join(parts...)
}

func recordFromValue(id string, v any) (datadiff.Record, error) {
	fields, err := datadiff.Flatten(v)
	if err != nil {
		return datadiff.Record{}, err
	}
	return datadiff.Record{ID: id, Fields: fields}, nil
}

func recordsFromValues[T any](values []T, id func(int, T) string) ([]datadiff.Record, error) {
	records := make([]datadiff.Record, len(values))
	for i, value := range values {
		record, err := recordFromValue(id(i, value), value)
		if err != nil {
			return nil, err
		}
		records[i] = record
	}
	return records, nil
}

// loadItemTable loads the item template table from root, the directory an
// aCis_datapack checkout is rooted at.
func loadItemTable(root string) (*item.Table, error) {
	return xml.LoadItemTemplates(xmlPath(root, "items"))
}

// loadItemRecords reduces every loaded item template to the fields the
// item loader itself models.
func loadItemRecords(root string) ([]datadiff.Record, error) {
	table, err := loadItemTable(root)
	if err != nil {
		return nil, err
	}

	templates := table.All()
	records := make([]datadiff.Record, len(templates))
	for i, tpl := range templates {
		records[i] = itemRecord(tpl)
	}
	return records, nil
}

func itemRecord(tpl *item.Template) datadiff.Record {
	fields := map[string]string{
		"name":           tpl.Name,
		"kind":           tpl.Kind.String(),
		"slot":           strconv.FormatInt(int64(tpl.Slot), 10),
		"weight":         strconv.FormatInt(int64(tpl.Weight), 10),
		"material":       tpl.Material.String(),
		"duration":       strconv.FormatInt(int64(tpl.Duration), 10),
		"referencePrice": strconv.FormatInt(int64(tpl.ReferencePrice), 10),
		"crystal":        tpl.Crystal.String(),
		"crystalCount":   strconv.FormatInt(int64(tpl.CrystalCount), 10),
		"stackable":      strconv.FormatBool(tpl.Stackable),
		"sellable":       strconv.FormatBool(tpl.Sellable),
		"dropable":       strconv.FormatBool(tpl.Dropable),
		"destroyable":    strconv.FormatBool(tpl.Destroyable),
		"tradable":       strconv.FormatBool(tpl.Tradable),
		"depositable":    strconv.FormatBool(tpl.Depositable),
		"olyRestricted":  strconv.FormatBool(tpl.OlyRestricted),
		"defaultAction":  tpl.DefaultAction.String(),
		"attachedSkills": jsonField(tpl.AttachedSkills),
		"modifiers":      formatItemModifiers(tpl.Modifiers),
		"useConditions":  jsonField(tpl.UseConditions),
	}

	switch {
	case tpl.Weapon != nil:
		w := tpl.Weapon
		fields["weapon.type"] = w.Type.String()
		fields["weapon.soulshots"] = strconv.FormatInt(int64(w.SoulshotCount), 10)
		fields["weapon.spiritshots"] = strconv.FormatInt(int64(w.SpiritshotCount), 10)
		fields["weapon.randomDamage"] = strconv.FormatInt(int64(w.RandomDamage), 10)
		fields["weapon.mpConsume"] = strconv.FormatInt(int64(w.MPConsume), 10)
		fields["weapon.mpConsumeReduceRate"] = strconv.FormatInt(int64(w.MPConsumeReduceRate), 10)
		fields["weapon.mpConsumeReduceValue"] = strconv.FormatInt(int64(w.MPConsumeReduceValue), 10)
		fields["weapon.reuseDelay"] = strconv.FormatInt(int64(w.ReuseDelay), 10)
		fields["weapon.magical"] = strconv.FormatBool(w.Magical)
		fields["weapon.reducedSoulshotChance"] = strconv.FormatInt(int64(w.ReducedSoulshotChance), 10)
		fields["weapon.reducedSoulshotCount"] = strconv.FormatInt(int64(w.ReducedSoulshotCount), 10)
		fields["weapon.enchant4Skill"] = jsonField(w.Enchant4Skill)
		fields["weapon.onCastSkill"] = jsonField(w.OnCastSkill)
		fields["weapon.onCritSkill"] = jsonField(w.OnCritSkill)
	case tpl.Armor != nil:
		fields["armor.type"] = tpl.Armor.Type.String()
	case tpl.EtcItem != nil:
		e := tpl.EtcItem
		fields["etcItem.type"] = e.Type.String()
		fields["etcItem.handler"] = e.Handler
		fields["etcItem.sharedReuseGroup"] = strconv.FormatInt(int64(e.SharedReuseGroup), 10)
		fields["etcItem.reuseDelay"] = strconv.FormatInt(int64(e.ReuseDelay), 10)
	}

	return datadiff.Record{
		ID:     strconv.FormatInt(int64(tpl.ID), 10),
		Fields: fields,
	}
}

func jsonField(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("datadiff: item field JSON: %v", err))
	}
	return string(b)
}

func formatItemModifiers(modifiers []item.StatModifier) string {
	if len(modifiers) == 0 {
		return ""
	}
	parts := make([]string, len(modifiers))
	for i, m := range modifiers {
		parts[i] = m.Op.String() + ":" + m.Stat + ":" + datadiff.FormatFloat(m.Value)
	}
	return strings.Join(parts, ";")
}

func loadHTMLRecords(root string) ([]datadiff.Record, error) {
	html, err := datacache.LoadHTML(filepath.Join(root, "data", "html"))
	if err != nil {
		return nil, err
	}

	paths := html.Paths()
	records := make([]datadiff.Record, len(paths))
	for i, path := range paths {
		content, _ := html.Get(path)
		sum := sha256.Sum256([]byte(content))
		records[i] = datadiff.Record{
			ID: path,
			Fields: map[string]string{
				"bytes":  strconv.Itoa(len(content)),
				"sha256": fmt.Sprintf("%x", sum),
			},
		}
	}
	return records, nil
}

// loadNPCRecords reduces every loaded NPC template to a representative
// subset of its scalar fields: base combat and movement stats. It first
// loads the item table, which the NPC loader needs to validate drop
// entries against.
func loadNPCRecords(root string) ([]datadiff.Record, error) {
	items, err := loadItemTable(root)
	if err != nil {
		return nil, fmt.Errorf("npc: load item table for drop validation: %w", err)
	}

	table, err := xml.LoadNPCTemplates(xmlPath(root, "npcs"), items, nil)
	if err != nil {
		return nil, err
	}

	templates := table.All()
	records := make([]datadiff.Record, len(templates))
	for i, tpl := range templates {
		records[i] = datadiff.Record{
			ID: strconv.Itoa(tpl.ID),
			Fields: map[string]string{
				"name":      tpl.Name,
				"level":     strconv.Itoa(tpl.Level),
				"hpMax":     datadiff.FormatFloat(tpl.HPMax),
				"mpMax":     datadiff.FormatFloat(tpl.MPMax),
				"pAtk":      datadiff.FormatFloat(tpl.PAtk),
				"pDef":      datadiff.FormatFloat(tpl.PDef),
				"mAtk":      datadiff.FormatFloat(tpl.MAtk),
				"mDef":      datadiff.FormatFloat(tpl.MDef),
				"runSpeed":  datadiff.FormatFloat(tpl.RunSpeed),
				"walkSpeed": datadiff.FormatFloat(tpl.WalkSpeed),
			},
		}
	}
	return records, nil
}

// loadClassTemplateRecords reduces every loaded player profession template
// to its scalar base stats, movement, and collision fields.
func loadClassTemplateRecords(root string) ([]datadiff.Record, error) {
	table, err := xml.LoadPlayerTemplates(xmlPath(root, "classes"))
	if err != nil {
		return nil, err
	}

	templates := table.All()
	records := make([]datadiff.Record, len(templates))
	for i, tpl := range templates {
		records[i] = datadiff.Record{
			ID: strconv.Itoa(tpl.ID),
			Fields: map[string]string{
				"baseLevel":             strconv.Itoa(tpl.BaseLevel),
				"fistsItemID":           strconv.Itoa(tpl.FistsItemID),
				"str":                   strconv.Itoa(tpl.STR),
				"con":                   strconv.Itoa(tpl.CON),
				"dex":                   strconv.Itoa(tpl.DEX),
				"int":                   strconv.Itoa(tpl.INT),
				"wit":                   strconv.Itoa(tpl.WIT),
				"men":                   strconv.Itoa(tpl.MEN),
				"pAtk":                  datadiff.FormatFloat(tpl.PAtk),
				"pDef":                  datadiff.FormatFloat(tpl.PDef),
				"mAtk":                  datadiff.FormatFloat(tpl.MAtk),
				"mDef":                  datadiff.FormatFloat(tpl.MDef),
				"runSpeed":              datadiff.FormatFloat(tpl.RunSpeed),
				"walkSpeed":             datadiff.FormatFloat(tpl.WalkSpeed),
				"swimSpeed":             strconv.Itoa(tpl.SwimSpeed),
				"collisionRadius":       datadiff.FormatFloat(tpl.CollisionRadius),
				"collisionHeight":       datadiff.FormatFloat(tpl.CollisionHeight),
				"collisionRadiusFemale": datadiff.FormatFloat(tpl.CollisionRadiusFemale),
				"collisionHeightFemale": datadiff.FormatFloat(tpl.CollisionHeightFemale),
				"safeFallHeightFemale":  strconv.Itoa(tpl.SafeFallHeightFemale),
				"safeFallHeightMale":    strconv.Itoa(tpl.SafeFallHeightMale),
			},
		}
	}
	return records, nil
}

// loadPlayerLevelRecords reduces every loaded character-level row to its
// experience and death-penalty fields, keyed by level number.
func loadPlayerLevelRecords(root string) ([]datadiff.Record, error) {
	table, err := xml.LoadPlayerLevels(xmlPath(root, "playerLevels.xml"))
	if err != nil {
		return nil, err
	}

	levels := table.Levels()
	records := make([]datadiff.Record, len(levels))
	for i, level := range levels {
		l, _ := table.Level(level) // level came from Levels(), always present
		records[i] = datadiff.Record{
			ID: strconv.Itoa(level),
			Fields: map[string]string{
				"requiredExpToLevelUp": strconv.FormatInt(l.RequiredExpToLevelUp, 10),
				"karmaModifier":        datadiff.FormatFloat(l.KarmaModifier),
				"expLossAtDeath":       datadiff.FormatFloat(l.ExpLossAtDeath),
			},
		}
	}
	return records, nil
}

func loadSkillRecords(root string) ([]datadiff.Record, error) {
	table, err := xml.LoadSkillDefinitions(xmlPath(root, "skills"))
	if err != nil {
		return nil, err
	}
	return recordsFromValues(table.All(), func(_ int, def skill.Definition) string {
		return fmt.Sprintf("%d/%d", def.ID, def.Level)
	})
}

func loadMultisellRecords(root string) ([]datadiff.Record, error) {
	items, err := loadItemTable(root)
	if err != nil {
		return nil, fmt.Errorf("multisell: load item table for item resolution: %w", err)
	}
	table, err := xml.LoadMultiSellLists(xmlPath(root, "multisell"), items)
	if err != nil {
		return nil, err
	}
	return recordsFromValues(table.All(), func(_ int, list *multisell.List) string {
		return strconv.FormatInt(int64(list.ID), 10)
	})
}

func loadDoorRecords(root string) ([]datadiff.Record, error) {
	table, err := xml.LoadDoors(xmlPath(root, "doors.xml"))
	if err != nil {
		return nil, err
	}
	return recordsFromValues(table.All(), func(_ int, tmpl *door.Template) string {
		return strconv.Itoa(tmpl.ID)
	})
}

func loadCastleRecords(root string) ([]datadiff.Record, error) {
	table, err := xml.LoadCastles(xmlPath(root, "castles.xml"))
	if err != nil {
		return nil, err
	}
	return recordsFromValues(table.All(), func(_ int, entry *castle.Castle) string {
		return strconv.Itoa(entry.ID)
	})
}

func loadClanHallRecords(root string) ([]datadiff.Record, error) {
	table, err := xml.LoadClanHalls(xmlPath(root, "clanHalls.xml"))
	if err != nil {
		return nil, err
	}
	return recordsFromValues(table.All(), func(_ int, entry *clanhall.Hall) string {
		return strconv.Itoa(entry.ID)
	})
}

func loadStaticObjectRecords(root string) ([]datadiff.Record, error) {
	table, err := xml.LoadStaticObjects(xmlPath(root, "staticObjects.xml"))
	if err != nil {
		return nil, err
	}
	return recordsFromValues(table.All(), func(_ int, tmpl *staticobject.Template) string {
		return strconv.Itoa(tmpl.ID)
	})
}

func loadTeleportRecords(root string) ([]datadiff.Record, error) {
	table, err := xml.LoadTeleports(xmlPath(root, "teleports.xml"))
	if err != nil {
		return nil, err
	}
	return teleportRecords(table)
}

func loadInstantTeleportRecords(root string) ([]datadiff.Record, error) {
	table, err := xml.LoadInstantTeleports(xmlPath(root, "instantTeleports.xml"))
	if err != nil {
		return nil, err
	}
	return instantTeleportRecords(table)
}

func teleportRecords(table travel.TeleportTable) ([]datadiff.Record, error) {
	npcIDs := mapsKeys(table)
	sort.Ints(npcIDs)
	records := make([]datadiff.Record, len(npcIDs))
	for i, npcID := range npcIDs {
		record, err := recordFromValue(strconv.Itoa(npcID), table[npcID])
		if err != nil {
			return nil, err
		}
		records[i] = record
	}
	return records, nil
}

func instantTeleportRecords(table travel.InstantTable) ([]datadiff.Record, error) {
	npcIDs := mapsKeys(table)
	sort.Ints(npcIDs)
	records := make([]datadiff.Record, len(npcIDs))
	for i, npcID := range npcIDs {
		record, err := recordFromValue(strconv.Itoa(npcID), table[npcID])
		if err != nil {
			return nil, err
		}
		records[i] = record
	}
	return records, nil
}

func loadBoatRouteRecords(root string) ([]datadiff.Record, error) {
	routes, err := xml.LoadBoatRoutes(xmlPath(root, "boatRoutes.xml"))
	if err != nil {
		return nil, err
	}
	return recordsFromValues(routes, func(i int, itinerary route.BoatItinerary) string {
		if len(itinerary.Routes) == 0 {
			return fmt.Sprintf("%03d", i)
		}
		id := string(itinerary.Routes[0].Dock)
		if len(itinerary.Routes) > 1 {
			id += "->" + string(itinerary.Routes[1].Dock)
		}
		return fmt.Sprintf("%03d:%s", i, id)
	})
}

func loadWalkerRouteRecords(root string) ([]datadiff.Record, error) {
	routes, err := xml.LoadWalkerRoutes(xmlPath(root, "walkerRoutes.xml"))
	if err != nil {
		return nil, err
	}

	routeNames := mapsKeys(routes)
	sort.Strings(routeNames)
	var records []datadiff.Record
	for _, routeName := range routeNames {
		byNPC := routes[routeName]
		npcNames := mapsKeys(byNPC)
		sort.Strings(npcNames)
		for _, npcName := range npcNames {
			record, err := recordFromValue(routeName+"/"+npcName, byNPC[npcName])
			if err != nil {
				return nil, err
			}
			records = append(records, record)
		}
	}
	return records, nil
}

func loadRestartRecords(root string) ([]datadiff.Record, error) {
	table, err := xml.LoadRestartPoints(xmlPath(root, "restartPointAreas.xml"))
	if err != nil {
		return nil, err
	}

	records := make([]datadiff.Record, 0, len(table.Areas)+len(table.Points))
	for i, area := range table.Areas {
		record, err := recordFromValue(fmt.Sprintf("area/%03d", i), area)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	for i, point := range table.Points {
		record, err := recordFromValue("point/"+point.Name, point)
		if err != nil {
			return nil, err
		}
		if point.Name == "" {
			record.ID = fmt.Sprintf("point/%03d", i)
		}
		records = append(records, record)
	}
	return records, nil
}

func mapsKeys[M ~map[K]V, K comparable, V any](m M) []K {
	keys := make([]K, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}
