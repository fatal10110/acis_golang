package skill

import "time"

// NPCPlacer places one NPC of template npcID at (x, y) on the ground below
// z, facing heading, as a spawn that never respawns, and deletes it once
// despawn has passed when despawn is positive.
type NPCPlacer func(npcID int32, x, y, z, heading int, despawn time.Duration)

// NewSpawn returns the SPAWN handler, which places each cast's NPC through
// place.
func NewSpawn(place NPCPlacer) Handler { return spawnHandler{place: place} }

type spawnHandler struct{ place NPCPlacer }

func (spawnHandler) Types() []string { return []string{"SPAWN"} }

// Use places the skill's NPC where a living caster stands, facing the way
// it faces, for the skill's despawn delay.
func (h spawnHandler) Use(cast Cast) {
	if h.place == nil || cast.Caster == nil || alikeDead(cast.Caster) {
		return
	}
	x, y, z := cast.Caster.Position()
	h.place(int32(cast.Skill.NpcID), x, y, z, cast.Caster.Heading(), time.Duration(cast.Skill.DespawnDelay)*time.Millisecond)
}
