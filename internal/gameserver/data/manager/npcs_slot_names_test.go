package manager

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// mustSlot returns the id of the spawn slot named name (Npcs.SlotID),
// failing t when no slot has that name.
func mustSlot(t testing.TB, n *Npcs, name string) task.SlotID {
	t.Helper()
	id, ok := n.SlotID(name)
	if !ok {
		t.Fatalf("no spawn slot named %s", name)
	}
	return id
}

// respawnArmed reports whether r has a respawn armed for the spawn slot
// named name.
func respawnArmed(n *Npcs, r *task.Respawn, name string) bool {
	id, ok := n.SlotID(name)
	return ok && r.Tracked(id)
}

// cancelRespawn cancels the respawn r has armed for the spawn slot named
// name, reporting whether one was.
func cancelRespawn(n *Npcs, r *task.Respawn, name string) bool {
	id, ok := n.SlotID(name)
	return ok && r.Cancel(id)
}

// respawnNamed brings back the NPC of the spawn slot named name, as its
// respawn coming due does; nothing when no slot has that name.
func respawnNamed(n *Npcs, name string) {
	if id, ok := n.SlotID(name); ok {
		n.Respawn(id)
	}
}
