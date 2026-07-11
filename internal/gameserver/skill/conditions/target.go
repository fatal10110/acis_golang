package conditions

// npcTarget and doorTarget are the two identifiable-by-id target shapes
// TargetNpcID checks against — an NPC (keyed by its template id) or a door
// (keyed by its static door id). door.Object already exposes DoorID();
// giving a world NPC instance a matching NpcID accessor is the world/NPC
// package's call to make once it wires targets through this engine.
type npcTarget interface{ NpcID() int32 }
type doorTarget interface{ DoorID() int }

// raceTarget is an NPC target's template race ordinal, as
// TargetRaceID needs it.
type raceTarget interface{ RaceOrdinal() int }

// TargetActiveSkillID requires the effected creature to currently know a
// skill of the given id, at any level.
type TargetActiveSkillID struct{ SkillID int }

func (c TargetActiveSkillID) Test(effector, effected, skill any) bool {
	_, ok := effected.(Actor).ActiveSkillLevel(c.SkillID)
	return ok
}

// TargetHpMinMax requires the effected creature's current HP percentage
// (0-100) to fall within [Min, Max]. A nil effected always fails.
type TargetHpMinMax struct{ Min, Max int }

func (c TargetHpMinMax) Test(effector, effected, skill any) bool {
	if effected == nil {
		return false
	}
	hp := effected.(Actor).HPRatio() * 100
	return hp >= float64(c.Min) && hp <= float64(c.Max)
}

// TargetNpcID requires the effected target to be an NPC or door whose id
// is in the given list.
type TargetNpcID struct{ IDs []int }

func (c TargetNpcID) Test(effector, effected, skill any) bool {
	contains := func(id int) bool {
		for _, want := range c.IDs {
			if want == id {
				return true
			}
		}
		return false
	}
	if npc, ok := effected.(npcTarget); ok {
		return contains(int(npc.NpcID()))
	}
	if door, ok := effected.(doorTarget); ok {
		return contains(door.DoorID())
	}
	return false
}

// TargetRaceID requires the effected target to be an NPC whose template
// race ordinal is in the given list.
type TargetRaceID struct{ IDs []int }

func (c TargetRaceID) Test(effector, effected, skill any) bool {
	npc, ok := effected.(raceTarget)
	if !ok {
		return false
	}
	race := npc.RaceOrdinal()
	for _, want := range c.IDs {
		if want == race {
			return true
		}
	}
	return false
}
