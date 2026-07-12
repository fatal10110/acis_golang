package skill

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// FishingSkill is one entry in the fishing skill tree: the level of a
// fishing-related skill a character can learn once they reach MinLevel, and
// the item and quantity spent to learn it. Deciding whether a specific
// character currently qualifies (character level, already-known skill
// level, dwarven-craft ownership) is that character system's job, not this
// loader's.
type FishingSkill struct {
	ID        ID
	Level     int
	MinLevel  int
	ItemID    int32
	ItemCount int
	Dwarven   bool
}

// NewFishingSkill builds a FishingSkill from set, the folded attributes of
// one <fishingSkill> element. id, lvl, minLvl, itemId and itemCount are all
// required; isDwarven defaults to false.
func NewFishingSkill(set *commons.StatSet) (FishingSkill, error) {
	idf := commons.NewFields(set, "skill: fishing skill")
	id := idf.Int32("id")
	if err := idf.Err(); err != nil {
		return FishingSkill{}, err
	}

	f := commons.NewFields(set, fmt.Sprintf("skill: fishing skill %d", id))
	entry := FishingSkill{
		ID:        ID(id),
		Level:     f.Int("lvl"),
		MinLevel:  f.Int("minLvl"),
		ItemID:    f.Int32("itemId"),
		ItemCount: f.Int("itemCount"),
		Dwarven:   f.BoolDefault("isDwarven", false),
	}
	if err := f.Err(); err != nil {
		return FishingSkill{}, err
	}
	return entry, nil
}

// ClanSkill is one entry in the clan skill tree: the level of a clan skill a
// clan can learn once it reaches MinLevel, and the reputation cost and item
// spent to learn it. Deciding whether a specific clan currently qualifies is
// that clan system's job, not this loader's.
type ClanSkill struct {
	ID       ID
	Level    int
	MinLevel int
	Cost     int
	ItemID   int32
}

// NewClanSkill builds a ClanSkill from set, the folded attributes of one
// <clanSkill> element. Every attribute is required.
func NewClanSkill(set *commons.StatSet) (ClanSkill, error) {
	idf := commons.NewFields(set, "skill: clan skill")
	id := idf.Int32("id")
	if err := idf.Err(); err != nil {
		return ClanSkill{}, err
	}

	f := commons.NewFields(set, fmt.Sprintf("skill: clan skill %d", id))
	entry := ClanSkill{
		ID:       ID(id),
		Level:    f.Int("lvl"),
		MinLevel: f.Int("minLvl"),
		Cost:     f.Int("cost"),
		ItemID:   f.Int32("itemId"),
	}
	if err := f.Err(); err != nil {
		return ClanSkill{}, err
	}
	return entry, nil
}

// SkillLevels maps a known skill id to its current level.
type SkillLevels map[ID]int

// Level returns the known level for id, or 0 when it is not known.
func (l SkillLevels) Level(id ID) int {
	return l[id]
}

// LearnStatus describes the result of checking a tree-learning request.
type LearnStatus uint8

const (
	// LearnAllowed means the requested skill can be learned now.
	LearnAllowed LearnStatus = iota
	// LearnUnavailable means the requested skill is not the next learnable
	// level for the current tree state.
	LearnUnavailable
	// LearnNeedsCost means the skill is otherwise learnable but the caller
	// does not have enough points to pay its cost.
	LearnNeedsCost
)

// EnchantSkill is one entry in the enchant skill tree: the exp/sp cost and
// per-caster-magic-level success rates for reaching one enchant level (101+
// or 141+) of a skill, and the item optionally consumed to attempt it (zero
// ItemID means none). Deciding whether a specific character currently
// qualifies, and rolling the actual success rate, is that enchant system's
// job, not this loader's.
type EnchantSkill struct {
	ID    ID
	Level int
	Exp   int
	SP    int

	// Rate76 through Rate80 are the percent chance of success when the
	// casting character's magic level is 76 through 80 respectively.
	Rate76, Rate77, Rate78, Rate79, Rate80 int

	ItemID    int32
	ItemCount int
}

// NewEnchantSkill builds an EnchantSkill from set, the folded attributes of
// one <enchantSkill> element. id, lvl, exp, sp and the five rate attributes
// are all required; itemNeeded ("itemId-count") is optional.
func NewEnchantSkill(set *commons.StatSet) (EnchantSkill, error) {
	idf := commons.NewFields(set, "skill: enchant skill")
	id := idf.Int32("id")
	if err := idf.Err(); err != nil {
		return EnchantSkill{}, err
	}

	f := commons.NewFields(set, fmt.Sprintf("skill: enchant skill %d", id))
	level := f.Int("lvl")
	exp := f.Int("exp")
	sp := f.Int("sp")

	rates := make([]int, 5)
	for i, key := range [...]string{"rate76", "rate77", "rate78", "rate79", "rate80"} {
		rates[i] = f.Int(key)
	}

	e := EnchantSkill{
		ID: ID(id), Level: level, Exp: exp, SP: sp,
		Rate76: rates[0], Rate77: rates[1], Rate78: rates[2], Rate79: rates[3], Rate80: rates[4],
	}

	if f.Has("itemNeeded") {
		raw := f.String("itemNeeded")
		if itemID, count, err := parseItemNeeded(raw); err != nil {
			f.Fail(fmt.Errorf("itemNeeded %q: %w", raw, err))
		} else {
			e.ItemID, e.ItemCount = itemID, count
		}
	}

	if err := f.Err(); err != nil {
		return EnchantSkill{}, err
	}
	return e, nil
}

// parseItemNeeded parses an "itemNeeded" attribute's "itemId-count" form.
func parseItemNeeded(raw string) (itemID int32, count int, err error) {
	return parseDashPair(raw)
}

// Trees holds every skill tree a character or clan learns from: the three
// standalone trees this system's data ships (fishing, clan, and skill
// enchantment). It is built once at boot and read for the remainder of the
// process lifetime.
type Trees struct {
	Fishing []FishingSkill
	Clan    []ClanSkill
	Enchant []EnchantSkill
}

// FishingSkillsFor returns fishing skill nodes learnable by the player now.
func (t *Trees) FishingSkillsFor(playerLevel int, hasDwarvenCraft bool, known SkillLevels) []FishingSkill {
	if t == nil {
		return nil
	}
	var skills []FishingSkill
	for _, skill := range t.Fishing {
		if skill.MinLevel <= playerLevel && fishingAllowed(skill, hasDwarvenCraft) && known.Level(skill.ID) == skill.Level-1 {
			skills = append(skills, skill)
		}
	}
	return skills
}

// FishingSkillFor returns the requested fishing skill when it is learnable.
func (t *Trees) FishingSkillFor(playerLevel int, hasDwarvenCraft bool, known SkillLevels, skillID ID, level int) (FishingSkill, bool) {
	if t == nil || skillID <= 0 || level <= 0 {
		return FishingSkill{}, false
	}
	for _, skill := range t.Fishing {
		if skill.ID != skillID || skill.Level != level || !fishingAllowed(skill, hasDwarvenCraft) {
			continue
		}
		if skill.MinLevel <= playerLevel && known.Level(skillID) == level-1 {
			return skill, true
		}
		return FishingSkill{}, false
	}
	return FishingSkill{}, false
}

// RequiredLevelForNextFishingSkill returns the lowest future player level
// with an eligible fishing skill, or 0 when there is none.
func (t *Trees) RequiredLevelForNextFishingSkill(playerLevel int, hasDwarvenCraft bool) int {
	if t == nil {
		return 0
	}
	next := 0
	for _, skill := range t.Fishing {
		if skill.MinLevel <= playerLevel || !fishingAllowed(skill, hasDwarvenCraft) {
			continue
		}
		if next == 0 || skill.MinLevel < next {
			next = skill.MinLevel
		}
	}
	return next
}

func fishingAllowed(skill FishingSkill, hasDwarvenCraft bool) bool {
	return !skill.Dwarven || hasDwarvenCraft
}

// ClanSkillsFor returns clan skill nodes learnable by the clan now.
func (t *Trees) ClanSkillsFor(clanLevel int, known SkillLevels) []ClanSkill {
	if t == nil {
		return nil
	}
	var skills []ClanSkill
	for _, skill := range t.Clan {
		if skill.MinLevel <= clanLevel && known.Level(skill.ID) == skill.Level-1 {
			skills = append(skills, skill)
		}
	}
	return skills
}

// ClanSkillFor returns the requested clan skill when it is learnable.
func (t *Trees) ClanSkillFor(clanLevel int, known SkillLevels, skillID ID, level int) (ClanSkill, bool) {
	if t == nil || skillID <= 0 || level <= 0 {
		return ClanSkill{}, false
	}
	for _, skill := range t.Clan {
		if skill.ID != skillID || skill.Level != level {
			continue
		}
		if skill.MinLevel <= clanLevel && known.Level(skillID) == level-1 {
			return skill, true
		}
		return ClanSkill{}, false
	}
	return ClanSkill{}, false
}

// CheckClanSkillLearn checks whether a clan skill can be learned now and
// whether reputation covers its cost.
func (t *Trees) CheckClanSkillLearn(clanLevel, reputation int, known SkillLevels, skillID ID, level int) (ClanSkill, LearnStatus) {
	skill, ok := t.ClanSkillFor(clanLevel, known, skillID, level)
	if !ok {
		return ClanSkill{}, LearnUnavailable
	}
	if reputation < skill.Cost {
		return skill, LearnNeedsCost
	}
	return skill, LearnAllowed
}
