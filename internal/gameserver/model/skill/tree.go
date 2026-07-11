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
