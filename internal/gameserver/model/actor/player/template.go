package player

import (
	"fmt"
	"sort"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
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

// CorrectedCost returns the SP cost shown and checked for learning.
func (g SkillGrant) CorrectedCost() int {
	if g.Cost < 0 {
		return 0
	}
	return g.Cost
}

// SkillLevels maps a known skill id to the character's current level for it.
type SkillLevels map[int]int

// Level returns the known level for skillID, or 0 when it is not known.
func (l SkillLevels) Level(skillID int) int {
	return l[skillID]
}

// LearnStatus describes the result of checking a skill-learning request.
type LearnStatus uint8

const (
	// LearnAllowed means the requested skill can be learned now.
	LearnAllowed LearnStatus = iota
	// LearnUnavailable means the requested skill is not the next learnable
	// level for the character's template, level, and known skills.
	LearnUnavailable
	// LearnNeedsSP means the skill is otherwise learnable but the character
	// does not have enough SP to pay the corrected cost.
	LearnNeedsSP
)

// TemplateTable is an in-memory lookup of player profession templates keyed
// by class id, built once at boot and read for the remainder of the process
// lifetime. The zero value is not usable; construct with NewTemplateTable.
type TemplateTable struct {
	*commons.Lookup[int, *Template]
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
			if id >= 58 && id <= 87 {
				continue
			}
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

	return &TemplateTable{commons.NewLookupFromMap(templates)}, nil
}

// Count returns the number of templates loaded.
func (t *TemplateTable) Count() int {
	return t.Len()
}

// FindSkillGrant returns the exact grant for skillID at level.
func (t *Template) FindSkillGrant(skillID, level int) (SkillGrant, bool) {
	if t == nil || skillID <= 0 || level <= 0 {
		return SkillGrant{}, false
	}
	for _, grant := range t.Skills {
		if grant.SkillID == skillID && grant.Level == level {
			return grant, true
		}
	}
	return SkillGrant{}, false
}

// AvailableSkillGrants returns manual skill grants the character can learn
// at characterLevel, preserving template order.
func (t *Template) AvailableSkillGrants(characterLevel int, known SkillLevels) []SkillGrant {
	if t == nil {
		return nil
	}
	var grants []SkillGrant
	for _, grant := range t.Skills {
		if grant.MinLevel <= characterLevel && grant.Cost != 0 && known.Level(grant.SkillID) == grant.Level-1 {
			grants = append(grants, grant)
		}
	}
	return grants
}

// skillGrantLookahead is how far below a grant's own MinLevel a character
// may sit and still hold the skill. A level loss therefore does not strip
// skills immediately; only a drop of more than this many levels does.
const skillGrantLookahead = 9

// AutoGetSkillGrants returns the free skill grants a character at
// characterLevel is entitled to without ever learning them: the
// highest-level zero-cost grant per skill id whose MinLevel the character
// has reached, skipping any id already known at that level or above. A cost
// of -1 is not free — it marks a grant that is bought but displays a price
// of 0 — so only an exact cost of 0 qualifies. The result is ordered by
// skill id.
func (t *Template) AutoGetSkillGrants(characterLevel int, known SkillLevels) []SkillGrant {
	if t == nil {
		return nil
	}
	highest := make(map[int]SkillGrant)
	for _, grant := range t.Skills {
		if grant.Cost != 0 || grant.MinLevel > characterLevel {
			continue
		}
		if best, ok := highest[grant.SkillID]; ok && best.Level >= grant.Level {
			continue
		}
		highest[grant.SkillID] = grant
	}
	grants := make([]SkillGrant, 0, len(highest))
	for id, grant := range highest {
		if known.Level(id) < grant.Level {
			grants = append(grants, grant)
		}
	}
	sort.Slice(grants, func(i, j int) bool { return grants[i].SkillID < grants[j].SkillID })
	return grants
}

// AllAvailableSkillGrants returns the highest-level unlocked grant per skill
// id, regardless of cost, skipping skills the character already knows at that
// level or above. The result is ordered by skill id.
func (t *Template) AllAvailableSkillGrants(characterLevel int, known SkillLevels) []SkillGrant {
	if t == nil {
		return nil
	}
	highest := make(map[int]SkillGrant)
	for _, grant := range t.Skills {
		if grant.MinLevel > characterLevel {
			continue
		}
		if best, ok := highest[grant.SkillID]; ok && best.Level >= grant.Level {
			continue
		}
		highest[grant.SkillID] = grant
	}
	grants := make([]SkillGrant, 0, len(highest))
	for id, grant := range highest {
		if known.Level(id) < grant.Level {
			grants = append(grants, grant)
		}
	}
	sort.Slice(grants, func(i, j int) bool { return grants[i].SkillID < grants[j].SkillID })
	return grants
}

// ReachableSkillGrants returns the highest-level grant per skill id a
// character at characterLevel is still close enough to hold, keyed by skill
// id. Every skill but expertise stays reachable skillGrantLookahead levels
// before its own MinLevel; expertise is checked against characterLevel
// exactly. A skill id absent from the result is one the character may no
// longer hold at all.
func (t *Template) ReachableSkillGrants(characterLevel int) map[int]SkillGrant {
	if t == nil {
		return nil
	}
	highest := make(map[int]SkillGrant)
	for _, grant := range t.Skills {
		reach := characterLevel + skillGrantLookahead
		if grant.SkillID == int(modelskill.ExpertiseSkillID) {
			reach = characterLevel
		}
		if grant.MinLevel > reach {
			continue
		}
		if best, ok := highest[grant.SkillID]; ok && best.Level >= grant.Level {
			continue
		}
		highest[grant.SkillID] = grant
	}
	return highest
}

// GrantsSkill reports whether this profession line defines any grant for
// skillID. A known skill it does not define came from an item, a quest or a
// temporary award, and profession-level bookkeeping leaves it alone.
func (t *Template) GrantsSkill(skillID int) bool {
	if t == nil {
		return false
	}
	for _, grant := range t.Skills {
		if grant.SkillID == skillID {
			return true
		}
	}
	return false
}

// RequiredLevelForNextSkillGrant returns the lowest future character level
// with a manual skill grant, or 0 when there is none.
func (t *Template) RequiredLevelForNextSkillGrant(characterLevel int) int {
	if t == nil {
		return 0
	}
	next := 0
	for _, grant := range t.Skills {
		if grant.MinLevel <= characterLevel || grant.Cost == 0 {
			continue
		}
		if next == 0 || grant.MinLevel < next {
			next = grant.MinLevel
		}
	}
	return next
}

// CheckSkillLearn checks whether a manual skill grant can be learned now and
// whether availableSP covers its corrected cost. It intentionally does not
// gate on MinLevel or a zero Cost: a general skill learn only checks
// known-level continuity, template membership (which ignores minLvl), and
// SP vs the corrected cost — it has no level or cost==0 rejection of its
// own.
func (t *Template) CheckSkillLearn(characterLevel, availableSP int, known SkillLevels, skillID, level int) (SkillGrant, LearnStatus) {
	grant, ok := t.FindSkillGrant(skillID, level)
	if !ok || known.Level(skillID) != level-1 {
		return SkillGrant{}, LearnUnavailable
	}
	if availableSP < grant.CorrectedCost() {
		return grant, LearnNeedsSP
	}
	return grant, LearnAllowed
}
