package world

import (
	"slices"
	"testing"
)

// RefreshKnown has every object around the region holding the subject, the
// region it kept through a SetPosition jump, discover the subject and then
// be discovered by it, object by object; the landing's neighborhood is not
// told, nothing is forgotten, and an object off the grid refreshes nothing.
func TestRefreshKnownRediscoversTheKeptNeighborhood(t *testing.T) {
	s := New()
	log := &teleportLog{}
	for id, col := range map[int32]int{1: 49, 2: 51, 3: 53} {
		x, y := teleportRegionAt(col)
		s.Spawn(&teleportObserver{id: id, log: log}, x, y, 0, 0)
	}
	subject := &teleportObserver{id: 99, log: log}
	x, y := teleportRegionAt(50)
	s.Spawn(subject, x, y, 0, 0)
	log.take()

	nx, ny := teleportRegionAt(53)
	s.SetPosition(subject, nx, ny, 0)
	s.RefreshKnown(subject)
	want := []string{"1 discover 99", "99 discover 1", "2 discover 99", "99 discover 2"}
	if got := log.take(); !slices.Equal(got, want) {
		t.Fatalf("callbacks after RefreshKnown = %q, want %q", got, want)
	}

	s.Leave(subject)
	log.take()
	s.RefreshKnown(subject)
	if got := log.take(); len(got) != 0 {
		t.Fatalf("callbacks off the grid = %q, want none", got)
	}
}
