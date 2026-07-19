package skill

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

const (
	storageSyncSkillLo int32 = 1368
	storageSyncSkillHi int32 = 1372
)

// LearnOutcome describes the result of applying a skill-learning request.
type LearnOutcome uint8

const (
	// LearnDone means the requested skill was learned.
	LearnDone LearnOutcome = iota
	// LearnUnavailable means the skill is not currently learnable.
	LearnUnavailable
	// LearnNeedsSP means the skill is learnable but the character lacks SP.
	LearnNeedsSP
	// LearnMissingItem means learning requires an item the character lacks.
	LearnMissingItem
)

// GeneralOffer is a general trainer skill the character can inspect.
type GeneralOffer struct {
	Grant  player.SkillGrant
	BookID int32
}

// FishingOffer is a fishing trainer skill the character can inspect.
type FishingOffer struct {
	Node modelskill.FishingSkill
}

// LearnResult describes the visible state changed by a successful learn.
type LearnResult struct {
	SkillID     int
	Level       int
	Cost        int
	StorageSync bool
}

// ValidAcquireRequest reports whether a skill-learning packet names a
// positive skill id and level.
func ValidAcquireRequest(skillID, level int32) bool {
	return skillID > 0 && level > 0
}

// LearnableGeneral returns the general skill grant for the requested skill
// when it is the next learnable level for the character.
func LearnableGeneral(tmpl *player.Template, playerLevel int, known player.SkillLevels, skillID, level int) (player.SkillGrant, bool) {
	if tmpl == nil {
		return player.SkillGrant{}, false
	}
	grant, ok := tmpl.FindSkillGrant(skillID, level)
	if !ok || grant.MinLevel > playerLevel || grant.Cost == 0 || known.Level(skillID) != level-1 {
		return player.SkillGrant{}, false
	}
	return grant, true
}

// GeneralOfferFor returns the general trainer offer for a loaded skill.
func GeneralOfferFor(c *player.Character, tmpl *player.Template, skills *Persistence, books modelskill.BookPolicy, skillID, level int) (GeneralOffer, bool) {
	if c == nil {
		return GeneralOffer{}, false
	}
	grant, ok := LearnableGeneral(tmpl, c.CharLevel, c.SkillLevels(), skillID, level)
	if !ok || !definitionLoaded(skills, skillID, level) {
		return GeneralOffer{}, false
	}
	return GeneralOffer{
		Grant:  grant,
		BookID: books.BookForSkill(modelskill.ID(skillID), level),
	}, true
}

// LearnableFishing returns the fishing-skill node for the requested skill
// when it is the next learnable level for the character.
func LearnableFishing(trees *modelskill.Trees, playerLevel int, hasDwarvenCraft bool, known player.SkillLevels, hasDefinition func(skillID, level int) bool, skillID, level int) (modelskill.FishingSkill, bool) {
	if trees == nil || skillID <= 0 || level <= 0 || hasDefinition == nil {
		return modelskill.FishingSkill{}, false
	}
	if known.Level(skillID) != level-1 {
		return modelskill.FishingSkill{}, false
	}
	node, ok := trees.FishingSkillFor(playerLevel, hasDwarvenCraft, TreeSkillLevels(known), modelskill.ID(skillID), level)
	if !ok || !hasDefinition(skillID, level) {
		return modelskill.FishingSkill{}, false
	}
	return node, true
}

// FishingOfferFor returns the fishing trainer offer for a loaded skill.
func FishingOfferFor(c *player.Character, trees *modelskill.Trees, skills *Persistence, skillID, level int) (FishingOffer, bool) {
	if c == nil {
		return FishingOffer{}, false
	}
	node, ok := LearnableFishing(trees, c.CharLevel, c.HasDwarvenCraft(), c.SkillLevels(), func(skillID, level int) bool {
		return definitionLoaded(skills, skillID, level)
	}, skillID, level)
	if !ok {
		return FishingOffer{}, false
	}
	return FishingOffer{Node: node}, true
}

// LearnGeneral applies a general trainer skill-learning request.
func LearnGeneral(ctx context.Context, c *player.Character, tmpl *player.Template, skills *Persistence, books modelskill.BookPolicy, skillID, level int) (LearnResult, LearnOutcome, error) {
	if c == nil {
		return LearnResult{}, LearnUnavailable, nil
	}
	grant, status := tmpl.CheckSkillLearn(c.CharLevel, c.SP, c.SkillLevels(), skillID, level)
	result := LearnResult{SkillID: grant.SkillID, Level: grant.Level, Cost: grant.CorrectedCost()}
	switch status {
	case player.LearnAllowed:
	case player.LearnNeedsSP:
		return result, LearnNeedsSP, nil
	default:
		return LearnResult{}, LearnUnavailable, nil
	}
	if !definitionLoaded(skills, skillID, level) {
		return LearnResult{}, LearnUnavailable, nil
	}
	if bookID := books.BookForSkill(modelskill.ID(skillID), level); bookID > 0 {
		if c.Inventory() == nil || c.Inventory().DestroyByTemplateID(bookID, 1) == nil {
			return result, LearnMissingItem, nil
		}
	}
	if err := setKnownSkill(ctx, skills, c, grant.SkillID, grant.Level); err != nil {
		return result, LearnDone, err
	}
	if result.Cost > 0 {
		c.RemoveSp(result.Cost)
	}
	return result, LearnDone, nil
}

// LearnFishing applies a fishing trainer skill-learning request.
func LearnFishing(ctx context.Context, c *player.Character, trees *modelskill.Trees, skills *Persistence, skillID, level int) (LearnResult, LearnOutcome, error) {
	offer, ok := FishingOfferFor(c, trees, skills, skillID, level)
	if !ok {
		return LearnResult{}, LearnUnavailable, nil
	}
	node := offer.Node
	result := LearnResult{
		SkillID:     int(node.ID),
		Level:       node.Level,
		StorageSync: NeedsStorageSync(int32(node.ID)),
	}
	if c.Inventory() == nil || c.Inventory().DestroyByTemplateID(node.ItemID, node.ItemCount) == nil {
		return result, LearnMissingItem, nil
	}
	if err := setKnownSkill(ctx, skills, c, skillID, level); err != nil {
		return result, LearnDone, err
	}
	return result, LearnDone, nil
}

// TreeSkillLevels converts player skill levels to the skill-tree model key
// type.
func TreeSkillLevels(src player.SkillLevels) modelskill.SkillLevels {
	out := make(modelskill.SkillLevels, len(src))
	for id, lvl := range src {
		out[modelskill.ID(id)] = lvl
	}
	return out
}

// NeedsStorageSync reports whether learning skillID changes client-visible
// storage limits.
func NeedsStorageSync(skillID int32) bool {
	return skillID >= storageSyncSkillLo && skillID <= storageSyncSkillHi
}

func definitionLoaded(skills *Persistence, skillID, level int) bool {
	return skills != nil && skills.HasDefinition(modelskill.Ref{ID: modelskill.ID(skillID), Level: level})
}

func setKnownSkill(ctx context.Context, skills *Persistence, c *player.Character, skillID, level int) error {
	if skills != nil {
		return skills.SetKnownSkill(ctx, c, skillID, level)
	}
	c.SetSkillLevel(skillID, level)
	return nil
}
