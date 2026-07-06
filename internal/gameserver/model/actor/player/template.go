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
	id, err := set.GetInt("id")
	if err != nil {
		return StarterItem{}, err
	}
	count, err := set.GetInt("count")
	if err != nil {
		return StarterItem{}, err
	}
	return StarterItem{ItemID: id, Count: count, Equipped: set.GetBoolDefault("isEquipped", true)}, nil
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
	id, err := set.GetInt("id")
	if err != nil {
		return SkillGrant{}, err
	}
	level, err := set.GetInt("lvl")
	if err != nil {
		return SkillGrant{}, err
	}
	minLevel, err := set.GetInt("minLvl")
	if err != nil {
		return SkillGrant{}, err
	}
	cost, err := set.GetInt("cost")
	if err != nil {
		return SkillGrant{}, err
	}
	return SkillGrant{SkillID: id, Level: level, MinLevel: minLevel, Cost: cost}, nil
}

// NewTemplate builds a Template from set, which carries the merged <set>
// attributes of one <class> element plus the "items", "skills" and "spawns"
// lists the loader packed in.
func NewTemplate(set *commons.StatSet) (*Template, error) {
	id, err := set.GetInt("id")
	if err != nil {
		return nil, fmt.Errorf("player template: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("player template %d: %w", id, err) }

	t := &Template{ID: id}

	if t.BaseLevel, err = set.GetInt("baseLvl"); err != nil {
		return nil, wrap(err)
	}
	if t.FistsItemID, err = set.GetInt("fists"); err != nil {
		return nil, wrap(err)
	}

	if t.STR, err = set.GetInt("str"); err != nil {
		return nil, wrap(err)
	}
	if t.CON, err = set.GetInt("con"); err != nil {
		return nil, wrap(err)
	}
	if t.DEX, err = set.GetInt("dex"); err != nil {
		return nil, wrap(err)
	}
	if t.INT, err = set.GetInt("int"); err != nil {
		return nil, wrap(err)
	}
	if t.WIT, err = set.GetInt("wit"); err != nil {
		return nil, wrap(err)
	}
	if t.MEN, err = set.GetInt("men"); err != nil {
		return nil, wrap(err)
	}

	if t.PAtk, err = set.GetDouble("pAtk"); err != nil {
		return nil, wrap(err)
	}
	if t.PDef, err = set.GetDouble("pDef"); err != nil {
		return nil, wrap(err)
	}
	if t.MAtk, err = set.GetDouble("mAtk"); err != nil {
		return nil, wrap(err)
	}
	if t.MDef, err = set.GetDouble("mDef"); err != nil {
		return nil, wrap(err)
	}
	if t.RunSpeed, err = set.GetDouble("runSpd"); err != nil {
		return nil, wrap(err)
	}
	if t.WalkSpeed, err = set.GetDouble("walkSpd"); err != nil {
		return nil, wrap(err)
	}

	// swimSpd is optional, but a present value that fails to parse must
	// still surface: the default substitutes only for an absent key.
	t.SwimSpeed = 1
	if set.Has("swimSpd") {
		if t.SwimSpeed, err = set.GetInt("swimSpd"); err != nil {
			return nil, wrap(err)
		}
	}

	if t.CollisionRadius, err = set.GetDouble("radius"); err != nil {
		return nil, wrap(err)
	}
	if t.CollisionHeight, err = set.GetDouble("height"); err != nil {
		return nil, wrap(err)
	}
	if t.CollisionRadiusFemale, err = set.GetDouble("radiusFemale"); err != nil {
		return nil, wrap(err)
	}
	if t.CollisionHeightFemale, err = set.GetDouble("heightFemale"); err != nil {
		return nil, wrap(err)
	}

	safeFall, err := set.GetIntArray("safeFallHeight")
	if err != nil {
		return nil, wrap(err)
	}
	if len(safeFall) != 2 {
		return nil, wrap(fmt.Errorf("attribute %q: want 2 values, got %d", "safeFallHeight", len(safeFall)))
	}
	t.SafeFallHeightFemale, t.SafeFallHeightMale = safeFall[0], safeFall[1]

	if t.HPTable, err = set.GetDoubleArray("hpTable"); err != nil {
		return nil, wrap(err)
	}
	if t.MPTable, err = set.GetDoubleArray("mpTable"); err != nil {
		return nil, wrap(err)
	}
	if t.CPTable, err = set.GetDoubleArray("cpTable"); err != nil {
		return nil, wrap(err)
	}
	if t.HPRegenTable, err = set.GetDoubleArray("hpRegenTable"); err != nil {
		return nil, wrap(err)
	}
	if t.MPRegenTable, err = set.GetDoubleArray("mpRegenTable"); err != nil {
		return nil, wrap(err)
	}
	if t.CPRegenTable, err = set.GetDoubleArray("cpRegenTable"); err != nil {
		return nil, wrap(err)
	}

	if t.Items, err = commons.GetList[StarterItem](set, "items"); err != nil {
		return nil, wrap(err)
	}
	if t.Skills, err = commons.GetList[SkillGrant](set, "skills"); err != nil {
		return nil, wrap(err)
	}
	if t.Spawns, err = commons.GetList[location.Location](set, "spawns"); err != nil {
		return nil, wrap(err)
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
