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
	id, err := set.GetInt32("id")
	if err != nil {
		return FishingSkill{}, fmt.Errorf("skill: fishing skill: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("skill: fishing skill %d: %w", id, err) }

	level, err := set.GetInt("lvl")
	if err != nil {
		return FishingSkill{}, wrap(err)
	}
	minLevel, err := set.GetInt("minLvl")
	if err != nil {
		return FishingSkill{}, wrap(err)
	}
	itemID, err := set.GetInt32("itemId")
	if err != nil {
		return FishingSkill{}, wrap(err)
	}
	itemCount, err := set.GetInt("itemCount")
	if err != nil {
		return FishingSkill{}, wrap(err)
	}
	dwarven := set.GetBoolDefault("isDwarven", false)

	return FishingSkill{
		ID: ID(id), Level: level, MinLevel: minLevel,
		ItemID: itemID, ItemCount: itemCount, Dwarven: dwarven,
	}, nil
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
	id, err := set.GetInt32("id")
	if err != nil {
		return ClanSkill{}, fmt.Errorf("skill: clan skill: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("skill: clan skill %d: %w", id, err) }

	level, err := set.GetInt("lvl")
	if err != nil {
		return ClanSkill{}, wrap(err)
	}
	minLevel, err := set.GetInt("minLvl")
	if err != nil {
		return ClanSkill{}, wrap(err)
	}
	cost, err := set.GetInt("cost")
	if err != nil {
		return ClanSkill{}, wrap(err)
	}
	itemID, err := set.GetInt32("itemId")
	if err != nil {
		return ClanSkill{}, wrap(err)
	}

	return ClanSkill{ID: ID(id), Level: level, MinLevel: minLevel, Cost: cost, ItemID: itemID}, nil
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
	id, err := set.GetInt32("id")
	if err != nil {
		return EnchantSkill{}, fmt.Errorf("skill: enchant skill: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("skill: enchant skill %d: %w", id, err) }

	level, err := set.GetInt("lvl")
	if err != nil {
		return EnchantSkill{}, wrap(err)
	}
	exp, err := set.GetInt("exp")
	if err != nil {
		return EnchantSkill{}, wrap(err)
	}
	sp, err := set.GetInt("sp")
	if err != nil {
		return EnchantSkill{}, wrap(err)
	}

	rates := make([]int, 5)
	for i, key := range [...]string{"rate76", "rate77", "rate78", "rate79", "rate80"} {
		rates[i], err = set.GetInt(key)
		if err != nil {
			return EnchantSkill{}, wrap(err)
		}
	}

	e := EnchantSkill{
		ID: ID(id), Level: level, Exp: exp, SP: sp,
		Rate76: rates[0], Rate77: rates[1], Rate78: rates[2], Rate79: rates[3], Rate80: rates[4],
	}

	if set.Has("itemNeeded") {
		raw, err := set.GetString("itemNeeded")
		if err != nil {
			return EnchantSkill{}, wrap(err)
		}
		itemID, count, err := parseItemNeeded(raw)
		if err != nil {
			return EnchantSkill{}, wrap(fmt.Errorf("itemNeeded %q: %w", raw, err))
		}
		e.ItemID, e.ItemCount = itemID, count
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
