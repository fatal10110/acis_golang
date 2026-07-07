package xml

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"

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
	paths, err := filepath.Glob(filepath.Join(dir, "*.xml"))
	if err != nil {
		return nil, fmt.Errorf("xml: list skill tree files in %s: %w", dir, err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("xml: no skill tree files found in %s", dir)
	}
	sort.Strings(paths)

	var trees skill.Trees
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("xml: read %s: %w", path, err)
		}

		var doc skillTreeFile
		if err := xml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("xml: parse %s: %w", path, err)
		}

		for _, el := range doc.Fishing {
			fs, err := skill.NewFishingSkill(commons.StatSetFromXMLAttrs(el.Attrs))
			if err != nil {
				return nil, fmt.Errorf("xml: %s: %w", path, err)
			}
			trees.Fishing = append(trees.Fishing, fs)
		}
		for _, el := range doc.Clan {
			cs, err := skill.NewClanSkill(commons.StatSetFromXMLAttrs(el.Attrs))
			if err != nil {
				return nil, fmt.Errorf("xml: %s: %w", path, err)
			}
			trees.Clan = append(trees.Clan, cs)
		}
		for _, el := range doc.Enchant {
			es, err := skill.NewEnchantSkill(commons.StatSetFromXMLAttrs(el.Attrs))
			if err != nil {
				return nil, fmt.Errorf("xml: %s: %w", path, err)
			}
			trees.Enchant = append(trees.Enchant, es)
		}
	}

	return &trees, nil
}
