package xml

import (
	"encoding/xml"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// classFile is the root <list> element of one class template XML file.
type classFile struct {
	Classes []classElement `xml:"class"`
}

// classElement is one <class> element. A shipped file spreads a
// profession's attributes across several <set> children purely for
// readability; buildTemplate merges them before reading the template.
type classElement struct {
	Sets   []attrsElement    `xml:"set"`
	Items  []attrsElement    `xml:"items>item"`
	Skills []attrsElement    `xml:"skills>skill"`
	Spawns []locationElement `xml:"spawns>spawn"`
}

// attrsElement captures every attribute of an element, whatever their
// names, so they can be folded into a name-keyed value map.
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

// buildTemplate reads one parsed <class> element: the merged <set>
// attributes, later ones winning, plus the items/skills/spawns lists.
func buildTemplate(c classElement) (*player.Template, error) {
	vals := make(map[string]string, 32)
	for _, s := range c.Sets {
		for _, a := range s.Attrs {
			vals[a.Name.Local] = a.Value
		}
	}
	a := newAttrValues(vals, "player template")
	id := a.int("id")
	if err := a.Err(); err != nil {
		return nil, err
	}
	a.prefix = fmt.Sprintf("player template %d", id)

	t := &player.Template{
		ID:          id,
		BaseLevel:   a.int("baseLvl"),
		FistsItemID: a.int("fists"),

		STR: a.intDefault("str", 40),
		CON: a.intDefault("con", 21),
		DEX: a.intDefault("dex", 30),
		INT: a.intDefault("int", 20),
		WIT: a.intDefault("wit", 43),
		MEN: a.intDefault("men", 20),

		PAtk:      a.float64("pAtk"),
		PDef:      a.float64("pDef"),
		MAtk:      a.float64("mAtk"),
		MDef:      a.float64("mDef"),
		RunSpeed:  a.float64Default("runSpd", 1),
		WalkSpeed: a.float64Default("walkSpd", 0),

		SwimSpeed: a.intDefault("swimSpd", 1),

		CollisionRadius:       a.float64("radius"),
		CollisionHeight:       a.float64("height"),
		CollisionRadiusFemale: a.float64("radiusFemale"),
		CollisionHeightFemale: a.float64("heightFemale"),

		HPTable:      a.float64Array("hpTable"),
		MPTable:      a.float64Array("mpTable"),
		CPTable:      a.float64Array("cpTable"),
		HPRegenTable: a.float64Array("hpRegenTable"),
		MPRegenTable: a.float64Array("mpRegenTable"),
		CPRegenTable: a.float64Array("cpRegenTable"),
	}
	safeFall := a.intArray("safeFallHeight")
	if len(safeFall) == 2 {
		t.SafeFallHeightFemale, t.SafeFallHeightMale = safeFall[0], safeFall[1]
	} else {
		a.fail(fmt.Errorf("attribute %q: want 2 values, got %d", "safeFallHeight", len(safeFall)))
	}
	if err := a.Err(); err != nil {
		return nil, err
	}

	if len(c.Items) > 0 {
		t.Items = make([]player.StarterItem, 0, len(c.Items))
		for _, node := range c.Items {
			item, err := buildStarterItem(newAttrValues(foldAttrs(node.Attrs), "player starter item"))
			if err != nil {
				return nil, fmt.Errorf("starter item: %w", err)
			}
			t.Items = append(t.Items, item)
		}
	}

	if len(c.Skills) > 0 {
		t.Skills = make([]player.SkillGrant, 0, len(c.Skills))
		for _, node := range c.Skills {
			skill, err := buildSkillGrant(newAttrValues(foldAttrs(node.Attrs), "player skill grant"))
			if err != nil {
				return nil, fmt.Errorf("skill grant: %w", err)
			}
			t.Skills = append(t.Skills, skill)
		}
	}

	if len(c.Spawns) > 0 {
		t.Spawns = make([]location.Location, 0, len(c.Spawns))
		for _, node := range c.Spawns {
			spawn, err := node.loc()
			if err != nil {
				return nil, fmt.Errorf("spawn point: %w", err)
			}
			t.Spawns = append(t.Spawns, spawn)
		}
	}
	return t, nil
}

// buildStarterItem reads a starter item: id and count are required, and
// isEquipped defaults to true.
func buildStarterItem(a *attrValues) (player.StarterItem, error) {
	item := player.StarterItem{
		ItemID:   a.int("id"),
		Count:    a.int("count"),
		Equipped: a.boolDefault("isEquipped", true),
	}
	if err := a.Err(); err != nil {
		return player.StarterItem{}, err
	}
	return item, nil
}

// buildSkillGrant reads a learnable skill grant; id, lvl, minLvl and cost
// are all required.
func buildSkillGrant(a *attrValues) (player.SkillGrant, error) {
	grant := player.SkillGrant{
		SkillID:  a.int("id"),
		Level:    a.int("lvl"),
		MinLevel: a.int("minLvl"),
		Cost:     a.int("cost"),
	}
	if err := a.Err(); err != nil {
		return player.SkillGrant{}, err
	}
	return grant, nil
}
