package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/geometry"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestIdleHostileWanderBroadcastsWalkThenMove pins AttackableAI.thinkWander's
// first idle step: walk stance, then a MoveToLocation offset from the spawn
// home (offset = walk speed * 3) on each axis.
func TestIdleHostileWanderBroadcastsWalkThenMove(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", 20001, home, home)
	drainUntilQuiet(t, c)

	if err := hostile.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionWander {
		t.Fatalf("CurrentIntention() = %v, want wander", got)
	}

	assertChangeMoveType(t, mustRead(t, c, "ChangeMoveType"), hostile.ObjectID(), false)
	move := mustRead(t, c, "MoveToLocation")
	dest := moveToLocationDest(t, move)
	offset := 60 * 3
	if absInt(dest.X-home.X) > offset || absInt(dest.Y-home.Y) > offset || dest.Z != home.Z {
		t.Fatalf("home-offset dest = %+v, want within ±%d of home %+v", dest, offset, home)
	}
	if !hostile.IsMoving() {
		t.Fatal("IsMoving() = false after wander Think, want a live walk")
	}
}

// TestGuardDoesNotIdleWander pins an unregistered Guard template: an empty
// desire queue stays idle instead of rolling a random walk.
func TestGuardDoesNotIdleWander(t *testing.T) {
	assertIdleNoWander(t, "Guard", 100)
}

// TestGuardMoveAroundIdleWanders pins opt-in Guard templates: 31845 queues
// wander the same way a field monster does.
func TestGuardMoveAroundIdleWanders(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Guard", 31845, home, home)
	drainUntilQuiet(t, c)

	if err := hostile.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionWander {
		t.Fatalf("CurrentIntention() = %v, want wander", got)
	}
	assertChangeMoveType(t, mustRead(t, c, "ChangeMoveType"), hostile.ObjectID(), false)
	assertFrameOpcode(t, mustRead(t, c, "MoveToLocation"), serverpackets.OpcodeMoveToLocation, "MoveToLocation")
}

func TestWarriorHoldDoesNotIdleWander(t *testing.T) {
	assertIdleNoWander(t, "Monster", 27102)
}

func TestUnscriptedSquashDoesNotIdleWander(t *testing.T) {
	assertIdleNoWander(t, "Monster", 12774)
}

func TestLavasaurusDoesNotIdleWander(t *testing.T) {
	assertIdleNoWander(t, "Monster", 21394)
}

func TestNurseAntIdleWanderTimer(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", 29003, home, home)
	drainUntilQuiet(t, c)

	if err := hostile.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	d, ok := hostile.AI().Desires().Peek()
	if !ok || d.Kind != ai.IntentionWander {
		t.Fatalf("Peek() = (%v, %v), want wander desire", d, ok)
	}
	if d.Timer != 40 || d.Weight != 20 {
		t.Fatalf("wander timer/weight = %d/%v, want 40/20", d.Timer, d.Weight)
	}
}

func assertIdleNoWander(t *testing.T, kind string, npcID int) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, kind, npcID, home, home)
	drainUntilQuiet(t, c)

	if err := hostile.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle", got)
	}
	if hostile.IsMoving() {
		t.Fatal("IsMoving() = true, want no wander")
	}
}

// TestMakerIdleWanderStaysInsideTerritory pins MultiSpawn's in-territory
// sample: a maker NPC's wander destination stays inside the maker polygon
// and is an offset sample, not the triangle-center fallback.
func TestMakerIdleWanderStaysInsideTerritory(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", 20001, home, home)
	poly := wanderPoly(
		spawn.Node{X: -2000, Y: -2000},
		spawn.Node{X: 2000, Y: -2000},
		spawn.Node{X: 2000, Y: 2000},
		spawn.Node{X: -2000, Y: 2000},
	)
	maker := &spawn.Maker{Territories: []*spawn.Territory{poly}}
	hostile.Instance.Maker = maker
	drainUntilQuiet(t, c)

	if err := hostile.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	assertChangeMoveType(t, mustRead(t, c, "ChangeMoveType"), hostile.ObjectID(), false)
	dest := moveToLocationDest(t, mustRead(t, c, "MoveToLocation"))
	if !poly.Contains(dest.X, dest.Y, dest.Z) {
		t.Fatalf("maker wander dest = %+v, want inside territory", dest)
	}
	if dest.Distance2D(home) > 180 {
		t.Fatalf("maker wander dest = %+v, 2D distance from home %v > offset 180 (center fallback)", dest, dest.Distance2D(home))
	}
	tri, ok := maker.ContainingTriangle(home.X, home.Y)
	if !ok {
		t.Fatal("home is outside the maker polygon")
	}
	center := tri.Center()
	if dest.X == center.X && dest.Y == center.Y && dest.Z == home.Z {
		t.Fatalf("maker wander dest = %+v, want an offset sample not the shape center", dest)
	}
}

// TestMakerIdleWanderFallsBackToShapeCenter pins MultiSpawn's three-sample
// miss: every offset leaves the tiny triangle (and the same polygon is
// banned), so the destination is the geo-validated triangle centroid
// (55+65+60)/3, (15+15+28)/3.
func TestMakerIdleWanderFallsBackToShapeCenter(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", 20001, home, home)
	poly := wanderPoly(
		spawn.Node{X: 55, Y: 15},
		spawn.Node{X: 65, Y: 15},
		spawn.Node{X: 60, Y: 28},
	)
	hostile.Instance.Maker = &spawn.Maker{
		Territories:       []*spawn.Territory{poly},
		BannedTerritories: []*spawn.Territory{poly},
	}
	drainUntilQuiet(t, c)

	if err := hostile.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	assertChangeMoveType(t, mustRead(t, c, "ChangeMoveType"), hostile.ObjectID(), false)
	dest := moveToLocationDest(t, mustRead(t, c, "MoveToLocation"))
	want := geometry.Point{X: (55 + 65 + 60) / 3, Y: (15 + 15 + 28) / 3}
	if dest.X != want.X || dest.Y != want.Y || dest.Z != home.Z {
		t.Fatalf("center-fallback dest = %+v, want {%d %d %d}", dest, want.X, want.Y, home.Z)
	}
}

// TestMakerIdleWanderOutOfTerritoryPicksTerritoryPoint pins MultiSpawn when
// the NPC's current XY is outside every triangle: destination is a
// geo-height sample inside the maker territory (avgZ, not the NPC's Z).
func TestMakerIdleWanderOutOfTerritoryPicksTerritoryPoint(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", 20001, home, home)
	poly := wanderPoly(
		spawn.Node{X: 500, Y: 500},
		spawn.Node{X: 800, Y: 500},
		spawn.Node{X: 800, Y: 800},
		spawn.Node{X: 500, Y: 800},
	)
	hostile.Instance.Maker = &spawn.Maker{Territories: []*spawn.Territory{poly}}
	drainUntilQuiet(t, c)

	if err := hostile.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	assertChangeMoveType(t, mustRead(t, c, "ChangeMoveType"), hostile.ObjectID(), false)
	dest := moveToLocationDest(t, mustRead(t, c, "MoveToLocation"))
	if !poly.Contains2D(dest.X, dest.Y) {
		t.Fatalf("out-of-territory dest XY = (%d,%d), want inside 500..800 square", dest.X, dest.Y)
	}
	if dest.Z != 50 {
		t.Fatalf("out-of-territory dest Z = %d, want avgZ 50", dest.Z)
	}
}

func wanderPoly(nodes ...spawn.Node) *spawn.Territory {
	return &spawn.Territory{Name: "wander", MinZ: 0, MaxZ: 100, Nodes: nodes}
}

func moveToLocationDest(t *testing.T, frame []byte) location.Location {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeMoveToLocation, "MoveToLocation")
	r := wireReader(frame[1:])
	_ = r.ReadInt32()
	return location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
