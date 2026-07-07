package xml

import (
	"fmt"
	"strconv"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// skillFile is the root <list> element of one skill definition XML file.
type skillFile struct {
	Skills []skillElement `xml:"skill"`
}

// skillElement is one <skill> element: its own id/name/level-count
// attributes, a set of per-level substitution tables, and the <set>,
// <enchant1> and <enchant2> children that carry the actual attribute values
// (a level's value may reference a table by "#name" instead of a literal).
type skillElement struct {
	ID             string `xml:"id,attr"`
	Name           string `xml:"name,attr"`
	Levels         string `xml:"levels,attr"`
	EnchantLevels1 string `xml:"enchantLevels1,attr"`
	EnchantLevels2 string `xml:"enchantLevels2,attr"`

	Tables   []tableElement `xml:"table"`
	Sets     []setElem      `xml:"set"`
	Enchant1 []setElem      `xml:"enchant1"`
	Enchant2 []setElem      `xml:"enchant2"`
}

// LoadSkillDefinitions parses every ".xml" skill definition file directly
// under dir and returns a lookup table of the resulting definitions, keyed
// by id and level. A directory that can't be listed, a file whose XML is
// not well-formed, or a <skill> element with a missing, mangled, or
// out-of-range attribute fails the whole load: the caller gets an error
// rather than a partially populated table.
func LoadSkillDefinitions(dir string) (*skill.Table, error) {
	docs, err := loadXMLDocuments[skillFile](dir, "skill definition")
	if err != nil {
		return nil, err
	}

	var defs []skill.Definition
	for _, doc := range docs {
		for _, el := range doc.Data.Skills {
			parsed, err := buildSkillDefinitions(el)
			if err != nil {
				return nil, fmt.Errorf("xml: %s: %w", doc.Path, err)
			}
			defs = append(defs, parsed...)
		}
	}

	return skill.NewTable(defs), nil
}

// buildSkillDefinitions expands one <skill> element into one Definition per
// regular level (1..levels) and per enchant level (101.. and 141.. when the
// element declares enchantLevels1/2).
func buildSkillDefinitions(el skillElement) ([]skill.Definition, error) {
	rawID, err := strconv.ParseInt(el.ID, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("skill id %q: %w", el.ID, err)
	}
	id := skill.ID(rawID)

	levels, err := strconv.Atoi(el.Levels)
	if err != nil {
		return nil, fmt.Errorf("skill %d: levels %q: %w", id, el.Levels, err)
	}
	enchant1, err := parseCountAttr(el.EnchantLevels1)
	if err != nil {
		return nil, fmt.Errorf("skill %d: enchantLevels1: %w", id, err)
	}
	enchant2, err := parseCountAttr(el.EnchantLevels2)
	if err != nil {
		return nil, fmt.Errorf("skill %d: enchantLevels2: %w", id, err)
	}

	tables, err := buildValueTables(el.Tables)
	if err != nil {
		return nil, fmt.Errorf("skill %d: %w", id, err)
	}

	defs := make([]skill.Definition, 0, levels+enchant1+enchant2)

	for i := 1; i <= levels; i++ {
		set, err := resolveSkillLevel(tables, el.Sets, i)
		if err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, i, err)
		}
		def, err := skill.NewDefinition(id, i, el.Name, set)
		if err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, i, err)
		}
		defs = append(defs, def)
	}

	// An enchant level's <set>-sourced values reuse the last regular
	// level's table row; only its <enchantN> values vary per enchant level.
	for i := 0; i < enchant1; i++ {
		level := i + 101
		set, err := resolveSkillLevel(tables, el.Sets, levels)
		if err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, level, err)
		}
		if err := applySkillAttrs(set, tables, el.Enchant1, i+1); err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, level, err)
		}
		def, err := skill.NewDefinition(id, level, el.Name, set)
		if err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, level, err)
		}
		defs = append(defs, def)
	}

	for i := 0; i < enchant2; i++ {
		level := i + 141
		set, err := resolveSkillLevel(tables, el.Sets, levels)
		if err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, level, err)
		}
		if err := applySkillAttrs(set, tables, el.Enchant2, i+1); err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, level, err)
		}
		def, err := skill.NewDefinition(id, level, el.Name, set)
		if err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, level, err)
		}
		defs = append(defs, def)
	}

	return defs, nil
}

// resolveSkillLevel builds the StatSet for one level by applying attrs in
// order, resolving any table-referencing value against row tableIndex (the
// level within the referenced table, 1-based).
func resolveSkillLevel(tables map[string][]string, attrs []setElem, tableIndex int) (*commons.StatSet, error) {
	set := commons.NewStatSetWithCapacity(len(attrs))
	if err := applySkillAttrs(set, tables, attrs, tableIndex); err != nil {
		return nil, err
	}
	return set, nil
}

// applySkillAttrs applies attrs to set in order, resolving any
// table-referencing value ("#name") against row tableIndex and overwriting
// whatever the same attribute name already held.
func applySkillAttrs(set *commons.StatSet, tables map[string][]string, attrs []setElem, tableIndex int) error {
	for _, a := range attrs {
		v, err := resolveTableValue(tables, a.Name, a.Val, tableIndex)
		if err != nil {
			return err
		}
		set.Set(a.Name, v)
	}
	return nil
}

// parseCountAttr parses an optional level-count attribute ("enchantLevels1",
// "enchantLevels2"), defaulting to 0 when the element omits it.
func parseCountAttr(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.Atoi(s)
}
