package npcs

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestRouteWalkerFolkWalksInAnInactiveRegion pins #4797: a route walker
// far from every player, in a region no player keeps active, still walks
// its route, node after node.
func TestRouteWalkerFolkWalksInAnInactiveRegion(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil)
	home := location.Location{X: w.at.X + 60000, Y: w.at.Y, Z: w.at.Z}
	a := location.Location{X: home.X, Y: home.Y + 60, Z: home.Z}
	delay := int(nodeDelay / time.Millisecond)
	f, walker := w.srv.SpawnRouteFolkNPC(t, gameservertest.RouteFolkSpawn{
		Template: walkerTemplate(),
		At:       home,
		Routes:   walkerRoute(route.WalkerLocation{Location: home, DelayMillis: delay}, route.WalkerLocation{Location: a, DelayMillis: delay}),
		WalkMode: true,
	})
	if w.srv.State.RegionActive(f) {
		t.Fatal("the walker's region is active, want it far from every player")
	}

	w.srv.AdvanceUntil(t, "walker stands on its spawn point", func() bool { return !f.IsMoving() && folkAt(f) == home })
	w.srv.Advance(t, nodeDelay)
	walker.Tick()
	w.srv.AdvanceUntil(t, "walker walks to the far node", func() bool { return !f.IsMoving() && folkAt(f) == a })
	w.srv.Advance(t, nodeDelay)
	walker.Tick()
	w.srv.AdvanceUntil(t, "walker walks back to its spawn point", func() bool { return !f.IsMoving() && folkAt(f) == home })
}
