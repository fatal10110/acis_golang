package pathfind

import "testing"

// A search whose origin and target resolve to the same cell and layer has no
// route: it reports no path rather than an empty found one.
func TestFindSameCellIsNoPath(t *testing.T) {
	finder := New(newTestEngine(t, flatOpenBlock()), DefaultOptions())
	origin := at(2, 2, 0)
	target := origin
	target.X++ // same 16-unit cell

	path, cost, ok := finder.Find(origin, target)
	if ok || len(path) != 0 || cost != 0 {
		t.Fatalf("Find(same cell) = (%#v, %d, %v), want (empty, 0, false)", path, cost, ok)
	}
	if finder.HasPath(origin, target) {
		t.Fatal("HasPath(same cell) = true, want false")
	}
}
