package xml

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/enums/actors"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/template"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/holder/skillnode"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/records"
)

// PlayerData holds every player profession's PlayerTemplate, keyed by class
// id, with each template's skills fed from its ancestor chain
// (PlayerData.java).
type PlayerData struct {
	templates map[int]*template.PlayerTemplate
}

// classListXML is the root <list> element of a classes/*.xml file.
type classListXML struct {
	Classes []classXML `xml:"class"`
}

// classXML is one <class> element. A real file spreads a profession's
// attributes across several <set> children purely for readability;
// buildPlayerTemplate merges them before handing the result to the model
// constructor.
type classXML struct {
	Sets   []attrsXML `xml:"set"`
	Items  []attrsXML `xml:"items>item"`
	Skills []attrsXML `xml:"skills>skill"`
	Spawns []attrsXML `xml:"spawns>spawn"`
}

// attrsXML captures every attribute of an element, whatever their names, so
// they can be folded into a StatSet.
type attrsXML struct {
	Attrs []xml.Attr `xml:",any,attr"`
}

// LoadPlayerData parses every classes/*.xml file in dir and returns the
// resulting templates keyed by class id, with each template's Skills field
// combined with every ancestor profession's granted skills.
func LoadPlayerData(dir string) (*PlayerData, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.xml"))
	if err != nil {
		return nil, fmt.Errorf("xml: list class template files in %s: %w", dir, err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("xml: no class template files found in %s", dir)
	}
	sort.Strings(paths)

	templates := make(map[int]*template.PlayerTemplate)
	for _, path := range paths {
		if err := loadClassFile(path, templates); err != nil {
			return nil, err
		}
	}

	if err := mergeInheritedSkills(templates); err != nil {
		return nil, err
	}

	return &PlayerData{templates: templates}, nil
}

// Template returns the template for class id, and whether one is loaded.
func (d *PlayerData) Template(id int) (*template.PlayerTemplate, bool) {
	t, ok := d.templates[id]
	return t, ok
}

// Count returns the number of class templates loaded.
func (d *PlayerData) Count() int {
	return len(d.templates)
}

// loadClassFile parses one classes/*.xml file and adds its templates to
// templates, keyed by id.
func loadClassFile(path string, templates map[int]*template.PlayerTemplate) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("xml: read %s: %w", path, err)
	}

	var doc classListXML
	if err := xml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("xml: parse %s: %w", path, err)
	}

	for _, c := range doc.Classes {
		tmpl, err := buildPlayerTemplate(c)
		if err != nil {
			return fmt.Errorf("xml: %s: %w", path, err)
		}
		if _, exists := templates[tmpl.ID]; exists {
			return fmt.Errorf("xml: %s: duplicate class template id %d", path, tmpl.ID)
		}
		templates[tmpl.ID] = tmpl
	}
	return nil
}

// buildPlayerTemplate packs one parsed <class> element into the StatSet
// shape template.NewPlayerTemplate consumes — merged <set> attributes plus
// the items/skills/spawnLocations lists — mirroring PlayerData.parseDocument.
func buildPlayerTemplate(c classXML) (*template.PlayerTemplate, error) {
	set := commons.NewStatSetWithCapacity(32)
	for _, s := range c.Sets {
		for _, a := range s.Attrs {
			set.Set(a.Name.Local, a.Value)
		}
	}

	if len(c.Items) > 0 {
		items := make([]records.NewbieItem, 0, len(c.Items))
		for _, node := range c.Items {
			item, err := records.NewNewbieItem(commons.StatSetFromXMLAttrs(node.Attrs))
			if err != nil {
				return nil, fmt.Errorf("starter item: %w", err)
			}
			items = append(items, item)
		}
		set.Set("items", items)
	}

	if len(c.Skills) > 0 {
		skills := make([]skillnode.GeneralSkillNode, 0, len(c.Skills))
		for _, node := range c.Skills {
			skill, err := skillnode.NewGeneralSkillNode(commons.StatSetFromXMLAttrs(node.Attrs))
			if err != nil {
				return nil, fmt.Errorf("skill: %w", err)
			}
			skills = append(skills, skill)
		}
		set.Set("skills", skills)
	}

	if len(c.Spawns) > 0 {
		spawns := make([]location.Location, 0, len(c.Spawns))
		for _, node := range c.Spawns {
			spawn, err := location.NewLocation(commons.StatSetFromXMLAttrs(node.Attrs))
			if err != nil {
				return nil, fmt.Errorf("spawn: %w", err)
			}
			spawns = append(spawns, spawn)
		}
		set.Set("spawnLocations", spawns)
	}

	return template.NewPlayerTemplate(set)
}

// mergeInheritedSkills appends every profession's Skills with its parent's
// (already-merged) Skills, so the final list covers the whole ancestor
// chain, mirroring the parent feed in PlayerData.load(). It processes ids
// in ascending order, which is always parent-before-child (see
// actors.ClassParent), so a single pass fully resolves chains up to three
// tiers deep without recursion.
func mergeInheritedSkills(templates map[int]*template.PlayerTemplate) error {
	ids := make([]int, 0, len(templates))
	for id := range templates {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	for _, id := range ids {
		parentID, ok := actors.ClassParent(id)
		if !ok {
			return fmt.Errorf("xml: class template %d: no known parent mapping", id)
		}
		if parentID < 0 {
			continue
		}
		parent, ok := templates[parentID]
		if !ok {
			return fmt.Errorf("xml: class template %d: parent class %d not loaded", id, parentID)
		}

		tmpl := templates[id]
		merged := make([]skillnode.GeneralSkillNode, 0, len(tmpl.Skills)+len(parent.Skills))
		merged = append(merged, tmpl.Skills...)
		merged = append(merged, parent.Skills...)
		tmpl.Skills = merged
	}
	return nil
}
