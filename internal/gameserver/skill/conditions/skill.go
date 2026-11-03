package conditions

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"

// SkillStats requires the skill under test to name the given Stat. A test
// with no skill (an item use condition, a stat func gate) fails.
type SkillStats struct{ Stat stat.Stat }

func (c SkillStats) Test(effector, effected Actor, skill Skill) bool {
	if skill == nil {
		return false
	}
	s, ok := skill.Stat()
	return ok && s == c.Stat
}

// UsingItemType requires the effector to be a player currently wearing an
// item whose type mask intersects Mask.
type UsingItemType struct{ Mask int }

func (c UsingItemType) Test(effector, effected Actor, skill Skill) bool {
	p, ok := asPlayer(effector)
	return ok && p.IsWearingType(c.Mask)
}
