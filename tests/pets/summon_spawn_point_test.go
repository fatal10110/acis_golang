package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// summonSpawnOffset is how far a summon appears from its owner along each
// axis its direction points.
const summonSpawnOffset = 40

// halfwayGeo is geodata with a wall halfway to every target: a valid
// location stops half way from the origin, on a floor 8 above it.
type halfwayGeo struct{ gameservertest.Geo }

func (halfwayGeo) ValidLocation(ox, oy, oz, tx, ty, _ int) location.Location {
	return location.Location{X: ox + (tx-ox)/2, Y: oy + (ty-oy)/2, Z: oz + 8}
}

// requireSpawnBeside checks s stands step along each axis of one of the
// eight compass directions from its owner, at its owner's Z plus dz, facing
// its owner from the point summonSpawnOffset out in that direction, and
// returns the direction.
func requireSpawnBeside(t *testing.T, srv *gameservertest.Server, ownerID int32, s *summon.Actor, step, dz int) [2]int {
	t.Helper()
	ox, oy, oz := srv.PlayerPosition(t, ownerID)
	sx, sy, sz := s.Position()
	dx, dy := sx-ox, sy-oy
	axis := func(d int) bool { return d == 0 || d == step || d == -step }
	if !axis(dx) || !axis(dy) || (dx == 0 && dy == 0) || sz != oz+dz {
		t.Fatalf("summon at (%d,%d,%d) beside owner (%d,%d,%d), want %d along one of the 8 directions at Z %d",
			sx, sy, sz, ox, oy, oz, step, oz+dz)
	}
	dir := [2]int{dx / step, dy / step}
	from := location.Location{X: ox + dir[0]*summonSpawnOffset, Y: oy + dir[1]*summonSpawnOffset}
	if want := from.HeadingTo(location.Location{X: ox, Y: oy}); s.Heading() != want {
		t.Fatalf("summon in direction %v heading = %d, want %d facing its owner", dir, s.Heading(), want)
	}
	return dir
}

// dismissServitor sends the servitor unsummon shortcut and waits until the
// owner's summon slot is free.
func (o *servitorOwner) dismissServitor(t *testing.T) {
	t.Helper()
	o.client.Send(encodeRequestActionUse(52, false))
	o.srv.AdvanceUntil(t, "servitor gone", func() bool {
		_, ok := o.srv.State.Summon(o.id)
		return !ok
	})
	drainUntilQuiet(t, o.client)
}

// TestServitorSpawnsInARandomDirectionFacingOwner calls a servitor out
// again and again: each one appears 40 out along one of the eight compass
// directions, facing its owner, and the direction changes between calls.
func TestServitorSpawnsInARandomDirectionFacingOwner(t *testing.T) {
	t.Parallel()
	o := bootServitorOwner(t, gameservertest.WithRandomSummonSpawn())
	seen := map[[2]int]bool{}
	for range 12 {
		servitor := o.summonServitor(t)
		seen[requireSpawnBeside(t, o.srv, o.id, servitor, summonSpawnOffset, 0)] = true
		o.dismissServitor(t)
	}
	if len(seen) < 2 {
		t.Fatalf("12 servitors all spawned in direction %v, want a random direction each call", seen)
	}
}

// TestPetSpawnsBesideOwnerFacingIt calls a pet out of its collar: it
// appears 40 out along one of the eight compass directions, facing its
// owner.
func TestPetSpawnsBesideOwnerFacingIt(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithRandomSummonSpawn()})
	pet, _ := h.spawnWolf(t)
	requireSpawnBeside(t, h.srv, h.ownerID, pet, summonSpawnOffset, 0)
}

// TestSummonSpawnStopsWhereGeodataAllows calls a pet and a servitor out
// beside a wall: each stops where geodata lets it stand on the way out from
// its owner, on the floor geodata reports there, still facing its owner.
func TestSummonSpawnStopsWhereGeodataAllows(t *testing.T) {
	t.Parallel()
	t.Run("pet", func(t *testing.T) {
		t.Parallel()
		h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithGeo(halfwayGeo{}), gameservertest.WithRandomSummonSpawn()})
		pet, _ := h.spawnWolf(t)
		requireSpawnBeside(t, h.srv, h.ownerID, pet, summonSpawnOffset/2, 8)
	})
	t.Run("servitor", func(t *testing.T) {
		t.Parallel()
		o := bootServitorOwner(t, gameservertest.WithGeo(halfwayGeo{}), gameservertest.WithRandomSummonSpawn())
		requireSpawnBeside(t, o.srv, o.id, o.summonServitor(t), summonSpawnOffset/2, 8)
	})
}
