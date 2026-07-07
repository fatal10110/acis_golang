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
	"html":          {load: loadHTMLRecords},
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
	return xml.LoadItemTemplates(filepath.Join(root, "data", "xml", "items"))
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
