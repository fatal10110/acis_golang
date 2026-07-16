package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

const (
	storageSyncSkillLo int32 = 1368
	storageSyncSkillHi int32 = 1372
)

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
