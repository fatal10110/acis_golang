package conditions

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"

// SkillStats requires the skill under test to target the given Stat.
type SkillStats struct{ Stat stat.Stat }

func (c SkillStats) Test(effector, effected, skillArg any) bool {
	s, ok := skillArg.(Skill)
	return ok && s.Stat() == c.Stat
}

// UsingItemType requires the effector to be a player currently wearing an
// item whose type mask intersects Mask.
type UsingItemType struct{ Mask int }

func (c UsingItemType) Test(effector, effected, skill any) bool {
	p, ok := asPlayer(effector)
	return ok && p.IsWearingType(c.Mask)
}
