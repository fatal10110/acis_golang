package conditions

import "slices"

// TargetActiveSkillID requires the effected creature to know a skill of the
// given id, at any level. A nil effected always fails.
type TargetActiveSkillID struct{ SkillID int }

func (c TargetActiveSkillID) Test(effector, effected Actor, skill Skill) bool {
	if effected == nil {
		return false
	}
	_, ok := effected.ActiveSkillLevel(c.SkillID)
	return ok
}

// TargetHpMinMax requires the effected creature's current HP percentage
// (0-100) to fall within [Min, Max]. A nil effected always fails.
type TargetHpMinMax struct{ Min, Max int }

func (c TargetHpMinMax) Test(effector, effected Actor, skill Skill) bool {
	if effected == nil {
		return false
	}
	hp := effected.HPRatio() * 100
	return hp >= float64(c.Min) && hp <= float64(c.Max)
}

// TargetNpcID requires the effected target to be an NPC or door whose id
// is in the given list.
type TargetNpcID struct{ IDs []int }

func (c TargetNpcID) Test(effector, effected Actor, skill Skill) bool {
	if effected == nil {
		return false
	}
	id, ok := effected.TargetID()
	return ok && slices.Contains(c.IDs, id)
}

// TargetRaceID requires the effected target to be an NPC whose template
// race ordinal is in the given list.
type TargetRaceID struct{ IDs []int }

func (c TargetRaceID) Test(effector, effected Actor, skill Skill) bool {
	if effected == nil {
		return false
	}
	race, ok := effected.RaceOrdinal()
	return ok && slices.Contains(c.IDs, race)
}
