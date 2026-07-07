package xml

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// skillTreeFile is the root <list> element of one skill tree XML file. A
// shipped file carries exactly one of the three element kinds; the other
// two slices simply stay empty for it.
type skillTreeFile struct {
	Fishing []attrsElement `xml:"fishingSkill"`
	Clan    []attrsElement `xml:"clanSkill"`
	Enchant []attrsElement `xml:"enchantSkill"`
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
			fs, err := skill.NewFishingSkill(commons.StatSetFromXMLAttrs(el.Attrs))
			if err != nil {
				return nil, fmt.Errorf("xml: %s: %w", doc.Path, err)
			}
			trees.Fishing = append(trees.Fishing, fs)
		}
		for _, el := range doc.Data.Clan {
			cs, err := skill.NewClanSkill(commons.StatSetFromXMLAttrs(el.Attrs))
			if err != nil {
				return nil, fmt.Errorf("xml: %s: %w", doc.Path, err)
			}
			trees.Clan = append(trees.Clan, cs)
		}
		for _, el := range doc.Data.Enchant {
			es, err := skill.NewEnchantSkill(commons.StatSetFromXMLAttrs(el.Attrs))
			if err != nil {
				return nil, fmt.Errorf("xml: %s: %w", doc.Path, err)
			}
			trees.Enchant = append(trees.Enchant, es)
		}
	}

	return &trees, nil
}
