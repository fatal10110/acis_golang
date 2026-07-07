package npc

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
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

	// AIParams carries the template's <ai> tuning values verbatim; it is
	// never nil, even when the template defines no <ai> block.
	AIParams *commons.StatSet

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
}

// PrivateEntry describes one escort minion a template spawns alongside its
// owner: the minion's own template id, its selection weight among
// siblings, and the respawn delay override applied to its spawn.
type PrivateEntry struct {
	NpcID        int
	Weight       int
	RespawnDelay time.Duration
}

// NewPrivateEntry builds a PrivateEntry from set, the folded attributes of
// one <private> element. id, weight and respawn are all required.
func NewPrivateEntry(set *commons.StatSet) (PrivateEntry, error) {
	id, err := set.GetInt("id")
	if err != nil {
		return PrivateEntry{}, fmt.Errorf("npc: private entry: %w", err)
	}
	weight, err := set.GetInt("weight")
	if err != nil {
		return PrivateEntry{}, fmt.Errorf("npc: private entry %d: %w", id, err)
	}
	respawnStr, err := set.GetString("respawn")
	if err != nil {
		return PrivateEntry{}, fmt.Errorf("npc: private entry %d: %w", id, err)
	}
	respawn, err := commons.ParseGameDuration(respawnStr)
	if err != nil {
		return PrivateEntry{}, fmt.Errorf("npc: private entry %d: %w", id, err)
	}
	return PrivateEntry{NpcID: id, Weight: weight, RespawnDelay: respawn}, nil
}

// PetData is the feeding and mount data carried by a template whose npc can
// be tamed or summoned as a pet.
type PetData struct {
	Food1, Food2                              int
	AutoFeedLimit, HungryLimit, UnsummonLimit float64
	Levels                                    map[int]PetLevelStats
}

// NewPetData builds a PetData from set, the folded attributes of one
// <petdata> element, and its already-parsed per-level stat rows. Every
// attribute is required.
func NewPetData(set *commons.StatSet, levels map[int]PetLevelStats) (*PetData, error) {
	food1, err := set.GetInt("food1")
	if err != nil {
		return nil, fmt.Errorf("npc: pet data: %w", err)
	}
	food2, err := set.GetInt("food2")
	if err != nil {
		return nil, fmt.Errorf("npc: pet data: %w", err)
	}
	autoFeedLimit, err := set.GetDouble("autoFeedLimit")
	if err != nil {
		return nil, fmt.Errorf("npc: pet data: %w", err)
	}
	hungryLimit, err := set.GetDouble("hungryLimit")
	if err != nil {
		return nil, fmt.Errorf("npc: pet data: %w", err)
	}
	unsummonLimit, err := set.GetDouble("unsummonLimit")
	if err != nil {
		return nil, fmt.Errorf("npc: pet data: %w", err)
	}
	return &PetData{
		Food1: food1, Food2: food2,
		AutoFeedLimit: autoFeedLimit, HungryLimit: hungryLimit, UnsummonLimit: unsummonLimit,
		Levels: levels,
	}, nil
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

// NewPetLevelStats builds a PetLevelStats from set, the folded attributes
// of one <stat> element. The mounted fields default to 0 when the row
// defines no "*OnRide"/"speedOnRide" attributes at all, matching a pet that
// cannot be ridden at this level.
func NewPetLevelStats(set *commons.StatSet) (PetLevelStats, error) {
	s := PetLevelStats{}
	var err error

	if s.MaxExp, err = set.GetLong("exp"); err != nil {
		return PetLevelStats{}, fmt.Errorf("npc: pet level stats: %w", err)
	}
	if s.MaxMeal, err = set.GetInt("maxMeal"); err != nil {
		return PetLevelStats{}, fmt.Errorf("npc: pet level stats: %w", err)
	}
	if s.ExpType, err = set.GetInt("expType"); err != nil {
		return PetLevelStats{}, fmt.Errorf("npc: pet level stats: %w", err)
	}
	if s.MealInBattle, err = set.GetInt("mealInBattle"); err != nil {
		return PetLevelStats{}, fmt.Errorf("npc: pet level stats: %w", err)
	}
	if s.MealInNormal, err = set.GetInt("mealInNormal"); err != nil {
		return PetLevelStats{}, fmt.Errorf("npc: pet level stats: %w", err)
	}
	if s.PAtk, err = set.GetDouble("pAtk"); err != nil {
		return PetLevelStats{}, fmt.Errorf("npc: pet level stats: %w", err)
	}
	if s.PDef, err = set.GetDouble("pDef"); err != nil {
		return PetLevelStats{}, fmt.Errorf("npc: pet level stats: %w", err)
	}
	if s.MAtk, err = set.GetDouble("mAtk"); err != nil {
		return PetLevelStats{}, fmt.Errorf("npc: pet level stats: %w", err)
	}
	if s.MDef, err = set.GetDouble("mDef"); err != nil {
		return PetLevelStats{}, fmt.Errorf("npc: pet level stats: %w", err)
	}
	if s.MaxHP, err = set.GetDouble("hp"); err != nil {
		return PetLevelStats{}, fmt.Errorf("npc: pet level stats: %w", err)
	}
	if s.MaxMP, err = set.GetDouble("mp"); err != nil {
		return PetLevelStats{}, fmt.Errorf("npc: pet level stats: %w", err)
	}
	if s.HPRegen, err = set.GetDouble("hpRegen"); err != nil {
		return PetLevelStats{}, fmt.Errorf("npc: pet level stats: %w", err)
	}
	if s.MPRegen, err = set.GetDouble("mpRegen"); err != nil {
		return PetLevelStats{}, fmt.Errorf("npc: pet level stats: %w", err)
	}
	if s.SSCount, err = set.GetInt("ssCount"); err != nil {
		return PetLevelStats{}, fmt.Errorf("npc: pet level stats: %w", err)
	}
	if s.SPSCount, err = set.GetInt("spsCount"); err != nil {
		return PetLevelStats{}, fmt.Errorf("npc: pet level stats: %w", err)
	}

	if s.MountMealInBattle, err = set.GetIntDefault("mealInBattleOnRide", 0); err != nil {
		return PetLevelStats{}, fmt.Errorf("npc: pet level stats: %w", err)
	}
	if s.MountMealInNormal, err = set.GetIntDefault("mealInNormalOnRide", 0); err != nil {
		return PetLevelStats{}, fmt.Errorf("npc: pet level stats: %w", err)
	}
	if s.MountAtkSpd, err = set.GetDoubleDefault("atkSpdOnRide", 0); err != nil {
		return PetLevelStats{}, fmt.Errorf("npc: pet level stats: %w", err)
	}
	if s.MountPAtk, err = set.GetDoubleDefault("pAtkOnRide", 0); err != nil {
		return PetLevelStats{}, fmt.Errorf("npc: pet level stats: %w", err)
	}
	if s.MountMAtk, err = set.GetDoubleDefault("mAtkOnRide", 0); err != nil {
		return PetLevelStats{}, fmt.Errorf("npc: pet level stats: %w", err)
	}

	if set.Has("speedOnRide") {
		speeds, err := set.GetIntArray("speedOnRide")
		if err != nil {
			return PetLevelStats{}, fmt.Errorf("npc: pet level stats: %w", err)
		}
		if len(speeds) < 5 {
			return PetLevelStats{}, fmt.Errorf("npc: pet level stats: attribute %q: want at least 5 values, got %d", "speedOnRide", len(speeds))
		}
		s.MountBaseSpeed, s.MountWaterSpeed, s.MountFlySpeed = speeds[0], speeds[2], speeds[4]
	}

	return s, nil
}

// NewTemplate builds a Template from set, the merged <set> attributes of
// one <npc> element plus the "aiParams", "drops", "privates", "teachTo" and
// "pet" values the loader packed in.
func NewTemplate(set *commons.StatSet) (*Template, error) {
	id, err := set.GetInt("id")
	if err != nil {
		return nil, fmt.Errorf("npc template: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("npc template %d: %w", id, err) }

	t := &Template{ID: id}

	if t.TemplateID, err = set.GetIntDefault("idTemplate", id); err != nil {
		return nil, wrap(err)
	}
	if t.Name, err = set.GetString("name"); err != nil {
		return nil, wrap(err)
	}
	t.Title = set.GetStringDefault("title", "")
	t.Alias = set.GetStringDefault("alias", "")
	t.UsingServerSideName = set.GetBoolDefault("usingServerSideName", false)
	t.UsingServerSideTitle = set.GetBoolDefault("usingServerSideTitle", false)

	if t.Type, err = set.GetString("type"); err != nil {
		return nil, wrap(err)
	}
	if t.Level, err = set.GetIntDefault("level", 1); err != nil {
		return nil, wrap(err)
	}
	if t.HitTimeFactor, err = set.GetDoubleDefault("hitTimeFactor", 0); err != nil {
		return nil, wrap(err)
	}
	if t.RightHand, err = set.GetIntDefault("rHand", 0); err != nil {
		return nil, wrap(err)
	}
	if t.LeftHand, err = set.GetIntDefault("lHand", 0); err != nil {
		return nil, wrap(err)
	}
	if t.RewardExp, err = set.GetDoubleDefault("exp", 0); err != nil {
		return nil, wrap(err)
	}
	if t.RewardSp, err = set.GetDoubleDefault("sp", 0); err != nil {
		return nil, wrap(err)
	}

	if t.BaseAttackRange, err = set.GetIntDefault("baseAttackRange", 0); err != nil {
		return nil, wrap(err)
	}
	if t.BaseDamageRange, err = set.GetIntArray("baseDamageRange"); err != nil {
		return nil, wrap(err)
	}
	if t.BaseRandomDamage, err = set.GetIntDefault("baseRandomDamage", 0); err != nil {
		return nil, wrap(err)
	}

	if race, ok := commons.GetObject[Race](set, "race"); ok {
		t.Race = race
	}

	if t.STR, err = set.GetIntDefault("str", 40); err != nil {
		return nil, wrap(err)
	}
	if t.CON, err = set.GetIntDefault("con", 21); err != nil {
		return nil, wrap(err)
	}
	if t.DEX, err = set.GetIntDefault("dex", 30); err != nil {
		return nil, wrap(err)
	}
	if t.INT, err = set.GetIntDefault("int", 20); err != nil {
		return nil, wrap(err)
	}
	if t.WIT, err = set.GetIntDefault("wit", 43); err != nil {
		return nil, wrap(err)
	}
	if t.MEN, err = set.GetIntDefault("men", 20); err != nil {
		return nil, wrap(err)
	}

	if t.HPMax, err = set.GetDoubleDefault("hp", 0); err != nil {
		return nil, wrap(err)
	}
	if t.MPMax, err = set.GetDoubleDefault("mp", 0); err != nil {
		return nil, wrap(err)
	}
	if t.HPRegen, err = set.GetDoubleDefault("hpRegen", 1.5); err != nil {
		return nil, wrap(err)
	}
	if t.MPRegen, err = set.GetDoubleDefault("mpRegen", 0.9); err != nil {
		return nil, wrap(err)
	}
	if t.PAtk, err = set.GetDouble("pAtk"); err != nil {
		return nil, wrap(err)
	}
	if t.MAtk, err = set.GetDouble("mAtk"); err != nil {
		return nil, wrap(err)
	}
	if t.PDef, err = set.GetDouble("pDef"); err != nil {
		return nil, wrap(err)
	}
	if t.MDef, err = set.GetDouble("mDef"); err != nil {
		return nil, wrap(err)
	}
	if t.AtkSpd, err = set.GetDoubleDefault("atkSpd", 300); err != nil {
		return nil, wrap(err)
	}
	if t.CritRate, err = set.GetDoubleDefault("crit", 4); err != nil {
		return nil, wrap(err)
	}
	if t.WalkSpeed, err = set.GetDoubleDefault("walkSpd", 0); err != nil {
		return nil, wrap(err)
	}
	if t.RunSpeed, err = set.GetDoubleDefault("runSpd", 1); err != nil {
		return nil, wrap(err)
	}
	if t.CollisionRadius, err = set.GetDouble("radius"); err != nil {
		return nil, wrap(err)
	}
	if t.CollisionHeight, err = set.GetDouble("height"); err != nil {
		return nil, wrap(err)
	}

	if t.SSCount, err = set.GetIntDefault("ssCount", 0); err != nil {
		return nil, wrap(err)
	}
	if t.SPSCount, err = set.GetIntDefault("spsCount", 0); err != nil {
		return nil, wrap(err)
	}

	t.Undying = set.GetBoolDefault("undying", false)
	t.CanBeAttacked = set.GetBoolDefault("canBeAttacked", true)
	if t.CorpseTime, err = set.GetIntDefault("corpseTime", 7); err != nil {
		return nil, wrap(err)
	}
	t.NoSleepMode = set.GetBoolDefault("noSleepMode", false)
	if t.AggroRange, err = set.GetIntDefault("aggroRange", 0); err != nil {
		return nil, wrap(err)
	}
	t.CanMove = set.GetBoolDefault("canMove", true)
	t.Seedable = set.GetBoolDefault("seedable", false)
	t.CanSeeThrough = set.GetBoolDefault("canSeeThrough", false)

	if aiParams, ok := commons.GetObject[*commons.StatSet](set, "aiParams"); ok {
		t.AIParams = aiParams
	} else {
		t.AIParams = commons.NewStatSet()
	}

	if t.Drops, err = commons.GetList[item.DropCategory](set, "drops"); err != nil {
		return nil, wrap(err)
	}
	if t.Privates, err = commons.GetList[PrivateEntry](set, "privates"); err != nil {
		return nil, wrap(err)
	}

	if set.Has("clan") {
		if t.Clans, err = set.GetStringArray("clan"); err != nil {
			return nil, wrap(err)
		}
		if t.ClanRange, err = set.GetInt("clanRange"); err != nil {
			return nil, wrap(err)
		}
		if set.Has("ignoredIds") {
			if t.IgnoredIDs, err = set.GetIntArray("ignoredIds"); err != nil {
				return nil, wrap(err)
			}
		}
	}

	if set.Has("teachTo") {
		if t.TeachTo, err = set.GetIntArray("teachTo"); err != nil {
			return nil, wrap(err)
		}
	}

	if pet, ok := commons.GetObject[*PetData](set, "pet"); ok {
		t.Pet = pet
	}

	return t, nil
}

// Table is an in-memory lookup of NPC templates keyed by id, built once at
// boot and read for the remainder of the process lifetime. The zero value
// is not usable; construct with NewTable.
type Table struct {
	templates map[int]*Template
}

// NewTable returns a Table backed by templates, keyed by each template's
// ID. A later entry silently overwrites an earlier one with the same ID.
func NewTable(templates []*Template) *Table {
	t := &Table{templates: make(map[int]*Template, len(templates))}
	for _, tpl := range templates {
		t.templates[tpl.ID] = tpl
	}
	return t
}

// Get returns the template with the given id, or false if none was loaded.
func (t *Table) Get(id int) (*Template, bool) {
	tpl, ok := t.templates[id]
	return tpl, ok
}

// GetByName returns the first template whose name matches name
// case-insensitively, or false if none does.
func (t *Table) GetByName(name string) (*Template, bool) {
	for _, tpl := range t.templates {
		if strings.EqualFold(tpl.Name, name) {
			return tpl, true
		}
	}
	return nil, false
}

// Len returns the number of templates in the table.
func (t *Table) Len() int {
	return len(t.templates)
}
