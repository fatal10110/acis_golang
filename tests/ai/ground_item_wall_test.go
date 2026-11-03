package ai

import (
	"slices"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// pickUpWallGeo is open ground with a wall across the x axis at wallX,
// 400 long, centered on wallY: a straight line crossing it is closed, and
// the pathfinder offers a detour 400 to the side of it. Until wallX is set
// there is no wall.
type pickUpWallGeo struct {
	gameservertest.Geo
	wallX, wallY *atomic.Int64
}

func newPickUpWallGeo() pickUpWallGeo {
	return pickUpWallGeo{wallX: new(atomic.Int64), wallY: new(atomic.Int64)}
}

func (g pickUpWallGeo) CanMove(ox, oy, _, tx, ty, _ int) bool {
	w := int(g.wallX.Load())
	if w == 0 || (ox < w) == (tx < w) {
		return true
	}
	y := oy + (ty-oy)*(w-ox)/(tx-ox)
	return y-int(g.wallY.Load()) >= 200 || int(g.wallY.Load())-y >= 200
}

func (g pickUpWallGeo) FindPath(from, to location.Location) ([]location.Location, bool) {
	return []location.Location{
		{X: from.X, Y: from.Y + 400, Z: from.Z},
		{X: to.X, Y: to.Y + 400, Z: to.Z},
		to,
	}, true
}

func (g pickUpWallGeo) CanFly(ox, oy, oz int, _ float64, tx, ty, tz int) bool {
	return g.CanMove(ox, oy, oz, tx, ty, tz)
}

// groundPicker is a picker NPC of either kind.
type groundPicker interface {
	attackable.Combatant
	IsMoving() bool
	Queue() *sim.Queue
}

// TestPickUpWalkStopsAtWall pins the NPC pick-up walk (NpcAI.thinkPickUp ->
// maybeMoveToLocation(item, 36, false, false): path-finding off): the walk
// heads straight for an item behind a wall, though a detour exists, and
// stops blocked at the wall; the item is never reached or picked up, however
// many times the step walks again. Hostile and civilian NPCs alike.
func TestPickUpWalkStopsAtWall(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		id   int32
	}{
		{name: "hostile", id: pickerHostileID},
		{name: "folk", id: pickerFolkID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			geo := newPickUpWallGeo()
			w := bootGround(t, gameservertest.WithGeo(geo))
			wallX := w.at.X + 350
			geo.wallY.Store(int64(w.at.Y))
			geo.wallX.Store(int64(wallX))

			p := w.spawn(t, tc.id, 200).(groundPicker)
			potion := w.drop(t, potionID, 500)
			w.look(t, p.Queue(), script.NPCOf(p), 500, potionID)
			if want := []string{line("see %d q=1 [%d/%d]", tc.id, potion.ObjectID(), potionID)}; !slices.Equal(w.log.take(), want) {
				t.Fatalf("look: want %q", want)
			}

			cycle(t, w.srv)
			moves := framesOf(cycle(t, w.srv), serverpackets.OpcodeMoveToLocation, p.ObjectID())
			if len(moves) != 1 {
				t.Fatalf("walks toward the item = %d, want 1", len(moves))
			}
			if got := moveDestination(moves[0]); got.X != w.at.X+500 || got.Y != w.at.Y {
				t.Fatalf("walk destination = %+v, want straight for the item at x %d, y %d", got, w.at.X+500, w.at.Y)
			}
			for range 3 {
				w.walk(t, p.IsMoving)
				if x, _, _ := p.Position(); x >= wallX {
					t.Fatalf("the NPC stands at x %d, want stopped short of the wall at x %d", x, wallX)
				}
				cycle(t, w.srv)
			}
			if got := w.log.take(); len(got) != 0 {
				t.Fatalf("hooks behind the wall = %q, want no pickup", got)
			}
			if _, ok := w.srv.State.Object(potion.ObjectID()); !ok {
				t.Fatal("the item behind the wall left the world")
			}
		})
	}
}
