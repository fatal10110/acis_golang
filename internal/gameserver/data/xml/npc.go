package xml

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/sirupsen/logrus"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// npcFile is the root <list> element of one NPC template XML file.
type npcFile struct {
	Npcs []npcElement `xml:"npc"`
}

// npcElement is one <npc> element. Its own attributes (id, idTemplate,
// name, title, alias) fold in directly; everything else is a distinctly
// shaped child block handled by its own type.
type npcElement struct {
	Attrs    []xml.Attr        `xml:",any,attr"`
	Sets     []setElem         `xml:"set"`
	AI       []setElem         `xml:"ai>set"`
	Drops    []categoryElement `xml:"drops>category"`
	Privates []attrsElement    `xml:"privates>private"`
	PetData  *petDataElement   `xml:"petdata"`
	Skills   []attrsElement    `xml:"skills>skill"`
	TeachTo  *attrsElement     `xml:"teachTo"`
}

// categoryElement is one <category> element under <drops>: its own
// type/chance attributes plus a flat list of <drop> children.
type categoryElement struct {
	Attrs []xml.Attr     `xml:",any,attr"`
	Drops []attrsElement `xml:"drop"`
}

// petDataElement is the <petdata> element: its own food/feed-limit
// attributes plus one <stat> child per pet level.
type petDataElement struct {
	Attrs []xml.Attr     `xml:",any,attr"`
	Stats []attrsElement `xml:"stat"`
}

// LoadNPCTemplates parses every ".xml" NPC template file directly under dir
// and returns a lookup table of the resulting templates keyed by npc id.
// items is consulted to validate each drop entry's item id, exactly as
// much of it as has been loaded by the time this runs; an entry referencing
// an id items doesn't have is logged and dropped rather than failing the
// load, matching how a shipped file can reference an item template that
// hasn't shipped in this data set.
//
// A directory that can't be listed, a file whose XML is not well-formed, or
// an <npc> element missing or mangling a required attribute fails the whole
// load: the caller gets an error rather than a partially populated table.
//
// log receives skipped-drop diagnostics; a nil log defaults to
// logrus.StandardLogger().
func LoadNPCTemplates(dir string, items *item.Table, log *logrus.Logger) (*npc.Table, error) {
	if log == nil {
		log = logrus.StandardLogger()
	}

	paths, err := filepath.Glob(filepath.Join(dir, "*.xml"))
	if err != nil {
		return nil, fmt.Errorf("xml: list npc template files in %s: %w", dir, err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("xml: no npc template files found in %s", dir)
	}
	sort.Strings(paths)

	var templates []*npc.Template
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("xml: read %s: %w", path, err)
		}

		var doc npcFile
		if err := xml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("xml: parse %s: %w", path, err)
		}

		for _, el := range doc.Npcs {
			tpl, err := buildNPCTemplate(el, items, log)
			if err != nil {
				return nil, fmt.Errorf("xml: %s: %w", path, err)
			}
			templates = append(templates, tpl)
		}
	}

	return npc.NewTable(templates), nil
}

// buildNPCTemplate packs one parsed <npc> element into the StatSet shape
// npc.NewTemplate consumes: its own attributes and <set> children merged
// flat, plus the "aiParams", "drops", "privates", "race", "teachTo" and
// "pet" values built from its other child blocks.
func buildNPCTemplate(el npcElement, items *item.Table, log *logrus.Logger) (*npc.Template, error) {
	set := commons.StatSetFromXMLAttrs(el.Attrs)
	for _, s := range el.Sets {
		set.Set(s.Name, s.Val)
	}
	npcID, _ := set.GetInt("id")

	if len(el.AI) > 0 {
		ai := commons.NewStatSetWithCapacity(len(el.AI))
		for _, s := range el.AI {
			ai.Set(s.Name, s.Val)
		}
		set.Set("aiParams", ai)
	}

	if len(el.Drops) > 0 {
		categories := make([]item.DropCategory, 0, len(el.Drops))
		for _, catEl := range el.Drops {
			drops := make([]item.Drop, 0, len(catEl.Drops))
			for _, dropEl := range catEl.Drops {
				drop, err := item.NewDrop(commons.StatSetFromXMLAttrs(dropEl.Attrs))
				if err != nil {
					return nil, fmt.Errorf("npc %d: %w", npcID, err)
				}
				if _, ok := items.Get(drop.ItemID); !ok {
					log.Warnf("data/xml: npc %d: drop references undefined item %d, skipping", npcID, drop.ItemID)
					continue
				}
				drops = append(drops, drop)
			}
			category, err := item.NewDropCategory(commons.StatSetFromXMLAttrs(catEl.Attrs), drops)
			if err != nil {
				return nil, fmt.Errorf("npc %d: %w", npcID, err)
			}
			categories = append(categories, category)
		}
		set.Set("drops", categories)
	}

	if len(el.Privates) > 0 {
		privates := make([]npc.PrivateEntry, 0, len(el.Privates))
		for _, p := range el.Privates {
			entry, err := npc.NewPrivateEntry(commons.StatSetFromXMLAttrs(p.Attrs))
			if err != nil {
				return nil, fmt.Errorf("npc %d: %w", npcID, err)
			}
			privates = append(privates, entry)
		}
		set.Set("privates", privates)
	}

	if el.PetData != nil {
		levels := make(map[int]npc.PetLevelStats, len(el.PetData.Stats))
		for _, s := range el.PetData.Stats {
			statSet := commons.StatSetFromXMLAttrs(s.Attrs)
			level, err := statSet.GetInt("level")
			if err != nil {
				return nil, fmt.Errorf("npc %d: pet level: %w", npcID, err)
			}
			stats, err := npc.NewPetLevelStats(statSet)
			if err != nil {
				return nil, fmt.Errorf("npc %d: %w", npcID, err)
			}
			levels[level] = stats
		}
		pet, err := npc.NewPetData(commons.StatSetFromXMLAttrs(el.PetData.Attrs), levels)
		if err != nil {
			return nil, fmt.Errorf("npc %d: %w", npcID, err)
		}
		set.Set("pet", pet)
	}

	// Resolving a <skill> entry to its effect is skill-engine behavior this
	// loader doesn't own. The one exception is race: a template's race is
	// encoded as either a secondary "race marker" skill id, or the level of
	// the dedicated race skill, and both are plain ids readable straight
	// off the XML with no skill-engine lookup at all.
	for _, s := range el.Skills {
		skillSet := commons.StatSetFromXMLAttrs(s.Attrs)
		skillID, err := skillSet.GetInt("id")
		if err != nil {
			return nil, fmt.Errorf("npc %d: skill: %w", npcID, err)
		}
		if race := npc.RaceBySecondarySkillID(skillID); race != npc.RaceDummy {
			set.Set("race", race)
			continue
		}
		if skillID == npc.RaceSkillID && !set.Has("race") {
			level, err := skillSet.GetInt("level")
			if err != nil {
				return nil, fmt.Errorf("npc %d: skill: %w", npcID, err)
			}
			race, ok := npc.RaceByOrdinal(level)
			if !ok {
				return nil, fmt.Errorf("npc %d: race skill level %d out of range", npcID, level)
			}
			set.Set("race", race)
		}
	}

	if el.TeachTo != nil {
		classes, err := commons.StatSetFromXMLAttrs(el.TeachTo.Attrs).GetIntArray("classes")
		if err != nil {
			return nil, fmt.Errorf("npc %d: teachTo: %w", npcID, err)
		}
		set.Set("teachTo", classes)
	}

	return npc.NewTemplate(set)
}
