package manager

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// SpawnMercenary places one NPC of template npcID, hired for the castle
// castleID, as a standalone spawn that never respawns: at (at.X, at.Y) on
// the ground height below at.Z, facing heading 0. The NPC belongs to the
// castle from the moment it spawns. It returns the placed NPC's object id.
func (n *Npcs) SpawnMercenary(npcID int, at location.Location, castleID int) (int32, error) {
	tmpl, ok := n.templates.Get(npcID)
	if !ok {
		return 0, fmt.Errorf("%w: %d", npc.ErrNoTemplate, npcID)
	}
	live, err := n.spawnStandalone(tmpl, at.X, at.Y, at.Z, 0, 0, nil, castleID, 0)
	if err != nil {
		return 0, err
	}
	return live.ObjectID(), nil
}

// KillFixed kills the live NPC id placed by SpawnFixed or SpawnMercenary,
// hostile or civilian, as if it struck itself down, on its own queue. It
// reports false, killing nothing, for any other NPC.
func (n *Npcs) KillFixed(id int32) bool {
	n.mu.Lock()
	key, live := n.live[id]
	slot := n.slot[key]
	n.mu.Unlock()
	if !live || !slot.fixed {
		return false
	}
	obj, ok := n.state.Object(id)
	if !ok {
		return false
	}
	switch a := obj.(type) {
	case *npc.Hostile:
		onQueue(a.Queue(), func() { a.Kill(a) })
	case *npc.Folk:
		onQueue(a.Queue(), func() { a.Kill(a) })
	default:
		return false
	}
	return true
}
