package conditions

import "slices"

// npcTarget and doorTarget are the two identifiable-by-id target shapes
// TargetNpcID checks against — an NPC's condition view (keyed by its
// template id) or a door's (keyed by its static door id).
type (
	npcTarget  interface{ NpcID() int }
	doorTarget interface{ DoorID() int }
)

// raceTarget is an NPC target's template race ordinal, as
// TargetRaceID needs it.
type raceTarget interface{ RaceOrdinal() int }

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
	if npc, ok := effected.(npcTarget); ok {
		return slices.Contains(c.IDs, npc.NpcID())
	}
	if door, ok := effected.(doorTarget); ok {
		return slices.Contains(c.IDs, door.DoorID())
	}
	return false
}

// TargetRaceID requires the effected target to be an NPC whose template
// race ordinal is in the given list.
type TargetRaceID struct{ IDs []int }

func (c TargetRaceID) Test(effector, effected Actor, skill Skill) bool {
	npc, ok := effected.(raceTarget)
	if !ok {
		return false
	}
	return slices.Contains(c.IDs, npc.RaceOrdinal())
}
