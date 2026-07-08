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
	f := commons.NewFields(set, "npc: private entry")
	id := f.Int("id")
	weight := f.Int("weight")
	respawnStr := f.String("respawn")
	if err := f.Err(); err != nil {
		return PrivateEntry{}, err
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
	f := commons.NewFields(set, "npc: pet data")
	p := &PetData{
		Food1:         f.Int("food1"),
		Food2:         f.Int("food2"),
		AutoFeedLimit: f.Double("autoFeedLimit"),
		HungryLimit:   f.Double("hungryLimit"),
		UnsummonLimit: f.Double("unsummonLimit"),
		Levels:        levels,
	}
	if err := f.Err(); err != nil {
		return nil, err
	}
	return p, nil
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
	f := commons.NewFields(set, "npc: pet level stats")
	s := PetLevelStats{
		MaxExp:       f.Long("exp"),
		MaxMeal:      f.Int("maxMeal"),
		ExpType:      f.Int("expType"),
		MealInBattle: f.Int("mealInBattle"),
		MealInNormal: f.Int("mealInNormal"),
		PAtk:         f.Double("pAtk"),
		PDef:         f.Double("pDef"),
		MAtk:         f.Double("mAtk"),
		MDef:         f.Double("mDef"),
		MaxHP:        f.Double("hp"),
		MaxMP:        f.Double("mp"),
		HPRegen:      f.Double("hpRegen"),
		MPRegen:      f.Double("mpRegen"),
		SSCount:      f.Int("ssCount"),
		SPSCount:     f.Int("spsCount"),

		MountMealInBattle: f.IntDefault("mealInBattleOnRide", 0),
		MountMealInNormal: f.IntDefault("mealInNormalOnRide", 0),
		MountAtkSpd:       f.DoubleDefault("atkSpdOnRide", 0),
		MountPAtk:         f.DoubleDefault("pAtkOnRide", 0),
		MountMAtk:         f.DoubleDefault("mAtkOnRide", 0),
	}

	if f.Has("speedOnRide") {
		speeds := f.IntArray("speedOnRide")
		if len(speeds) < 5 {
			f.Fail(fmt.Errorf("attribute %q: want at least 5 values, got %d", "speedOnRide", len(speeds)))
		} else {
			s.MountBaseSpeed, s.MountWaterSpeed, s.MountFlySpeed = speeds[0], speeds[2], speeds[4]
		}
	}

	if err := f.Err(); err != nil {
		return PetLevelStats{}, err
	}
	return s, nil
}

// NewTemplate builds a Template from set, the merged <set> attributes of
// one <npc> element plus the "aiParams", "drops", "privates", "teachTo" and
// "pet" values the loader packed in.
func NewTemplate(set *commons.StatSet) (*Template, error) {
	idf := commons.NewFields(set, "npc template")
	id := idf.Int("id")
	if err := idf.Err(); err != nil {
		return nil, err
	}

	f := commons.NewFields(set, fmt.Sprintf("npc template %d", id))
	t := &Template{
		ID:         id,
		TemplateID: f.IntDefault("idTemplate", id),
		Name:       f.String("name"),

		Title: f.StringDefault("title", ""),
		Alias: f.StringDefault("alias", ""),

		UsingServerSideName:  f.BoolDefault("usingServerSideName", false),
		UsingServerSideTitle: f.BoolDefault("usingServerSideTitle", false),

		Type:          f.String("type"),
		Level:         f.IntDefault("level", 1),
		HitTimeFactor: f.DoubleDefault("hitTimeFactor", 0),
		RightHand:     f.IntDefault("rHand", 0),
		LeftHand:      f.IntDefault("lHand", 0),
		RewardExp:     f.DoubleDefault("exp", 0),
		RewardSp:      f.DoubleDefault("sp", 0),

		BaseAttackRange:  f.IntDefault("baseAttackRange", 0),
		BaseDamageRange:  f.IntArray("baseDamageRange"),
		BaseRandomDamage: f.IntDefault("baseRandomDamage", 0),

		STR: f.IntDefault("str", 40),
		CON: f.IntDefault("con", 21),
		DEX: f.IntDefault("dex", 30),
		INT: f.IntDefault("int", 20),
		WIT: f.IntDefault("wit", 43),
		MEN: f.IntDefault("men", 20),

		HPMax:   f.DoubleDefault("hp", 0),
		MPMax:   f.DoubleDefault("mp", 0),
		HPRegen: f.DoubleDefault("hpRegen", 1.5),
		MPRegen: f.DoubleDefault("mpRegen", 0.9),
		PAtk:    f.Double("pAtk"),
		MAtk:    f.Double("mAtk"),
		PDef:    f.Double("pDef"),
		MDef:    f.Double("mDef"),

		AtkSpd:    f.DoubleDefault("atkSpd", 300),
		CritRate:  f.DoubleDefault("crit", 4),
		WalkSpeed: f.DoubleDefault("walkSpd", 0),
		RunSpeed:  f.DoubleDefault("runSpd", 1),

		CollisionRadius: f.Double("radius"),
		CollisionHeight: f.Double("height"),

		SSCount:  f.IntDefault("ssCount", 0),
		SPSCount: f.IntDefault("spsCount", 0),

		Undying:       f.BoolDefault("undying", false),
		CanBeAttacked: f.BoolDefault("canBeAttacked", true),
		CorpseTime:    f.IntDefault("corpseTime", 7),
		NoSleepMode:   f.BoolDefault("noSleepMode", false),
		AggroRange:    f.IntDefault("aggroRange", 0),
		CanMove:       f.BoolDefault("canMove", true),
		Seedable:      f.BoolDefault("seedable", false),
		CanSeeThrough: f.BoolDefault("canSeeThrough", false),

		Drops:    commons.FieldList[item.DropCategory](f, "drops"),
		Privates: commons.FieldList[PrivateEntry](f, "privates"),
	}

	if race, ok := commons.FieldObject[Race](f, "race"); ok {
		t.Race = race
	}

	if aiParams, ok := commons.FieldObject[*commons.StatSet](f, "aiParams"); ok {
		t.AIParams = aiParams
	} else {
		t.AIParams = commons.NewStatSet()
	}

	if f.Has("clan") {
		t.Clans = f.StringArray("clan")
		t.ClanRange = f.Int("clanRange")
		if f.Has("ignoredIds") {
			t.IgnoredIDs = f.IntArray("ignoredIds")
		}
	}

	if f.Has("teachTo") {
		t.TeachTo = f.IntArray("teachTo")
	}

	if pet, ok := commons.FieldObject[*PetData](f, "pet"); ok {
		t.Pet = pet
	}

	if err := f.Err(); err != nil {
		return nil, err
	}
	return t, nil
}

// Table is an in-memory lookup of NPC templates keyed by id, built once at
// boot and read for the remainder of the process lifetime. The zero value
// is not usable; construct with NewTable.
type Table struct {
	*commons.Lookup[int, *Template]
}

// NewTable returns a Table backed by templates, keyed by each template's
// ID. A later entry silently overwrites an earlier one with the same ID.
func NewTable(templates []*Template) *Table {
	return &Table{commons.NewLookup(templates, func(tpl *Template) int { return tpl.ID })}
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
