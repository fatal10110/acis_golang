package xml

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ClassTemplate holds the base stats, starter equipment, spawn points and
// granted skills for one player profession (e.g. Human Fighter, Warrior,
// Duelist). The game defines one ClassTemplate per profession id, forming a
// tree that starts at 9 base professions and runs three tiers deep.
type ClassTemplate struct {
	ID int

	// BaseLevel is the character level required to take this profession.
	BaseLevel int

	// FistsItemID is the weapon id used when a character of this profession
	// has nothing equipped. Resolving it to an actual item template is the
	// job of the item template loader, not this package.
	FistsItemID int

	STR, CON, DEX, INT, WIT, MEN int

	PAtk, PDef, MAtk, MDef float64
	RunSpeed, WalkSpeed    float64
	SwimSpeed              int

	CollisionRadius, CollisionHeight             float64
	CollisionRadiusFemale, CollisionHeightFemale float64

	// SafeFallHeight{Female,Male} is the fall distance, in units, a
	// character of this profession can drop without taking damage.
	SafeFallHeightFemale, SafeFallHeightMale int

	// {HP,MP,CP}Table and their Regen counterparts are indexed by
	// level-1, giving the max/regen value at every character level.
	HPTable, MPTable, CPTable                []float64
	HPRegenTable, MPRegenTable, CPRegenTable []float64

	// Items and SpawnPoints are populated for the 9 base professions only;
	// every other profession in the tree carries none of its own.
	Items       []StarterItem
	SpawnPoints []SpawnPoint

	// Skills holds this profession's own granted skills followed by every
	// ancestor profession's granted skills, so a character on this
	// profession's line can learn anything the line ever unlocked.
	Skills []SkillGrant
}

// StarterItem is one piece of starter equipment granted to a freshly
// created character of a base profession.
type StarterItem struct {
	ItemID   int
	Count    int
	Equipped bool
}

// SkillGrant is one skill/level combination a character may learn, along
// with its SP cost and the character level required to learn it.
type SkillGrant struct {
	SkillID  int
	Level    int
	MinLevel int
	Cost     int
}

// SpawnPoint is one candidate world location where a freshly created
// character of a base profession appears.
type SpawnPoint struct {
	X, Y, Z int
}

// classParentID maps a class template's id to the id of the profession it
// upgrades from, or -1 for one of the 9 base professions. It only covers ids
// that actually appear in classes/*.xml: 0-57 for the base, first and second
// tier professions across the 9 lines, and 88-118 for the third tier. The 30
// ids in between are reserved by the data format and never assigned to a
// profession, so they are omitted here.
//
// mergeInheritedSkills relies on every parent id being numerically smaller
// than its children's - true for every entry below - to resolve the full,
// up-to-three-tier, skill inheritance chain with one ascending pass instead
// of recursion.
var classParentID = map[int]int{
	0: -1, 1: 0, 2: 1, 3: 1, 4: 0, 5: 4, 6: 4, 7: 0, 8: 7, 9: 7,
	10: -1, 11: 10, 12: 11, 13: 11, 14: 11, 15: 10, 16: 15, 17: 15,
	18: -1, 19: 18, 20: 19, 21: 19, 22: 18, 23: 22, 24: 22,
	25: -1, 26: 25, 27: 26, 28: 26, 29: 25, 30: 29,
	31: -1, 32: 31, 33: 32, 34: 32, 35: 31, 36: 35, 37: 35,
	38: -1, 39: 38, 40: 39, 41: 39, 42: 38, 43: 42,
	44: -1, 45: 44, 46: 45, 47: 44, 48: 47,
	49: -1, 50: 49, 51: 50, 52: 50,
	53: -1, 54: 53, 55: 54, 56: 53, 57: 56,

	88: 2, 89: 3, 90: 5, 91: 6, 92: 9, 93: 8, 94: 12, 95: 13, 96: 14, 97: 16, 98: 17,
	99: 20, 100: 21, 101: 23, 102: 24, 103: 27, 104: 28, 105: 30,
	106: 33, 107: 34, 108: 36, 109: 37, 110: 40, 111: 41, 112: 43,
	113: 46, 114: 48, 115: 51, 116: 52,
	117: 55, 118: 57,
}

// LoadClassTemplates parses every classes/*.xml file in dir and returns the
// resulting templates keyed by class id, with each template's Skills field
// combined with every ancestor profession's granted skills.
func LoadClassTemplates(dir string) (map[int]*ClassTemplate, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.xml"))
	if err != nil {
		return nil, fmt.Errorf("xml: list class template files in %s: %w", dir, err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("xml: no class template files found in %s", dir)
	}
	sort.Strings(paths)

	templates := make(map[int]*ClassTemplate)
	for _, path := range paths {
		if err := loadClassFile(path, templates); err != nil {
			return nil, err
		}
	}

	if err := mergeInheritedSkills(templates); err != nil {
		return nil, err
	}

	return templates, nil
}

// loadClassFile parses one classes/*.xml file and adds its templates to
// templates, keyed by id.
func loadClassFile(path string, templates map[int]*ClassTemplate) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("xml: read %s: %w", path, err)
	}

	var doc classListXML
	if err := xml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("xml: parse %s: %w", path, err)
	}

	for _, c := range doc.Classes {
		tmpl, err := buildClassTemplate(c)
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

// mergeInheritedSkills appends every profession's Skills with its parent's
// (already-merged) Skills, so the final list covers the whole ancestor
// chain. It processes ids in ascending order, which is always parent-before-
// child (see classParentID), so a single pass fully resolves chains up to
// three tiers deep without recursion.
func mergeInheritedSkills(templates map[int]*ClassTemplate) error {
	ids := make([]int, 0, len(templates))
	for id := range templates {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	for _, id := range ids {
		parentID, ok := classParentID[id]
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
		merged := make([]SkillGrant, 0, len(tmpl.Skills)+len(parent.Skills))
		merged = append(merged, tmpl.Skills...)
		merged = append(merged, parent.Skills...)
		tmpl.Skills = merged
	}
	return nil
}

// classListXML is the root <list> element of a classes/*.xml file.
type classListXML struct {
	Classes []classXML `xml:"class"`
}

// classXML is one <class> element. Its <set> children each carry a subset
// of the profession's attributes; a real file spreads them across several
// <set> elements purely for readability, so buildClassTemplate merges them
// before extracting any field.
type classXML struct {
	Sets   []attrsXML `xml:"set"`
	Items  *itemsXML  `xml:"items"`
	Skills *skillsXML `xml:"skills"`
	Spawns *spawnsXML `xml:"spawns"`
}

// attrsXML captures every attribute of a <set> element, whatever their
// names, so they can be merged across a <class> element's several <set>
// children.
type attrsXML struct {
	Attrs []xml.Attr `xml:",any,attr"`
}

type itemsXML struct {
	Items []itemXML `xml:"item"`
}

type itemXML struct {
	ID       int    `xml:"id,attr"`
	Count    int    `xml:"count,attr"`
	Equipped string `xml:"isEquipped,attr"`
}

type skillsXML struct {
	Skills []skillXML `xml:"skill"`
}

type skillXML struct {
	ID       int `xml:"id,attr"`
	Level    int `xml:"lvl,attr"`
	Cost     int `xml:"cost,attr"`
	MinLevel int `xml:"minLvl,attr"`
}

type spawnsXML struct {
	Spawns []spawnXML `xml:"spawn"`
}

type spawnXML struct {
	X int `xml:"x,attr"`
	Y int `xml:"y,attr"`
	Z int `xml:"z,attr"`
}

// buildClassTemplate converts one parsed <class> element into a
// ClassTemplate, merging all of its <set> elements' attributes first.
func buildClassTemplate(c classXML) (*ClassTemplate, error) {
	attrs := make(attrSet, 32)
	for _, set := range c.Sets {
		for _, a := range set.Attrs {
			attrs[a.Name.Local] = a.Value
		}
	}

	id, err := attrs.intAttr("id")
	if err != nil {
		return nil, fmt.Errorf("class template: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("class template %d: %w", id, err) }

	t := &ClassTemplate{ID: id}

	if t.BaseLevel, err = attrs.intAttr("baseLvl"); err != nil {
		return nil, wrap(err)
	}
	if t.FistsItemID, err = attrs.intAttr("fists"); err != nil {
		return nil, wrap(err)
	}

	if t.STR, err = attrs.intAttr("str"); err != nil {
		return nil, wrap(err)
	}
	if t.CON, err = attrs.intAttr("con"); err != nil {
		return nil, wrap(err)
	}
	if t.DEX, err = attrs.intAttr("dex"); err != nil {
		return nil, wrap(err)
	}
	if t.INT, err = attrs.intAttr("int"); err != nil {
		return nil, wrap(err)
	}
	if t.WIT, err = attrs.intAttr("wit"); err != nil {
		return nil, wrap(err)
	}
	if t.MEN, err = attrs.intAttr("men"); err != nil {
		return nil, wrap(err)
	}

	if t.PAtk, err = attrs.floatAttr("pAtk"); err != nil {
		return nil, wrap(err)
	}
	if t.PDef, err = attrs.floatAttr("pDef"); err != nil {
		return nil, wrap(err)
	}
	if t.MAtk, err = attrs.floatAttr("mAtk"); err != nil {
		return nil, wrap(err)
	}
	if t.MDef, err = attrs.floatAttr("mDef"); err != nil {
		return nil, wrap(err)
	}
	if t.RunSpeed, err = attrs.floatAttr("runSpd"); err != nil {
		return nil, wrap(err)
	}
	if t.WalkSpeed, err = attrs.floatAttr("walkSpd"); err != nil {
		return nil, wrap(err)
	}
	t.SwimSpeed = attrs.intAttrDefault("swimSpd", 1)

	if t.CollisionRadius, err = attrs.floatAttr("radius"); err != nil {
		return nil, wrap(err)
	}
	if t.CollisionHeight, err = attrs.floatAttr("height"); err != nil {
		return nil, wrap(err)
	}
	if t.CollisionRadiusFemale, err = attrs.floatAttr("radiusFemale"); err != nil {
		return nil, wrap(err)
	}
	if t.CollisionHeightFemale, err = attrs.floatAttr("heightFemale"); err != nil {
		return nil, wrap(err)
	}

	safeFall, err := attrs.intListAttr("safeFallHeight")
	if err != nil {
		return nil, wrap(err)
	}
	if len(safeFall) != 2 {
		return nil, wrap(fmt.Errorf("attribute %q: want 2 values, got %d", "safeFallHeight", len(safeFall)))
	}
	t.SafeFallHeightFemale, t.SafeFallHeightMale = safeFall[0], safeFall[1]

	if t.HPTable, err = attrs.floatListAttr("hpTable"); err != nil {
		return nil, wrap(err)
	}
	if t.MPTable, err = attrs.floatListAttr("mpTable"); err != nil {
		return nil, wrap(err)
	}
	if t.CPTable, err = attrs.floatListAttr("cpTable"); err != nil {
		return nil, wrap(err)
	}
	if t.HPRegenTable, err = attrs.floatListAttr("hpRegenTable"); err != nil {
		return nil, wrap(err)
	}
	if t.MPRegenTable, err = attrs.floatListAttr("mpRegenTable"); err != nil {
		return nil, wrap(err)
	}
	if t.CPRegenTable, err = attrs.floatListAttr("cpRegenTable"); err != nil {
		return nil, wrap(err)
	}

	if c.Items != nil {
		t.Items = make([]StarterItem, 0, len(c.Items.Items))
		for _, it := range c.Items.Items {
			equipped := true
			if it.Equipped != "" {
				equipped = strings.EqualFold(it.Equipped, "true")
			}
			t.Items = append(t.Items, StarterItem{ItemID: it.ID, Count: it.Count, Equipped: equipped})
		}
	}

	if c.Skills != nil {
		t.Skills = make([]SkillGrant, 0, len(c.Skills.Skills))
		for _, sk := range c.Skills.Skills {
			t.Skills = append(t.Skills, SkillGrant{SkillID: sk.ID, Level: sk.Level, MinLevel: sk.MinLevel, Cost: sk.Cost})
		}
	}

	if c.Spawns != nil {
		t.SpawnPoints = make([]SpawnPoint, 0, len(c.Spawns.Spawns))
		for _, sp := range c.Spawns.Spawns {
			t.SpawnPoints = append(t.SpawnPoints, SpawnPoint{X: sp.X, Y: sp.Y, Z: sp.Z})
		}
	}

	return t, nil
}

// attrSet is the merged set of a <class> element's <set> attributes, keyed
// by attribute name.
type attrSet map[string]string

func (a attrSet) require(key string) (string, error) {
	v, ok := a[key]
	if !ok {
		return "", fmt.Errorf("missing attribute %q", key)
	}
	return v, nil
}

func (a attrSet) intAttr(key string) (int, error) {
	v, err := a.require(key)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("attribute %q: %w", key, err)
	}
	return n, nil
}

func (a attrSet) intAttrDefault(key string, def int) int {
	v, ok := a[key]
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func (a attrSet) floatAttr(key string) (float64, error) {
	v, err := a.require(key)
	if err != nil {
		return 0, err
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("attribute %q: %w", key, err)
	}
	return f, nil
}

func (a attrSet) intListAttr(key string) ([]int, error) {
	v, err := a.require(key)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(v, ";")
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("attribute %q: %w", key, err)
		}
		out[i] = n
	}
	return out, nil
}

func (a attrSet) floatListAttr(key string) ([]float64, error) {
	v, err := a.require(key)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(v, ";")
	out := make([]float64, len(parts))
	for i, p := range parts {
		f, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return nil, fmt.Errorf("attribute %q: %w", key, err)
		}
		out[i] = f
	}
	return out, nil
}
