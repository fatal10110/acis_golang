package manager

import (
	"maps"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// mustSlot returns the id of the spawn slot named name (Npcs.SlotID),
// failing t when no slot has that name.
func mustSlot(t testing.TB, n *Npcs, name string) task.SlotID {
	t.Helper()
	id, ok := n.SlotID(name)
	if !ok {
		t.Fatalf("no spawn slot named %s; live slots: %q", name, slices.Sorted(maps.Values(n.Slots())))
	}
	return id
}

// respawnArmed reports whether r has a respawn armed for the spawn slot
// named name, failing t when no slot has that name.
func respawnArmed(t testing.TB, n *Npcs, r *task.Respawn, name string) bool {
	t.Helper()
	return r.Tracked(mustSlot(t, n, name))
}

// cancelRespawn cancels the respawn r has armed for the spawn slot named
// name, reporting whether one was; it fails t when no slot has that name.
func cancelRespawn(t testing.TB, n *Npcs, r *task.Respawn, name string) bool {
	t.Helper()
	return r.Cancel(mustSlot(t, n, name))
}

// respawnNamed brings back the NPC of the spawn slot named name, as its
// respawn coming due does; it fails t when no slot has that name.
func respawnNamed(t testing.TB, n *Npcs, name string) {
	t.Helper()
	n.Respawn(mustSlot(t, n, name))
}
