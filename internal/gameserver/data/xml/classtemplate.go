package xml

import (
	"encoding/xml"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"

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
	Sets   []attrsElement    `xml:"set"`
	Items  []attrsElement    `xml:"items>item"`
	Skills []attrsElement    `xml:"skills>skill"`
	Spawns []locationElement `xml:"spawns>spawn"`
}

// attrsElement captures every attribute of an element, whatever their
// names, so they can be folded into a StatSet.
type attrsElement struct {
	Attrs []xml.Attr `xml:",any,attr"`
}

// LoadPlayerTemplates parses class template files below dir and returns a
// lookup table keyed by class id, with each template's skills extended across
// its profession line's ancestors. Files that can't be read or parsed are
// skipped; duplicate ids replace earlier templates. A class with a missing or
// mangled required attribute fails the whole load.
func LoadPlayerTemplates(dir string) (*player.TemplateTable, error) {
	docs, err := loadClassDocuments(dir)
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

// loadClassDocuments mirrors PlayerData's recursive, per-file-tolerant load:
// malformed files are ignored while well-formed siblings still contribute.
func loadClassDocuments(dir string) ([]xmlDocument[classFile], error) {
	var docs []xmlDocument[classFile]
	if err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		var doc classFile
		if err := readXML(path, &doc); err == nil {
			docs = append(docs, xmlDocument[classFile]{Path: path, Data: doc})
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("xml: walk class templates in %s: %w", dir, err)
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })
	return docs, nil
}

// loadClassFile parses one class template file and adds its templates to
// templates, keyed by class id.
func loadClassFile(path string, doc classFile, templates map[int]*player.Template) error {
	for _, c := range doc.Classes {
		tmpl, err := buildTemplate(c)
		if err != nil {
			return fmt.Errorf("xml: %s: %w", path, err)
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
			spawn, err := node.loc()
			if err != nil {
				return nil, fmt.Errorf("spawn point: %w", err)
			}
			spawns = append(spawns, spawn)
		}
		set.Set("spawns", spawns)
	}

	return player.NewTemplate(set)
}
