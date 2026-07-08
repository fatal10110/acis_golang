package player

import (
	"fmt"
	"sort"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Template holds the base stats, starter equipment, spawn points and
// learnable skills for one player profession (e.g. Human Fighter, Warrior,
// Duelist). The game defines one Template per profession id, forming a tree
// that starts at 9 base professions and runs three tiers deep; see
// ClassParent.
type Template struct {
	ID int

	// BaseLevel is the character level required to take this profession.
	BaseLevel int

	// FistsItemID is the weapon id used when a character of this profession
	// has nothing equipped. Resolving it to an item template is the item
	// table's job, not this type's.
	FistsItemID int

	STR, CON, DEX, INT, WIT, MEN int

	PAtk, PDef, MAtk, MDef float64
	RunSpeed, WalkSpeed    float64
	SwimSpeed              int

	CollisionRadius, CollisionHeight             float64
	CollisionRadiusFemale, CollisionHeightFemale float64

	// SafeFallHeight{Female,Male} is the fall distance, in units, a
	// character of this profession can drop without taking damage. The data
	// stores the female value first.
	SafeFallHeightFemale, SafeFallHeightMale int

	// {HP,MP,CP}Table and their Regen counterparts are indexed by level-1,
	// giving the max/regen value at every character level.
	HPTable, MPTable, CPTable                []float64
	HPRegenTable, MPRegenTable, CPRegenTable []float64

	// Items and Spawns are populated for the 9 base professions only; every
	// other profession in the tree carries none of its own.
	Items  []StarterItem
	Spawns []location.Location

	// Skills holds this profession's own learnable skills; NewTemplateTable
	// appends every ancestor profession's afterwards, so a character on
	// this line can learn anything the line ever unlocked.
	Skills []SkillGrant
}

// StarterItem is one piece of starter equipment granted to a freshly
// created character of a base profession.
type StarterItem struct {
	ItemID   int
	Count    int
	Equipped bool
}

// NewStarterItem builds a StarterItem from set. id and count are required;
// isEquipped defaults to true when absent.
func NewStarterItem(set *commons.StatSet) (StarterItem, error) {
	f := commons.NewFields(set, "player starter item")
	item := StarterItem{
		ItemID:   f.Int("id"),
		Count:    f.Int("count"),
		Equipped: f.BoolDefault("isEquipped", true),
	}
	if err := f.Err(); err != nil {
		return StarterItem{}, err
	}
	return item, nil
}

// SkillGrant is one skill/level combination a character may learn, along
// with its SP cost and the character level required to learn it.
type SkillGrant struct {
	SkillID int
	Level   int
	// MinLevel is the character level required to learn this grant.
	MinLevel int
	// Cost is the SP cost. A cost of -1 marks a grant that is given
	// automatically but must still display a price of 0 to the client; 0
	// itself would make it a freely-learned skill.
	Cost int
}

// NewSkillGrant builds a SkillGrant from set; id, lvl, minLvl and cost are
// all required.
func NewSkillGrant(set *commons.StatSet) (SkillGrant, error) {
	f := commons.NewFields(set, "player skill grant")
	grant := SkillGrant{
		SkillID:  f.Int("id"),
		Level:    f.Int("lvl"),
		MinLevel: f.Int("minLvl"),
		Cost:     f.Int("cost"),
	}
	if err := f.Err(); err != nil {
		return SkillGrant{}, err
	}
	return grant, nil
}

// NewTemplate builds a Template from set, which carries the merged <set>
// attributes of one <class> element plus the "items", "skills" and "spawns"
// lists the loader packed in.
func NewTemplate(set *commons.StatSet) (*Template, error) {
	idf := commons.NewFields(set, "player template")
	id := idf.Int("id")
	if err := idf.Err(); err != nil {
		return nil, err
	}

	f := commons.NewFields(set, fmt.Sprintf("player template %d", id))
	t := &Template{
		ID:          id,
		BaseLevel:   f.Int("baseLvl"),
		FistsItemID: f.Int("fists"),

		STR: f.Int("str"),
		CON: f.Int("con"),
		DEX: f.Int("dex"),
		INT: f.Int("int"),
		WIT: f.Int("wit"),
		MEN: f.Int("men"),

		PAtk:      f.Double("pAtk"),
		PDef:      f.Double("pDef"),
		MAtk:      f.Double("mAtk"),
		MDef:      f.Double("mDef"),
		RunSpeed:  f.Double("runSpd"),
		WalkSpeed: f.Double("walkSpd"),

		SwimSpeed: f.IntDefault("swimSpd", 1),

		CollisionRadius:       f.Double("radius"),
		CollisionHeight:       f.Double("height"),
		CollisionRadiusFemale: f.Double("radiusFemale"),
		CollisionHeightFemale: f.Double("heightFemale"),

		HPTable:      f.DoubleArray("hpTable"),
		MPTable:      f.DoubleArray("mpTable"),
		CPTable:      f.DoubleArray("cpTable"),
		HPRegenTable: f.DoubleArray("hpRegenTable"),
		MPRegenTable: f.DoubleArray("mpRegenTable"),
		CPRegenTable: f.DoubleArray("cpRegenTable"),

		Items:  commons.FieldList[StarterItem](f, "items"),
		Skills: commons.FieldList[SkillGrant](f, "skills"),
		Spawns: commons.FieldList[location.Location](f, "spawns"),
	}

	safeFall := f.IntArray("safeFallHeight")
	if len(safeFall) != 2 {
		f.Fail(fmt.Errorf("attribute %q: want 2 values, got %d", "safeFallHeight", len(safeFall)))
	} else {
		t.SafeFallHeightFemale, t.SafeFallHeightMale = safeFall[0], safeFall[1]
	}

	if err := f.Err(); err != nil {
		return nil, err
	}
	return t, nil
}

// TemplateTable is an in-memory lookup of player profession templates keyed
// by class id, built once at boot and read for the remainder of the process
// lifetime. The zero value is not usable; construct with NewTemplateTable.
type TemplateTable struct {
	templates map[int]*Template
}

// NewTemplateTable returns a TemplateTable backed by templates, keyed by
// class id, after resolving the profession tree: every template's Skills
// list is extended with its ancestors' so each profession can learn
// anything its line ever unlocked. It returns an error for a class id with
// no ClassParent entry or with a parent that isn't in templates.
//
// Ids are processed in ascending order, which is always parent-before-child
// (see classParent), so a single pass fully resolves chains up to three
// tiers deep without recursion.
func NewTemplateTable(templates map[int]*Template) (*TemplateTable, error) {
	ids := make([]int, 0, len(templates))
	for id := range templates {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	for _, id := range ids {
		parentID, ok := ClassParent(id)
		if !ok {
			return nil, fmt.Errorf("player: class template %d: unknown profession id", id)
		}
		if parentID < 0 {
			continue
		}
		parent, ok := templates[parentID]
		if !ok {
			return nil, fmt.Errorf("player: class template %d: parent class %d not loaded", id, parentID)
		}

		tmpl := templates[id]
		merged := make([]SkillGrant, 0, len(tmpl.Skills)+len(parent.Skills))
		merged = append(merged, tmpl.Skills...)
		merged = append(merged, parent.Skills...)
		tmpl.Skills = merged
	}

	return &TemplateTable{templates: templates}, nil
}

// Get returns the template for class id, or false if none was loaded.
func (t *TemplateTable) Get(id int) (*Template, bool) {
	tmpl, ok := t.templates[id]
	return tmpl, ok
}

// Count returns the number of templates loaded.
func (t *TemplateTable) Count() int {
	return len(t.templates)
}

// All returns every loaded template, ordered ascending by class ID.
func (t *TemplateTable) All() []*Template {
	templates := make([]*Template, 0, len(t.templates))
	for _, tmpl := range t.templates {
		templates = append(templates, tmpl)
	}
	sort.Slice(templates, func(i, j int) bool { return templates[i].ID < templates[j].ID })
	return templates
}
