package manager

import (
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
)

// SpawnArtifact places the castle holy artifact a as a standalone spawn of
// its own, at its position on the ground height below it, facing its
// heading, and returns the placed NPC's object id. An artifact naming an
// unknown NPC template, or one that cannot be placed, is skipped with a
// warning and reports false.
func (n *Npcs) SpawnArtifact(a castledata.Artifact) (int32, bool) {
	tmpl, ok := n.templates.Get(a.NPCID)
	if !ok {
		n.log.Warn().Int("npc_id", a.NPCID).Msg("castle artifact references unknown npc template")
		return 0, false
	}
	id, err := n.spawnFixed(tmpl, a.Position.X, a.Position.Y, a.Position.Z, a.Heading)
	if err != nil {
		n.log.Warn().Err(err).Int("npc_id", a.NPCID).Msg("castle artifact: cannot place npc")
		return 0, false
	}
	return id, true
}
