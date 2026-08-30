package xml

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// maxClassID is the highest valid profession id: the reference's ClassId
// enum has 119 ordinals (0-118), including 30 reserved "dummy" slots at
// 58-87 that resolve but name no real profession.
const maxClassID = 118

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
// hasn't shipped in this data set. skills is consulted the same way for
// each <skill> id/level pair: unknown pairs are logged and skipped.
//
// A directory that can't be listed, a file whose XML is not well-formed, or
// an <npc> element missing or mangling a required attribute fails the whole
// load: the caller gets an error rather than a partially populated table.
//
// log receives skipped-drop and skipped-skill diagnostics. Its zero value
// disables logging.
func LoadNPCTemplates(dir string, items *item.Table, skills *skill.Table, log zerolog.Logger) (*npc.Table, error) {
	if skills == nil {
		return nil, fmt.Errorf("xml: npc templates: missing skill table")
	}

	docs, err := loadXMLDocuments[npcFile](dir, "npc template")
	if err != nil {
		return nil, err
	}
	var templates []*npc.Template
	for _, doc := range docs {
		for _, el := range doc.Data.Npcs {
			tpl, err := buildNPCTemplate(el, items, skills, log)
			if err != nil {
				return nil, fmt.Errorf("xml: %s: %w", doc.Path, err)
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
func buildNPCTemplate(el npcElement, items *item.Table, skillsTable *skill.Table, log zerolog.Logger) (*npc.Template, error) {
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
				drop, err := buildDrop(dropEl.Attrs)
				if err != nil {
					return nil, fmt.Errorf("npc %d: %w", npcID, err)
				}
				if _, ok := items.Get(drop.ItemID); !ok {
					log.Warn().Int("npc_id", npcID).Int32("item_id", drop.ItemID).Msg("data/xml: skipping drop with undefined item")
					continue
				}
				drops = append(drops, drop)
			}
			category, err := buildDropCategory(catEl.Attrs, drops)
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

	// Race-marker skill ids (secondary, or the dedicated primary race
	// skill) resolve race from the XML id/level and never enter the skills
	// or passives lists. Other entries are looked up in the skill table:
	// unknown id/level pairs are logged and skipped. Each type token
	// (';'-separated) of "PASSIVE" appends the ref to passives; any other
	// token records the id/level in the skills map used by pet/servitor
	// commanded-skill lookups.
	if len(el.Skills) > 0 {
		skills := make(map[int]int, len(el.Skills))
		passives := make([]skill.Ref, 0)
		for _, s := range el.Skills {
			skillSet := commons.StatSetFromXMLAttrs(s.Attrs)
			skillID, err := skillSet.GetInt("id")
			if err != nil {
				return nil, fmt.Errorf("npc %d: skill: %w", npcID, err)
			}
			level, err := skillSet.GetInt("level")
			if err != nil {
				return nil, fmt.Errorf("npc %d: skill: %w", npcID, err)
			}

			if race := npc.RaceBySecondarySkillID(skillID); race != npc.RaceDummy {
				set.Set("race", race)
				continue
			}
			if skillID == npc.RaceSkillID && !set.Has("race") {
				race, ok := npc.RaceByOrdinal(level)
				if !ok {
					return nil, fmt.Errorf("npc %d: race skill level %d out of range", npcID, level)
				}
				set.Set("race", race)
				continue
			}

			if _, ok := skillsTable.Get(skill.ID(skillID), level); !ok {
				log.Warn().Int("npc_id", npcID).Int("skill_id", skillID).Int("level", level).Msg("data/xml: skipping skill with undefined id/level")
				continue
			}

			for _, nst := range strings.Split(skillSet.GetStringDefault("type", ""), ";") {
				if nst == "PASSIVE" {
					passives = append(passives, skill.Ref{ID: skill.ID(skillID), Level: level})
					continue
				}
				skills[skillID] = level
			}
		}
		set.Set("skills", skills)
		set.Set("passives", passives)
	}

	if el.TeachTo != nil {
		classes, err := commons.StatSetFromXMLAttrs(el.TeachTo.Attrs).GetIntArray("classes")
		if err != nil {
			return nil, fmt.Errorf("npc %d: teachTo: %w", npcID, err)
		}
		for _, classID := range classes {
			if classID < 0 || classID > maxClassID {
				return nil, fmt.Errorf("npc %d: teachTo: class id %d out of range", npcID, classID)
			}
		}
		set.Set("teachTo", classes)
	}

	return npc.NewTemplate(set)
}
