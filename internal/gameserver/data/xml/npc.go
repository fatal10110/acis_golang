package xml

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// maxClassID is the highest valid profession id: there are 119 class ids
// (0-118), including 30 reserved "dummy" slots at 58-87 that resolve but
// name no real profession.
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

// buildNPCTemplate builds one parsed <npc> element: its own attributes and
// <set> children merged flat (a <set> overrides an attribute of the same
// name), plus the AI parameters, drops, privates, race, teachTo and pet data
// built from its other child blocks, and its <skills> block resolved into a
// typed npc.TemplateSkills.
func buildNPCTemplate(el npcElement, items *item.Table, skillsTable *skill.Table, log zerolog.Logger) (*npc.Template, error) {
	vals := foldAttrs(el.Attrs)
	if err := canonicalizeLiteralAttrs(vals, "id", "idTemplate"); err != nil {
		return nil, fmt.Errorf("npc %s: %w", newAttrValues(vals, "").strDefault("id", "?"), err)
	}
	for _, s := range el.Sets {
		vals[s.Name] = s.Val
	}
	var npcID int
	if n, err := strconv.Atoi(vals["id"]); err == nil {
		npcID = n
	}

	var aiParams npc.AIParams
	var aiParamKeys []string
	if len(el.AI) > 0 {
		aiParams = make(npc.AIParams, len(el.AI))
		for _, s := range el.AI {
			if _, seen := aiParams[s.Name]; !seen {
				aiParamKeys = append(aiParamKeys, s.Name)
			}
			aiParams[s.Name] = s.Val
		}
	}

	var categories []item.DropCategory
	if len(el.Drops) > 0 {
		categories = make([]item.DropCategory, 0, len(el.Drops))
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
	}

	var privates []npc.PrivateEntry
	if len(el.Privates) > 0 {
		privates = make([]npc.PrivateEntry, 0, len(el.Privates))
		for _, p := range el.Privates {
			entry, err := buildNPCPrivate(newAttrValues(foldAttrs(p.Attrs), ""))
			if err != nil {
				return nil, fmt.Errorf("npc %d: %w", npcID, err)
			}
			privates = append(privates, entry)
		}
	}

	var pet *npc.PetData
	if el.PetData != nil {
		levels := make(map[int]npc.PetLevelStats, len(el.PetData.Stats))
		for _, s := range el.PetData.Stats {
			a := newAttrValues(foldAttrs(s.Attrs), fmt.Sprintf("npc %d: pet level", npcID))
			level := a.int("level")
			if err := a.Err(); err != nil {
				return nil, err
			}
			stats, err := buildPetLevelStats(a)
			if err != nil {
				return nil, fmt.Errorf("npc %d: %w", npcID, err)
			}
			levels[level] = stats
		}
		a := newAttrValues(foldAttrs(el.PetData.Attrs), fmt.Sprintf("npc %d: petdata", npcID))
		var err error
		pet, err = buildPetData(a, levels)
		if err != nil {
			return nil, err
		}
	}

	// Race-marker skill ids (secondary, or the dedicated primary race
	// skill while no race is set) resolve race from the XML id/level and
	// grant nothing. Other entries are looked up in the skill table:
	// unknown id/level pairs are logged and skipped before their types are
	// read. Each token of the ';'-separated type list files the skill under
	// that type, or among the passives for PASSIVE.
	var (
		skills  npc.TemplateSkills
		race    npc.Race
		hasRace bool
	)
	if len(el.Skills) > 0 {
		skills.ByType = make(map[npc.SkillType]skill.Ref, len(el.Skills))
		skills.Passives = make([]skill.Ref, 0)
		for _, s := range el.Skills {
			a := newAttrValues(foldAttrs(s.Attrs), fmt.Sprintf("npc %d: skill", npcID))
			skillID32 := a.int32Literal("id")
			level := int(a.int32Literal("level"))
			if err := a.Err(); err != nil {
				return nil, err
			}
			skillID := int(skillID32)

			if r := npc.RaceBySecondarySkillID(skillID); r != npc.RaceDummy {
				race, hasRace = r, true
				continue
			}
			if skillID == npc.RaceSkillID && !hasRace {
				r, ok := npc.RaceByOrdinal(level)
				if !ok {
					return nil, fmt.Errorf("npc %d: race skill level %d out of range", npcID, level)
				}
				race, hasRace = r, true
				continue
			}

			typeList := a.str("type")
			if err := a.Err(); err != nil {
				return nil, err
			}

			id := skill.ID(skillID32)
			if _, ok := skillsTable.Get(id, level); !ok {
				log.Warn().Int("npc_id", npcID).Int("skill_id", skillID).Int("level", level).Msg("data/xml: skipping skill with undefined id/level")
				continue
			}

			ref := skill.Ref{ID: id, Level: level}
			for _, token := range splitSkillTypeList(typeList) {
				typ, ok := npc.ParseSkillType(token)
				if !ok {
					return nil, fmt.Errorf("npc %d: skill %d: unknown type %q", npcID, skillID, token)
				}
				if typ == npc.SkillTypePassive {
					skills.Passives = append(skills.Passives, ref)
					continue
				}
				skills.ByType[typ] = ref
			}
		}
	}

	var teachTo []int
	if el.TeachTo != nil {
		a := newAttrValues(foldAttrs(el.TeachTo.Attrs), fmt.Sprintf("npc %d: teachTo", npcID))
		teachTo = a.intArray("classes")
		if err := a.Err(); err != nil {
			return nil, err
		}
		for _, classID := range teachTo {
			if classID < 0 || classID > maxClassID {
				return nil, fmt.Errorf("npc %d: teachTo: class id %d out of range", npcID, classID)
			}
		}
	}

	a := newAttrValues(vals, "npc template")
	id := a.int("id")
	if err := a.Err(); err != nil {
		return nil, err
	}
	a.prefix = fmt.Sprintf("npc template %d", id)
	t := npc.Template{
		ID:         id,
		TemplateID: a.intDefault("idTemplate", id),
		Name:       a.str("name"),

		Title: a.strDefault("title", ""),
		Alias: a.strDefault("alias", ""),

		UsingServerSideName:  a.boolDefault("usingServerSideName", false),
		UsingServerSideTitle: a.boolDefault("usingServerSideTitle", false),

		Type:          a.str("type"),
		Level:         a.intDefault("level", 1),
		HitTimeFactor: a.float64Default("hitTimeFactor", 0),
		RightHand:     a.intDefault("rHand", 0),
		LeftHand:      a.intDefault("lHand", 0),
		RewardExp:     a.float64Default("exp", 0),
		RewardSp:      a.float64Default("sp", 0),

		BaseAttackRange:  a.intDefault("baseAttackRange", 0),
		BaseDamageRange:  a.intArray("baseDamageRange"),
		BaseRandomDamage: a.intDefault("baseRandomDamage", 0),

		Race: race,

		STR: a.intDefault("str", 40),
		CON: a.intDefault("con", 21),
		DEX: a.intDefault("dex", 30),
		INT: a.intDefault("int", 20),
		WIT: a.intDefault("wit", 43),
		MEN: a.intDefault("men", 20),

		HPMax:   a.float64Default("hp", 0),
		MPMax:   a.float64Default("mp", 0),
		HPRegen: a.float64Default("hpRegen", 1.5),
		MPRegen: a.float64Default("mpRegen", 0.9),
		PAtk:    a.float64("pAtk"),
		MAtk:    a.float64("mAtk"),
		PDef:    a.float64("pDef"),
		MDef:    a.float64("mDef"),

		AtkSpd:    a.float64Default("atkSpd", 300),
		CritRate:  a.float64Default("crit", 4),
		WalkSpeed: a.float64Default("walkSpd", 0),
		RunSpeed:  a.float64Default("runSpd", 1),

		CollisionRadius: a.float64("radius"),
		CollisionHeight: a.float64("height"),

		SSCount:  a.intDefault("ssCount", 0),
		SPSCount: a.intDefault("spsCount", 0),

		Undying:       a.boolDefault("undying", false),
		CanBeAttacked: a.boolDefault("canBeAttacked", true),
		CorpseTime:    a.intDefault("corpseTime", 7),
		NoSleepMode:   a.boolDefault("noSleepMode", false),
		AggroRange:    a.intDefault("aggroRange", 0),
		CanMove:       a.boolDefault("canMove", true),
		Seedable:      a.boolDefault("seedable", false),
		CanSeeThrough: a.boolDefault("canSeeThrough", false),

		AIParams:    aiParams,
		AIParamKeys: aiParamKeys,
		Drops:       categories,
		Privates:    privates,
		TeachTo:     teachTo,
		Pet:         pet,
	}
	if a.has("clan") {
		t.Clans = a.strArray("clan")
		t.ClanRange = a.int("clanRange")
		if a.has("ignoredIds") {
			t.IgnoredIDs = a.intArray("ignoredIds")
		}
	}
	if err := a.Err(); err != nil {
		return nil, err
	}
	return npc.NewTemplate(t, skills), nil
}

// buildNPCPrivate builds one template <private> escort entry. id, weight
// and respawn are all required.
func buildNPCPrivate(a *attrValues) (npc.PrivateEntry, error) {
	a.prefix = "npc: private entry"
	id := a.int("id")
	if err := a.Err(); err != nil {
		return npc.PrivateEntry{}, err
	}
	a.prefix = fmt.Sprintf("npc: private entry %d", id)
	weight := a.int("weight")
	respawnStr := a.str("respawn")
	if err := a.Err(); err != nil {
		return npc.PrivateEntry{}, err
	}
	respawn, err := commons.ParseGameDuration(respawnStr)
	if err != nil {
		return npc.PrivateEntry{}, fmt.Errorf("npc: private entry %d: %w", id, err)
	}
	return npc.PrivateEntry{NpcID: id, Weight: weight, RespawnDelay: respawn}, nil
}

// buildPetData builds a <petdata> element from its attributes and its
// already-built per-level stat rows. food1 and food2 are integer literals;
// every attribute is required.
func buildPetData(a *attrValues, levels map[int]npc.PetLevelStats) (*npc.PetData, error) {
	p := &npc.PetData{
		Food1:         int(a.int32Literal("food1")),
		Food2:         int(a.int32Literal("food2")),
		AutoFeedLimit: a.float64("autoFeedLimit"),
		HungryLimit:   a.float64("hungryLimit"),
		UnsummonLimit: a.float64("unsummonLimit"),
		Levels:        levels,
	}
	if err := a.Err(); err != nil {
		return nil, err
	}
	return p, nil
}

// buildPetLevelStats builds one <petdata> <stat> row. The mounted fields
// default to 0 when the row defines no "*OnRide"/"speedOnRide" attributes at
// all, matching a pet that cannot be ridden at this level.
func buildPetLevelStats(a *attrValues) (npc.PetLevelStats, error) {
	a.prefix = "npc: pet level stats"
	s := npc.PetLevelStats{
		MaxExp:       a.int64("exp"),
		MaxMeal:      a.int("maxMeal"),
		ExpType:      a.int("expType"),
		MealInBattle: a.int("mealInBattle"),
		MealInNormal: a.int("mealInNormal"),
		PAtk:         a.float64("pAtk"),
		PDef:         a.float64("pDef"),
		MAtk:         a.float64("mAtk"),
		MDef:         a.float64("mDef"),
		MaxHP:        a.float64("hp"),
		MaxMP:        a.float64("mp"),
		HPRegen:      a.float64("hpRegen"),
		MPRegen:      a.float64("mpRegen"),
		SSCount:      a.int("ssCount"),
		SPSCount:     a.int("spsCount"),

		MountMealInBattle: a.intDefault("mealInBattleOnRide", 0),
		MountMealInNormal: a.intDefault("mealInNormalOnRide", 0),
		MountAtkSpd:       a.float64Default("atkSpdOnRide", 0),
		MountPAtk:         a.float64Default("pAtkOnRide", 0),
		MountMAtk:         a.float64Default("mAtkOnRide", 0),
	}
	if a.has("speedOnRide") {
		if speeds := a.intArray("speedOnRide"); len(speeds) >= 5 {
			s.MountBaseSpeed, s.MountWaterSpeed, s.MountFlySpeed = speeds[0], speeds[2], speeds[4]
		} else {
			a.fail(fmt.Errorf("attribute %q: want at least 5 values, got %d", "speedOnRide", len(speeds)))
		}
	}
	if err := a.Err(); err != nil {
		return npc.PetLevelStats{}, err
	}
	return s, nil
}

// splitSkillTypeList splits a <skill> type list on ';'. A list with no
// separator is its own single token, even when empty; otherwise trailing
// empty tokens are dropped, so "BUFF;" is one token and ";;" is none.
func splitSkillTypeList(list string) []string {
	tokens := strings.Split(list, ";")
	if len(tokens) == 1 {
		return tokens
	}
	for len(tokens) > 0 && tokens[len(tokens)-1] == "" {
		tokens = tokens[:len(tokens)-1]
	}
	return tokens
}
