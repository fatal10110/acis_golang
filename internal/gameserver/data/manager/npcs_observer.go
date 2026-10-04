package manager

import (
	"math/rand/v2"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/observer"
)

// SpawnObservers places the broadcasting towers of t, each a standalone
// spawn facing a random way, and gives each the viewpoint groups it
// offers. A tower naming an unknown NPC template is skipped with a
// warning before anything is built, as is one that cannot be placed. It
// reports how many towers were placed.
func (n *Npcs) SpawnObservers(t *observer.Table) int {
	placed := 0
	for _, s := range t.Spawns() {
		tmpl, ok := n.templates.Get(s.NPCID)
		if !ok {
			n.log.Warn().Int("npc_id", s.NPCID).Msg("observer spawn references unknown npc template")
			continue
		}
		id, err := n.spawnFixed(tmpl, s.Location.X, s.Location.Y, s.Location.Z, rand.IntN(65536))
		if err != nil {
			n.log.Warn().Err(err).Int("npc_id", s.NPCID).Msg("observer spawn: cannot place npc")
			continue
		}
		placed++
		obj, ok := n.state.Object(id)
		if !ok {
			continue
		}
		if f, ok := obj.(*npc.Folk); ok {
			f.SetObserverGroups(s.Groups)
		}
	}
	return placed
}
