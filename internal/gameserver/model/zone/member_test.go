package zone

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// memberStub is a non-player occupant whose zones follow a Member.
type memberStub struct {
	Member
	id          int32
	class       Class
	at          location.Location
	teleporting bool
}

func (s *memberStub) ObjectID() int32             { return s.id }
func (s *memberStub) Position() location.Location { return s.at }
func (s *memberStub) Class() Class                { return s.class }
func (s *memberStub) Teleporting() bool           { return s.teleporting }

func testCuboid(t *testing.T, x1, x2, y1, y2 int) Form {
	t.Helper()
	f, err := NewCuboid(x1, x2, y1, y2, -100, 100)
	if err != nil {
		t.Fatalf("NewCuboid: %v", err)
	}
	return f
}

// TestMemberRevalidationCadence pins when a creature's zones follow its
// position: a spawn, a move's end and a region change revalidate at once;
// plain movement steps only every fifth step (a step counter that a forced
// revalidation restarts); nothing enters a zone between Leave and Enter, and
// a teleport under way counts steps without revalidating.
func TestMemberRevalidationCadence(t *testing.T) {
	ix := NewIndex()
	ix.Add(NewPeace(1, testCuboid(t, 0, 1000, 0, 1000)))
	s := &memberStub{id: 1, class: ClassNPC, at: location.Location{X: 2000, Y: 500}}
	in := func() bool { return s.ZoneFlags().Has(FlagPeace) }
	step := func(x int) {
		previous := s.at
		s.at = location.Location{X: x, Y: 500}
		s.Step(ix, s, previous)
	}

	s.Step(ix, s, s.at)
	s.at = location.Location{X: 500, Y: 500}
	s.Settle(ix, s)
	if in() {
		t.Fatal("before Enter: a settle entered a zone")
	}

	s.at = location.Location{X: 2000, Y: 500}
	s.Enter(ix, s)
	if in() {
		t.Fatal("spawned outside the zone: in peace")
	}
	for i := 1; i <= 4; i++ {
		step(500 + i)
		if in() {
			t.Fatalf("step %d inside the zone: entered before the fifth step", i)
		}
	}
	step(505)
	if !in() {
		t.Fatal("fifth step inside the zone: not entered")
	}

	step(2000)
	if !in() {
		t.Fatal("first step out: left before the fifth step")
	}
	s.Settle(ix, s)
	if in() {
		t.Fatal("move ended outside the zone: still in peace")
	}

	// A settle restarts the count: four steps later the zone is still not
	// entered.
	for i := 1; i <= 4; i++ {
		step(500 + i)
	}
	if in() {
		t.Fatal("four steps after a settle: entered")
	}

	s.Leave(ix, s, s.at.X, s.at.Y)
	s.Settle(ix, s)
	step(500)
	if in() {
		t.Fatal("off the grid: a position change entered a zone")
	}

	s.teleporting = true
	s.Enter(ix, s)
	if in() {
		t.Fatal("mid-teleport: Enter revalidated")
	}
	s.teleporting = false
	s.Enter(ix, s)
	if !in() {
		t.Fatal("teleport landed inside the zone: not entered")
	}
	s.Leave(ix, s, s.at.X, s.at.Y)
	if in() {
		t.Fatal("Leave kept the zone's flag")
	}
}

// TestMemberRegionChangeRevalidatesAtOnce pins that a step crossing into
// another world region revalidates the zones of both regions at once,
// without waiting for the fifth step.
func TestMemberRegionChangeRevalidatesAtOnce(t *testing.T) {
	edge := world.MinX + 100*regionEdge
	ix := NewIndex()
	ix.Add(NewPeace(1, testCuboid(t, edge, edge+1000, 0, 1000)))
	ix.Add(NewArena(2, testCuboid(t, edge-1000, edge-1, 0, 1000)))
	s := &memberStub{id: 1, class: ClassNPC, at: location.Location{X: edge - 10, Y: 500}}
	s.Enter(ix, s)
	if !s.ZoneFlags().Has(FlagNoSummonFriend) {
		t.Fatal("spawned in the arena: no arena flag")
	}
	previous := s.at
	s.at = location.Location{X: edge + 10, Y: 500}
	if world.RegionKey(previous.X, previous.Y) == world.RegionKey(s.at.X, s.at.Y) {
		t.Fatal("test setup: the step does not change region")
	}
	s.Step(ix, s, previous)
	if !s.ZoneFlags().Has(FlagPeace) {
		t.Fatal("a region-changing step did not enter the new region's zone")
	}
	if s.ZoneFlags().Has(FlagNoSummonFriend) {
		t.Fatal("a region-changing step did not leave the old region's zone")
	}
}

// TestNPCPeaceFromZoneMembership pins which zones pacify an NPC occupant: a
// Peace zone always, a peaceful Town unless its combat rule disables peace
// (rule 1 only exempts siege players), never a non-peaceful Town, and never
// a derby track, which pacifies playables only. The z bound holds no peace.
func TestNPCPeaceFromZoneMembership(t *testing.T) {
	town := func(id, x int, peaceful bool, rule int) *Town {
		return &Town{Zone: newZone(id, testCuboid(t, x, x+1000, 0, 1000)), Peaceful: peaceful, CombatRule: rule}
	}
	ix := NewIndex()
	ix.Add(NewPeace(1, testCuboid(t, 0, 1000, 0, 1000)))
	ix.Add(town(2, 2000, true, 0))
	ix.Add(town(3, 4000, false, 0))
	ix.Add(town(4, 6000, true, 2))
	ix.Add(town(5, 8000, true, 1))
	ix.Add(NewDerbyTrack(6, testCuboid(t, 10000, 11000, 0, 1000)))

	cases := []struct {
		name    string
		x, y, z int
		want    bool
	}{
		{"peace zone", 500, 500, 0, true},
		{"peaceful town", 2500, 500, 0, true},
		{"non-peaceful town", 4500, 500, 0, false},
		{"town with combat rule 2", 6500, 500, 0, false},
		{"town with combat rule 1", 8500, 500, 0, true},
		{"derby track", 10500, 500, 0, false},
		{"above peaceful town z bound", 2500, 500, 101, false},
		{"above peace zone z bound", 500, 500, 101, false},
		{"outside every zone", 12500, 500, 0, false},
	}
	for i, tc := range cases {
		s := &memberStub{id: int32(i + 1), class: ClassNPC, at: location.Location{X: tc.x, Y: tc.y, Z: tc.z}}
		s.Enter(ix, s)
		if got := s.ZoneFlags().Has(FlagPeace); got != tc.want {
			t.Errorf("%s: NPC peace flag = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestSummonCombatFlagsFromZoneMembership pins the combat flags a summon
// occupant's zones raise: PvP from an arena, an active siege or a running
// stadium, cancelled by any peace hold; siege from an active siege only.
func TestSummonCombatFlagsFromZoneMembership(t *testing.T) {
	cuboid := func(x int) Form { return testCuboid(t, x, x+1000, 0, 1000) }
	stadium := NewOlympiad(7, cuboid(12000))
	stadium.BattleStarted = func() bool { return true }
	activeSiege := &Siege{Zone: newZone(3, cuboid(4000))}
	activeSiege.SetActive(true)
	peacefulSiege := &Siege{Zone: newZone(5, cuboid(8000))}
	peacefulSiege.SetActive(true)

	ix := NewIndex()
	ix.Add(NewArena(1, cuboid(0)))
	ix.Add(NewArena(2, cuboid(2000)))
	ix.Add(NewPeace(20, cuboid(2000)))
	ix.Add(activeSiege)
	ix.Add(&Siege{Zone: newZone(4, cuboid(6000))})
	ix.Add(peacefulSiege)
	ix.Add(&Town{Zone: newZone(21, cuboid(8000)), Peaceful: true})
	ix.Add(NewArena(6, cuboid(10000)))
	ix.Add(&Town{Zone: newZone(22, cuboid(10000)), Peaceful: true, CombatRule: 2})
	ix.Add(stadium)
	ix.Add(NewArena(8, cuboid(14000)))
	ix.Add(NewDerbyTrack(23, cuboid(14000)))

	cases := []struct {
		name       string
		x          int
		pvp, siege bool
	}{
		{"arena", 500, true, false},
		{"arena under a peace zone", 2500, false, false},
		{"active siege", 4500, true, true},
		{"inactive siege", 6500, false, false},
		{"active siege inside a peaceful town", 8500, false, true},
		{"arena inside a town with combat rule 2", 10500, true, false},
		{"stadium with a match running", 12500, true, false},
		{"arena under a derby track", 14500, false, false},
		{"outside every zone", 16500, false, false},
	}
	for i, tc := range cases {
		s := &memberStub{id: int32(i + 1), class: ClassSummon, at: location.Location{X: tc.x, Y: 500}}
		s.Enter(ix, s)
		if got := s.ZoneFlags().Has(FlagPvP); got != tc.pvp {
			t.Errorf("%s: PvP %v, want %v", tc.name, got, tc.pvp)
		}
		if got := s.ZoneFlags().Has(FlagSiege); got != tc.siege {
			t.Errorf("%s: siege %v, want %v", tc.name, got, tc.siege)
		}
	}
}
