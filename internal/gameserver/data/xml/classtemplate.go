package xml

import (
	"encoding/xml"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// classFile is the root <list> element of one class template XML file.
type classFile struct {
	Classes []classElement `xml:"class"`
}

// classElement is one <class> element. A shipped file spreads a
// profession's attributes across several <set> children purely for
// readability; buildTemplate merges them before handing the result to the
// model constructor.
type classElement struct {
	Sets   []attrsElement `xml:"set"`
	Items  []attrsElement `xml:"items>item"`
	Skills []attrsElement `xml:"skills>skill"`
	Spawns []attrsElement `xml:"spawns>spawn"`
}

// attrsElement captures every attribute of an element, whatever their
// names, so they can be folded into a StatSet.
type attrsElement struct {
	Attrs []xml.Attr `xml:",any,attr"`
}

// LoadPlayerTemplates parses every ".xml" class template file directly
// under dir and returns a lookup table of the resulting templates keyed by
// class id, with each template's skills extended across its profession
// line's ancestors. A file that can't be read or parsed, a duplicated class
// id, or a class with a missing or mangled attribute fails the whole load.
func LoadPlayerTemplates(dir string) (*player.TemplateTable, error) {
	docs, err := loadXMLDocuments[classFile](dir, "class template")
	if err != nil {
		return nil, err
	}

	templates := make(map[int]*player.Template)
	for _, doc := range docs {
		if err := loadClassFile(doc.Path, doc.Data, templates); err != nil {
			return nil, err
		}
	}

	table, err := player.NewTemplateTable(templates)
	if err != nil {
		return nil, fmt.Errorf("xml: class templates in %s: %w", dir, err)
	}
	return table, nil
}

// loadClassFile parses one class template file and adds its templates to
// templates, keyed by class id.
func loadClassFile(path string, doc classFile, templates map[int]*player.Template) error {
	for _, c := range doc.Classes {
		tmpl, err := buildTemplate(c)
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

// buildTemplate packs one parsed <class> element into the StatSet shape
// player.NewTemplate consumes: the merged <set> attributes plus the
// items/skills/spawns lists.
func buildTemplate(c classElement) (*player.Template, error) {
	set := commons.NewStatSetWithCapacity(32)
	for _, s := range c.Sets {
		for _, a := range s.Attrs {
			set.Set(a.Name.Local, a.Value)
		}
	}

	if len(c.Items) > 0 {
		items := make([]player.StarterItem, 0, len(c.Items))
		for _, node := range c.Items {
			item, err := player.NewStarterItem(commons.StatSetFromXMLAttrs(node.Attrs))
			if err != nil {
				return nil, fmt.Errorf("starter item: %w", err)
			}
			items = append(items, item)
		}
		set.Set("items", items)
	}

	if len(c.Skills) > 0 {
		skills := make([]player.SkillGrant, 0, len(c.Skills))
		for _, node := range c.Skills {
			skill, err := player.NewSkillGrant(commons.StatSetFromXMLAttrs(node.Attrs))
			if err != nil {
				return nil, fmt.Errorf("skill grant: %w", err)
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
				return nil, fmt.Errorf("spawn point: %w", err)
			}
			spawns = append(spawns, spawn)
		}
		set.Set("spawns", spawns)
	}

	return player.NewTemplate(set)
}
