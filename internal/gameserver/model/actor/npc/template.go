package npc

import (
	"errors"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// Template holds the static, load-once data for one NPC id: base stats,
// combat parameters, AI tuning, drop table, and (for pets) feeding and
// mount data. The game defines one Template per npc id across the shipped
// data files.
type Template struct {
	ID         int
	TemplateID int

	Name  string
	Title string
	Alias string

	UsingServerSideName  bool
	UsingServerSideTitle bool

	Type          string
	Level         int
	HitTimeFactor float64
	RightHand     int
	LeftHand      int
	RewardExp     float64
	RewardSp      float64

	BaseAttackRange  int
	BaseDamageRange  []int
	BaseRandomDamage int

	Race Race

	STR, CON, DEX, INT, WIT, MEN int

	HPMax, MPMax     float64
	HPRegen, MPRegen float64
	PAtk, MAtk       float64
	PDef, MDef       float64
	AtkSpd           float64
	CritRate         float64
	WalkSpeed        float64
	RunSpeed         float64

	CollisionRadius float64
	CollisionHeight float64

	SSCount, SPSCount int

	Undying       bool
	CanBeAttacked bool
	CorpseTime    int
	NoSleepMode   bool
	AggroRange    int
	CanMove       bool
	Seedable      bool
	CanSeeThrough bool

	// AIParams carries the template's <ai> tuning values verbatim; nil
	// when the template defines no <ai> block. A spawn's own values shadow
	// them (see SpawnSlot).
	AIParams AIParams

	Drops    []item.DropCategory
	Privates []PrivateEntry

	// Clans is the set of aggro-clan names this template belongs to, used
	// to pull in nearby clan-mates when one member is attacked. ClanRange
	// is the radius that search runs at; IgnoredIDs excludes specific npc
	// ids from a clan pull even though they share a clan name.
	Clans      []string
	ClanRange  int
	IgnoredIDs []int

	// TeachTo lists the profession ids this template can train, when it
	// acts as a class (skill) teacher.
	TeachTo []int

	// Pet is non-nil when this template also defines a <petdata> block,
	// i.e. the npc can be tamed/summoned as a pet or mount.
	Pet *PetData

	// SkillsByType maps each non-PASSIVE type of the template's <skills>
	// block to the skill it names; behaviors select the skill they cast
	// through it. A type holds one skill: a later entry with the same type
	// replaces an earlier one. Race-marker entries and unknown id/level
	// pairs never reach it.
	SkillsByType map[SkillType]skill.Ref

	// Skills maps skill id to the level this template grants it: every
	// skill in SkillsByType, at the level of its lowest type when one id is
	// granted under several types at different levels. A pet/servitor may
	// only cast a skill via its owner's commanded action-bar shortcut when
	// its own template grants that skill id.
	Skills map[int]int

	// Passives are the type="PASSIVE" <skill> entries that resolved in the
	// skill table. Race-marker ids never enter this list, and commanded-skill
	// lookups do not search it.
	Passives []skill.Ref
}

// PrivateEntry describes one escort minion a template spawns alongside its
// owner: the minion's own template id, its selection weight among
// siblings, and the respawn delay override applied to its spawn.
type PrivateEntry struct {
	NpcID        int
	Weight       int
	RespawnDelay time.Duration
}

// PetData is the feeding and mount data carried by a template whose npc can
// be tamed or summoned as a pet.
type PetData struct {
	Food1, Food2                              int
	AutoFeedLimit, HungryLimit, UnsummonLimit float64
	Levels                                    map[int]PetLevelStats
}

// PetLevelStats is one level's row of a pet's growth table: its stats
// unmounted, and (when the pet can be ridden) its stats and speed while
// mounted.
type PetLevelStats struct {
	MaxExp                     int64
	MaxMeal                    int
	ExpType                    int
	MealInBattle, MealInNormal int
	PAtk, PDef, MAtk, MDef     float64
	MaxHP, MaxMP               float64
	HPRegen, MPRegen           float64
	SSCount, SPSCount          int

	MountMealInBattle, MountMealInNormal int
	MountAtkSpd, MountPAtk, MountMAtk    float64
	MountBaseSpeed                       int
	MountWaterSpeed                      int
	MountFlySpeed                        int
}

// TemplateSkills is the resolved <skills> block of one template: its
// typed skills and its passives. The zero value is a template with no
// <skills> block.
type TemplateSkills struct {
	ByType   map[SkillType]skill.Ref
	Passives []skill.Ref
}

// NewTemplate returns t with its <skills> block applied: SkillsByType and
// Passives from skills, and Skills derived from them. The loader decodes
// every other field.
func NewTemplate(t Template, skills TemplateSkills) *Template {
	t.SkillsByType = skills.ByType
	t.Passives = skills.Passives
	t.Skills = nil
	if skills.ByType != nil {
		t.Skills = make(map[int]int, len(skills.ByType))
		for typ := range skillTypeCount {
			ref, ok := skills.ByType[typ]
			if !ok {
				continue
			}
			if _, seen := t.Skills[int(ref.ID)]; !seen {
				t.Skills[int(ref.ID)] = ref.Level
			}
		}
	}
	return &t
}

// ErrNoTemplate reports an NPC id no template has.
var ErrNoTemplate = errors.New("npc: no template")

// Table is an in-memory lookup of NPC templates keyed by id, built at boot
// and replaced in place by //reload npc. The zero value is not usable;
// construct with NewTable.
type Table struct {
	*commons.Lookup[int, *Template]
}

// NewTable returns a Table backed by templates, keyed by each template's
// ID. A later entry silently overwrites an earlier one with the same ID.
func NewTable(templates []*Template) *Table {
	return &Table{commons.NewLookup(templates, func(tpl *Template) int { return tpl.ID })}
}

// Replace swaps t's templates for from's, at once for every holder of t:
// NpcData.reload. A live NPC keeps the template it was built from; NPCs
// built afterwards use the new ones.
func (t *Table) Replace(from *Table) {
	t.Swap(from.Lookup)
}

// CollisionBody returns npcID's template collision radius and height, or
// false when no such template is loaded.
func (t *Table) CollisionBody(npcID int32) (radius, height float64, ok bool) {
	tpl, ok := t.Get(int(npcID))
	if !ok {
		return 0, 0, false
	}
	return tpl.CollisionRadius, tpl.CollisionHeight, true
}

// GetByName returns the first template whose name matches name
// case-insensitively, or false if none does.
func (t *Table) GetByName(name string) (*Template, bool) {
	for _, tpl := range t.All() {
		if strings.EqualFold(tpl.Name, name) {
			return tpl, true
		}
	}
	return nil, false
}
