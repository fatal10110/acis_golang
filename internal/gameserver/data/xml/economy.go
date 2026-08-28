package xml

import (
	stdxml "encoding/xml"
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/armorset"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/augmentation"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/fish"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/recipe"
)

type recipeFile struct {
	Recipes []attrsElement `xml:"recipe"`
}

type buyListFile struct {
	BuyLists []buyListElement `xml:"buyList"`
}

type buyListElement struct {
	Attrs    []stdxml.Attr  `xml:",any,attr"`
	Products []attrsElement `xml:"product"`
}

type hennaFile struct {
	Hennas []hennaElement `xml:"henna"`
}

// hennaElement is one <henna> element. symbolId, dyeId and classes are
// required; the price and the six stat modifiers are optional and default to
// 0, but a present-but-malformed value is rejected by coord exactly as the
// attribute bag being replaced did.
type hennaElement struct {
	SymbolID *coord32     `xml:"symbolId,attr"`
	DyeID    *coord32     `xml:"dyeId,attr"`
	Price    *coord       `xml:"price,attr"`
	INT      *coord       `xml:"INT,attr"`
	STR      *coord       `xml:"STR,attr"`
	CON      *coord       `xml:"CON,attr"`
	MEN      *coord       `xml:"MEN,attr"`
	DEX      *coord       `xml:"DEX,attr"`
	WIT      *coord       `xml:"WIT,attr"`
	Classes  *intListAttr `xml:"classes,attr"`
}

func (e hennaElement) build() (henna.Henna, error) {
	if e.SymbolID == nil {
		return henna.Henna{}, fmt.Errorf("henna: symbolId is required")
	}
	if e.DyeID == nil {
		return henna.Henna{}, fmt.Errorf("henna %d: dyeId is required", *e.SymbolID)
	}
	if e.Classes == nil {
		return henna.Henna{}, fmt.Errorf("henna %d: classes is required", *e.SymbolID)
	}
	h := henna.Henna{
		SymbolID: int(*e.SymbolID),
		DyeID:    int32(*e.DyeID),
		Classes:  []int(*e.Classes),
	}
	if e.Price != nil {
		h.DrawPrice = int(*e.Price)
	}
	if e.INT != nil {
		h.INT = int(*e.INT)
	}
	if e.STR != nil {
		h.STR = int(*e.STR)
	}
	if e.CON != nil {
		h.CON = int(*e.CON)
	}
	if e.MEN != nil {
		h.MEN = int(*e.MEN)
	}
	if e.DEX != nil {
		h.DEX = int(*e.DEX)
	}
	if e.WIT != nil {
		h.WIT = int(*e.WIT)
	}
	return h, nil
}

type armorSetFile struct {
	Sets []attrsElement `xml:"armorset"`
}

type fishFile struct {
	Fishes []fishElement `xml:"fish"`
}

// fishElement is one <fish> element. Every attribute is required, so each
// field is a pointer: nil means the attribute is absent, and coord-style
// parsing rejects empty, padded, and non-numeric values exactly like the
// attribute bag being replaced did.
type fishElement struct {
	ID            *coord32 `xml:"id,attr"`
	Level         *coord   `xml:"level,attr"`
	HP            *coord   `xml:"hp,attr"`
	HPRegen       *coord   `xml:"hpRegen,attr"`
	Type          *coord   `xml:"type,attr"`
	Group         *coord   `xml:"group,attr"`
	Guts          *coord   `xml:"guts,attr"`
	GutsCheckTime *coord   `xml:"gutsCheckTime,attr"`
	WaitTime      *coord   `xml:"waitTime,attr"`
	CombatTime    *coord   `xml:"combatTime,attr"`
}

func (e fishElement) build() (fish.Fish, error) {
	if e.ID == nil {
		return fish.Fish{}, fmt.Errorf("fish: id is required")
	}
	if e.Level == nil {
		return fish.Fish{}, fmt.Errorf("fish %d: level is required", *e.ID)
	}
	if e.HP == nil {
		return fish.Fish{}, fmt.Errorf("fish %d: hp is required", *e.ID)
	}
	if e.HPRegen == nil {
		return fish.Fish{}, fmt.Errorf("fish %d: hpRegen is required", *e.ID)
	}
	if e.Type == nil {
		return fish.Fish{}, fmt.Errorf("fish %d: type is required", *e.ID)
	}
	if e.Group == nil {
		return fish.Fish{}, fmt.Errorf("fish %d: group is required", *e.ID)
	}
	if e.Guts == nil {
		return fish.Fish{}, fmt.Errorf("fish %d: guts is required", *e.ID)
	}
	if e.GutsCheckTime == nil {
		return fish.Fish{}, fmt.Errorf("fish %d: gutsCheckTime is required", *e.ID)
	}
	if e.WaitTime == nil {
		return fish.Fish{}, fmt.Errorf("fish %d: waitTime is required", *e.ID)
	}
	if e.CombatTime == nil {
		return fish.Fish{}, fmt.Errorf("fish %d: combatTime is required", *e.ID)
	}
	return fish.New(
		int32(*e.ID),
		int(*e.Level),
		int(*e.HP),
		int(*e.HPRegen),
		int(*e.Type),
		int(*e.Group),
		int(*e.Guts),
		int(*e.GutsCheckTime),
		int(*e.WaitTime),
		int(*e.CombatTime),
	), nil
}

type augmentationFile struct {
	Skills []augmentationSkillElement `xml:"augmentation"`
	Sets   []augmentationSetElement   `xml:"set"`
}

// augmentationSkillElement is required because zero is a legal skill/level
// id: a plain int cannot tell an absent attribute from "0", so id, skillId,
// and skillLevel use coord to reject that ambiguity the same way the
// StatSet bag being replaced did.
type augmentationSkillElement struct {
	ID         *coord `xml:"id,attr"`
	SkillID    *coord `xml:"skillId,attr"`
	SkillLevel *coord `xml:"skillLevel,attr"`
	Type       string `xml:"type,attr"`
}

func (e augmentationSkillElement) skill() (augmentation.Skill, error) {
	if e.ID == nil {
		return augmentation.Skill{}, fmt.Errorf("augmentation skill: id is required")
	}
	if e.SkillID == nil {
		return augmentation.Skill{}, fmt.Errorf("augmentation skill %d: skillId is required", *e.ID)
	}
	if e.SkillLevel == nil {
		return augmentation.Skill{}, fmt.Errorf("augmentation skill %d: skillLevel is required", *e.ID)
	}
	skillID := int(*e.SkillID)
	if skillID < math.MinInt32 || skillID > math.MaxInt32 {
		return augmentation.Skill{}, fmt.Errorf("augmentation skill %d: skillId %d overflows int32", *e.ID, skillID)
	}
	return augmentation.NewSkill(int(*e.ID), int32(skillID), int(*e.SkillLevel), e.Type)
}

type augmentationSetElement struct {
	Order *coord                    `xml:"order,attr"`
	Stats []augmentationStatElement `xml:"stat"`
}

type augmentationStatElement struct {
	Name   string                     `xml:"name,attr"`
	Tables []augmentationTableElement `xml:"table"`
}

type augmentationTableElement struct {
	Name string `xml:"name,attr"`
	Text string `xml:",chardata"`
}

// LoadRecipes parses recipes.xml and returns recipes keyed by recipe id.
func LoadRecipes(path string) (*recipe.Table, error) {
	var file recipeFile
	if err := readXML(path, &file); err != nil {
		return nil, fmt.Errorf("recipes: %w", err)
	}
	recipes, err := buildAll(path, file.Recipes, recipe.New)
	if err != nil {
		return nil, err
	}
	return recipe.NewTable(recipes), nil
}

// LoadBuyLists parses buyLists.xml and returns buylists keyed by list id.
func LoadBuyLists(path string) (*buylist.Table, error) {
	var file buyListFile
	if err := readXML(path, &file); err != nil {
		return nil, fmt.Errorf("buy lists: %w", err)
	}
	lists := make([]buylist.List, 0, len(file.BuyLists))
	for _, el := range file.BuyLists {
		set := commons.StatSetFromXMLAttrs(el.Attrs)
		id, err := set.GetInt("id")
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		products, err := buildAll(path, el.Products, func(set *commons.StatSet) (buylist.Product, error) {
			return buylist.NewProduct(id, set)
		})
		if err != nil {
			return nil, err
		}
		list, err := buylist.NewList(set, products)
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		lists = append(lists, list)
	}
	return buylist.NewTable(lists), nil
}

// LoadHennas parses hennas.xml and returns hennas keyed by symbol id.
func LoadHennas(path string) (*henna.Table, error) {
	var file hennaFile
	if err := readXML(path, &file); err != nil {
		return nil, fmt.Errorf("hennas: %w", err)
	}
	hennas := make([]henna.Henna, 0, len(file.Hennas))
	for _, el := range file.Hennas {
		h, err := el.build()
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		hennas = append(hennas, h)
	}
	return henna.NewTable(hennas), nil
}

// LoadArmorSets parses armorSets.xml and returns armor sets keyed by chest item id.
func LoadArmorSets(path string) (*armorset.Table, error) {
	var file armorSetFile
	if err := readXML(path, &file); err != nil {
		return nil, fmt.Errorf("armor sets: %w", err)
	}
	sets, err := buildAll(path, file.Sets, armorset.New)
	if err != nil {
		return nil, err
	}
	return armorset.NewTable(sets), nil
}

// LoadFish parses fish.xml and returns fish rows keyed by fish id.
func LoadFish(path string) (*fish.Table, error) {
	var file fishFile
	if err := readXML(path, &file); err != nil {
		return nil, fmt.Errorf("fish: %w", err)
	}
	rows := make([]fish.Fish, 0, len(file.Fishes))
	for _, el := range file.Fishes {
		row, err := el.build()
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		rows = append(rows, row)
	}
	return fish.NewTable(rows), nil
}

// LoadAugmentations parses the augmentation XML directory and returns stat and skill tables.
func LoadAugmentations(dir string) (*augmentation.Table, error) {
	docs, err := loadXMLDocuments[augmentationFile](dir, "augmentation")
	if err != nil {
		return nil, err
	}
	var groups []augmentation.StatGroup
	var skills []augmentation.Skill
	for _, doc := range docs {
		for _, el := range doc.Data.Skills {
			skill, err := el.skill()
			if err != nil {
				return nil, fmt.Errorf("xml: %s: %w", doc.Path, err)
			}
			skills = append(skills, skill)
		}
		for _, el := range doc.Data.Sets {
			group, err := buildAugmentationStatGroup(el)
			if err != nil {
				return nil, fmt.Errorf("xml: %s: %w", doc.Path, err)
			}
			groups = append(groups, group)
		}
	}
	table, err := augmentation.NewTable(groups, skills)
	if err != nil {
		return nil, fmt.Errorf("xml: %s: %w", filepath.Join(dir, "*.xml"), err)
	}
	return table, nil
}

func buildAugmentationStatGroup(el augmentationSetElement) (augmentation.StatGroup, error) {
	stats := make([]augmentation.Stat, 0, len(el.Stats))
	for _, statEl := range el.Stats {
		var solo, combined []float32
		for _, tableEl := range statEl.Tables {
			values, err := parseFloatTable(tableEl.Text)
			if err != nil {
				return augmentation.StatGroup{}, fmt.Errorf("stat %s table %s: %w", statEl.Name, tableEl.Name, err)
			}
			if strings.EqualFold(tableEl.Name, "#soloValues") {
				solo = values
			} else {
				combined = values
			}
		}
		stat, err := augmentation.NewStat(statEl.Name, solo, combined)
		if err != nil {
			return augmentation.StatGroup{}, err
		}
		stats = append(stats, stat)
	}
	if el.Order == nil {
		return augmentation.StatGroup{}, fmt.Errorf("augmentation stat group: order is required")
	}
	return augmentation.NewStatGroup(int(*el.Order), stats)
}

func parseFloatTable(raw string) ([]float32, error) {
	fields := strings.Fields(raw)
	values := make([]float32, len(fields))
	for i, field := range fields {
		v, err := strconv.ParseFloat(field, 32)
		if err != nil {
			return nil, err
		}
		values[i] = float32(v)
	}
	return values, nil
}
