package skill

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// EnchantMinClassLevel and EnchantMinCharLevel are the two gates a
// character must clear before any skill-enchant packet is answered: a
// third-class (or awakened) profession, and character level 76, the
// lowest level the enchant tree's success-rate columns cover.
const (
	EnchantMinClassLevel = 3
	EnchantMinCharLevel  = 76
)

// EnchantOutcome describes the result of applying a skill-enchant request.
type EnchantOutcome uint8

const (
	// EnchantUnavailable means the requested enchant level is not currently
	// offered to the character.
	EnchantUnavailable EnchantOutcome = iota
	// EnchantNeedsSP means the enchant is offered but the character lacks SP.
	EnchantNeedsSP
	// EnchantNeedsExp means the enchant is offered but the character lacks
	// the experience to spend while staying at or above level 76.
	EnchantNeedsExp
	// EnchantMissingItem means the enchant requires an item the character
	// lacks.
	EnchantMissingItem
	// EnchantSucceeded means the roll succeeded and the skill was enchanted
	// to the requested level.
	EnchantSucceeded
	// EnchantFailed means the roll failed; the skill was reset to its
	// current max normal (non-enchanted) level.
	EnchantFailed
)

// EnchantOffer is one enchant-skill trainer offer: the tree entry plus the
// success rate for the character's current level.
type EnchantOffer struct {
	Skill modelskill.EnchantSkill
	Rate  int
}

// EnchantEligible reports whether a character of classID and charLevel may
// use the skill-enchant system at all.
func EnchantEligible(classID, charLevel int) bool {
	level, ok := player.ClassLevel(classID)
	return ok && level >= EnchantMinClassLevel && charLevel >= EnchantMinCharLevel
}

// EnchantOfferFor returns the enchant trainer offer for a loaded skill when
// it is the next enchantable level for the character.
func EnchantOfferFor(c *player.Character, trees *modelskill.Trees, skills *Persistence, skillID, level int) (EnchantOffer, bool) {
	if c == nil || trees == nil || skills == nil || skills.skills == nil {
		return EnchantOffer{}, false
	}
	if !EnchantEligible(c.ClassID, c.CharLevel) {
		return EnchantOffer{}, false
	}
	if c.SkillLevel(skillID) >= level {
		return EnchantOffer{}, false
	}
	if !definitionLoaded(skills, skillID, level) {
		return EnchantOffer{}, false
	}
	node, ok := trees.EnchantSkillFor(skills.skills, TreeSkillLevels(c.SkillLevels()), modelskill.ID(skillID), level)
	if !ok {
		return EnchantOffer{}, false
	}
	rate, ok := node.SuccessRateForLevel(c.CharLevel)
	if !ok {
		return EnchantOffer{}, false
	}
	return EnchantOffer{Skill: node, Rate: rate}, true
}

// EnchantResult describes the visible state changed by an enchant attempt.
type EnchantResult struct {
	SkillID      int
	Level        int
	AppliedLevel int
	SP           int
	Exp          int
}

// Enchant applies a skill-enchant attempt against a loaded offer: SP and
// exp checks, optional item consumption (skipped entirely when
// spBookNeeded is false), exp/sp deduction, and a rate roll. roll must
// return a value in [0,99]; the attempt succeeds when roll <= the offer's
// rate, raising the skill to the requested level, and otherwise resets it
// to its current max normal (non-enchanted) level.
func Enchant(ctx context.Context, c *player.Character, table *player.LevelTable, tmpl *player.Template, trees *modelskill.Trees, skills *Persistence, spBookNeeded bool, roll func() int, skillID, level int) (EnchantResult, EnchantOutcome, error) {
	if c == nil {
		return EnchantResult{}, EnchantUnavailable, nil
	}
	offer, ok := EnchantOfferFor(c, trees, skills, skillID, level)
	if !ok {
		return EnchantResult{}, EnchantUnavailable, nil
	}
	node := offer.Skill
	result := EnchantResult{SkillID: skillID, Level: level, SP: node.SP, Exp: node.Exp}

	if c.SP < node.SP {
		return result, EnchantNeedsSP, nil
	}
	if c.Exp-int64(node.Exp) < table.RequiredExpForLevel(EnchantMinCharLevel) {
		return result, EnchantNeedsExp, nil
	}
	if spBookNeeded && node.ItemID != 0 {
		if c.Inventory() == nil || c.Inventory().DestroyByTemplateID(node.ItemID, node.ItemCount) == nil {
			return result, EnchantMissingItem, nil
		}
	}

	c.RemoveExpAndSp(table, tmpl, int64(node.Exp), node.SP)

	rolled := 0
	if roll != nil {
		rolled = roll()
	}
	if rolled <= offer.Rate {
		result.AppliedLevel = level
		if err := setKnownSkill(ctx, skills, c, skillID, level); err != nil {
			return result, EnchantSucceeded, err
		}
		return result, EnchantSucceeded, nil
	}

	maxLevel := skills.skills.MaxLevel(modelskill.ID(skillID))
	result.AppliedLevel = maxLevel
	if err := setKnownSkill(ctx, skills, c, skillID, maxLevel); err != nil {
		return result, EnchantFailed, err
	}
	return result, EnchantFailed, nil
}
