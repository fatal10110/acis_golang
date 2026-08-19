package xml

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// fishingSkillElement is one <fishingSkill> element. id, lvl, minLvl, itemId
// and itemCount are all required; isDwarven defaults to false.
type fishingSkillElement struct {
	ID        *coord32 `xml:"id,attr"`
	Level     *coord   `xml:"lvl,attr"`
	MinLevel  *coord   `xml:"minLvl,attr"`
	ItemID    *coord32 `xml:"itemId,attr"`
	ItemCount *coord   `xml:"itemCount,attr"`
	Dwarven   boolAttr `xml:"isDwarven,attr"`
}

func (e fishingSkillElement) build() (skill.FishingSkill, error) {
	if e.ID == nil {
		return skill.FishingSkill{}, fmt.Errorf("fishing skill: id is required")
	}
	if e.Level == nil {
		return skill.FishingSkill{}, fmt.Errorf("fishing skill %d: lvl is required", *e.ID)
	}
	if e.MinLevel == nil {
		return skill.FishingSkill{}, fmt.Errorf("fishing skill %d: minLvl is required", *e.ID)
	}
	if e.ItemID == nil {
		return skill.FishingSkill{}, fmt.Errorf("fishing skill %d: itemId is required", *e.ID)
	}
	if e.ItemCount == nil {
		return skill.FishingSkill{}, fmt.Errorf("fishing skill %d: itemCount is required", *e.ID)
	}
	return skill.NewFishingSkill(skill.ID(*e.ID), int(*e.Level), int(*e.MinLevel), int32(*e.ItemID), int(*e.ItemCount), bool(e.Dwarven)), nil
}

// clanSkillElement is one <clanSkill> element. Every attribute is required.
type clanSkillElement struct {
	ID       *coord32 `xml:"id,attr"`
	Level    *coord   `xml:"lvl,attr"`
	MinLevel *coord   `xml:"minLvl,attr"`
	Cost     *coord   `xml:"cost,attr"`
	ItemID   *coord32 `xml:"itemId,attr"`
}

func (e clanSkillElement) build() (skill.ClanSkill, error) {
	if e.ID == nil {
		return skill.ClanSkill{}, fmt.Errorf("clan skill: id is required")
	}
	if e.Level == nil {
		return skill.ClanSkill{}, fmt.Errorf("clan skill %d: lvl is required", *e.ID)
	}
	if e.MinLevel == nil {
		return skill.ClanSkill{}, fmt.Errorf("clan skill %d: minLvl is required", *e.ID)
	}
	if e.Cost == nil {
		return skill.ClanSkill{}, fmt.Errorf("clan skill %d: cost is required", *e.ID)
	}
	if e.ItemID == nil {
		return skill.ClanSkill{}, fmt.Errorf("clan skill %d: itemId is required", *e.ID)
	}
	return skill.NewClanSkill(skill.ID(*e.ID), int(*e.Level), int(*e.MinLevel), int(*e.Cost), int32(*e.ItemID)), nil
}

// enchantSkillElement is one <enchantSkill> element. id, lvl, exp, sp and the
// five rate attributes are all required; itemNeeded ("itemId-count") is
// optional.
type enchantSkillElement struct {
	ID         *coord32      `xml:"id,attr"`
	Level      *coord        `xml:"lvl,attr"`
	Exp        *coord        `xml:"exp,attr"`
	SP         *coord        `xml:"sp,attr"`
	Rate76     *coord        `xml:"rate76,attr"`
	Rate77     *coord        `xml:"rate77,attr"`
	Rate78     *coord        `xml:"rate78,attr"`
	Rate79     *coord        `xml:"rate79,attr"`
	Rate80     *coord        `xml:"rate80,attr"`
	ItemNeeded *dashPairAttr `xml:"itemNeeded,attr"`
}

func (e enchantSkillElement) build() (skill.EnchantSkill, error) {
	if e.ID == nil {
		return skill.EnchantSkill{}, fmt.Errorf("enchant skill: id is required")
	}
	if e.Level == nil {
		return skill.EnchantSkill{}, fmt.Errorf("enchant skill %d: lvl is required", *e.ID)
	}
	if e.Exp == nil {
		return skill.EnchantSkill{}, fmt.Errorf("enchant skill %d: exp is required", *e.ID)
	}
	if e.SP == nil {
		return skill.EnchantSkill{}, fmt.Errorf("enchant skill %d: sp is required", *e.ID)
	}
	if e.Rate76 == nil {
		return skill.EnchantSkill{}, fmt.Errorf("enchant skill %d: rate76 is required", *e.ID)
	}
	if e.Rate77 == nil {
		return skill.EnchantSkill{}, fmt.Errorf("enchant skill %d: rate77 is required", *e.ID)
	}
	if e.Rate78 == nil {
		return skill.EnchantSkill{}, fmt.Errorf("enchant skill %d: rate78 is required", *e.ID)
	}
	if e.Rate79 == nil {
		return skill.EnchantSkill{}, fmt.Errorf("enchant skill %d: rate79 is required", *e.ID)
	}
	if e.Rate80 == nil {
		return skill.EnchantSkill{}, fmt.Errorf("enchant skill %d: rate80 is required", *e.ID)
	}
	var itemID int32
	var itemCount int
	if e.ItemNeeded != nil {
		itemID, itemCount = e.ItemNeeded.ID, e.ItemNeeded.Count
	}
	return skill.NewEnchantSkill(
		skill.ID(*e.ID), int(*e.Level), int(*e.Exp), int(*e.SP),
		int(*e.Rate76), int(*e.Rate77), int(*e.Rate78), int(*e.Rate79), int(*e.Rate80),
		itemID, itemCount,
	), nil
}

// skillTreeFile is the root <list> element of one skill tree XML file. A
// shipped file carries exactly one of the three element kinds; the other
// two slices simply stay empty for it.
type skillTreeFile struct {
	Fishing []fishingSkillElement `xml:"fishingSkill"`
	Clan    []clanSkillElement    `xml:"clanSkill"`
	Enchant []enchantSkillElement `xml:"enchantSkill"`
}

// LoadSkillTrees parses every ".xml" file directly under dir (a shipped
// directory holds fishingSkills.xml, clanSkills.xml and enchantSkills.xml)
// and returns the combined trees. A directory that can't be listed, a file
// whose XML is not well-formed, or an element with a missing or mangled
// attribute fails the whole load.
func LoadSkillTrees(dir string) (*skill.Trees, error) {
	docs, err := loadXMLDocuments[skillTreeFile](dir, "skill tree")
	if err != nil {
		return nil, err
	}
	var trees skill.Trees
	for _, doc := range docs {
		for _, el := range doc.Data.Fishing {
			fs, err := el.build()
			if err != nil {
				return nil, fmt.Errorf("xml: %s: %w", doc.Path, err)
			}
			trees.Fishing = append(trees.Fishing, fs)
		}
		for _, el := range doc.Data.Clan {
			cs, err := el.build()
			if err != nil {
				return nil, fmt.Errorf("xml: %s: %w", doc.Path, err)
			}
			trees.Clan = append(trees.Clan, cs)
		}
		for _, el := range doc.Data.Enchant {
			es, err := el.build()
			if err != nil {
				return nil, fmt.Errorf("xml: %s: %w", doc.Path, err)
			}
			trees.Enchant = append(trees.Enchant, es)
		}
	}

	return &trees, nil
}
