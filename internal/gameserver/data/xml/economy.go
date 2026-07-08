package xml

import (
	stdxml "encoding/xml"
	"fmt"
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
	Hennas []attrsElement `xml:"henna"`
}

type armorSetFile struct {
	Sets []attrsElement `xml:"armorset"`
}

type fishFile struct {
	Fish []attrsElement `xml:"fish"`
}

type augmentationFile struct {
	Skills []attrsElement           `xml:"augmentation"`
	Sets   []augmentationSetElement `xml:"set"`
}

type augmentationSetElement struct {
	Attrs []stdxml.Attr             `xml:",any,attr"`
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
		return nil, err
	}
	recipes := make([]recipe.Recipe, 0, len(file.Recipes))
	for _, el := range file.Recipes {
		r, err := recipe.New(commons.StatSetFromXMLAttrs(el.Attrs))
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		recipes = append(recipes, r)
	}
	return recipe.NewTable(recipes), nil
}

// LoadBuyLists parses buyLists.xml and returns buylists keyed by list id.
func LoadBuyLists(path string) (*buylist.Table, error) {
	var file buyListFile
	if err := readXML(path, &file); err != nil {
		return nil, err
	}
	lists := make([]buylist.List, 0, len(file.BuyLists))
	for _, el := range file.BuyLists {
		set := commons.StatSetFromXMLAttrs(el.Attrs)
		id, err := set.GetInt("id")
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		products := make([]buylist.Product, 0, len(el.Products))
		for _, productEl := range el.Products {
			product, err := buylist.NewProduct(id, commons.StatSetFromXMLAttrs(productEl.Attrs))
			if err != nil {
				return nil, fmt.Errorf("xml: %s: %w", path, err)
			}
			products = append(products, product)
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
		return nil, err
	}
	hennas := make([]henna.Henna, 0, len(file.Hennas))
	for _, el := range file.Hennas {
		h, err := henna.New(commons.StatSetFromXMLAttrs(el.Attrs))
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
		return nil, err
	}
	sets := make([]armorset.Set, 0, len(file.Sets))
	for _, el := range file.Sets {
		set, err := armorset.New(commons.StatSetFromXMLAttrs(el.Attrs))
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		sets = append(sets, set)
	}
	return armorset.NewTable(sets), nil
}

// LoadFish parses fish.xml and returns fish rows keyed by fish id.
func LoadFish(path string) (*fish.Table, error) {
	var file fishFile
	if err := readXML(path, &file); err != nil {
		return nil, err
	}
	rows := make([]fish.Fish, 0, len(file.Fish))
	for _, el := range file.Fish {
		row, err := fish.New(commons.StatSetFromXMLAttrs(el.Attrs))
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
			skill, err := augmentation.NewSkill(commons.StatSetFromXMLAttrs(el.Attrs))
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
			switch tableEl.Name {
			case "#soloValues":
				solo = values
			case "#combinedValues":
				combined = values
			default:
				return augmentation.StatGroup{}, fmt.Errorf("stat %s: unknown table %q", statEl.Name, tableEl.Name)
			}
		}
		stat, err := augmentation.NewStat(statEl.Name, solo, combined)
		if err != nil {
			return augmentation.StatGroup{}, err
		}
		stats = append(stats, stat)
	}
	return augmentation.NewStatGroup(commons.StatSetFromXMLAttrs(el.Attrs), stats)
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
