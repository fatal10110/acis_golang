package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/fatal10110/acis_golang/internal/datadiff"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// category wires one loader's output into the dump format.
//
// Adding a category later is mechanical: write one function shaped like
// load below — call the loader, reduce each result to an ID plus a
// map of comparable fields (rendering any float with datadiff.FormatFloat
// so both sides of a future comparison format it the same way) — then add
// one entry to the categories map.
type category struct {
	// load reads every record of this category from the aCis_datapack
	// checkout rooted at root. It does not need to sort the result;
	// datadiff.WriteDump and datadiff.Compare both do that themselves.
	load func(root string) ([]datadiff.Record, error)
}

// categories lists every data category this command can dump or compare
// today.
var categories = map[string]category{
	"item":          {load: loadItemRecords},
	"npc":           {load: loadNPCRecords},
	"classtemplate": {load: loadClassTemplateRecords},
	"playerlevels":  {load: loadPlayerLevelRecords},
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

// loadItemTable loads the item template table from root, the directory an
// aCis_datapack checkout is rooted at.
func loadItemTable(root string) (*item.Table, error) {
	return xml.LoadItemTemplates(filepath.Join(root, "data", "xml", "items"), nil)
}

// loadItemRecords reduces every loaded item template to the fields the
// item loader itself models: name, kind, equip slot, and stackability.
func loadItemRecords(root string) ([]datadiff.Record, error) {
	table, err := loadItemTable(root)
	if err != nil {
		return nil, err
	}

	templates := table.All()
	records := make([]datadiff.Record, len(templates))
	for i, tpl := range templates {
		records[i] = datadiff.Record{
			ID: strconv.FormatInt(int64(tpl.ID), 10),
			Fields: map[string]string{
				"name":      tpl.Name,
				"kind":      tpl.Kind.String(),
				"slot":      strconv.FormatInt(int64(tpl.Slot), 10),
				"stackable": strconv.FormatBool(tpl.Stackable),
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

	table, err := xml.LoadNPCTemplates(filepath.Join(root, "data", "xml", "npcs"), items, nil)
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
// to its scalar base stats, movement, and collision fields. Its starter
// items, skill grants, and spawn points (nested list fields) aren't
// reduced to comparable text yet; extending this function to cover them is
// the same mechanical shape as everything already here.
func loadClassTemplateRecords(root string) ([]datadiff.Record, error) {
	table, err := xml.LoadPlayerTemplates(filepath.Join(root, "data", "xml", "classes"))
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
	table, err := xml.LoadPlayerLevels(filepath.Join(root, "data", "xml", "playerLevels.xml"))
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
