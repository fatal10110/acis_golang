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
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
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

// TestMinionIdleWanderOffsetsFromCurrentPosition pins private idle wander:
// origin is the minion's current XY, not spawn home. Master stays in
// territory so the private is treated as in-territory even after leaving
// its own spawn point.
func TestMinionIdleWanderOffsetsFromCurrentPosition(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	current := location.Location{X: hostileX + 500, Y: hostileY, Z: hostileZ}
	master := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
	minion := srv.SpawnMovingHostileNPCAt(t, "Monster", home, current)
	master.AddMinion(minion)
	minion.SetMaster(master)
	drainUntilQuiet(t, c)

	if err := minion.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	if got := minion.AI().CurrentIntention(); got != ai.IntentionWander {
		t.Fatalf("CurrentIntention() = %v, want wander", got)
	}

	assertChangeMoveType(t, mustRead(t, c, "ChangeMoveType"), minion.ObjectID(), false)
	dest := moveToLocationDest(t, mustRead(t, c, "MoveToLocation"))
	offset := 60 * 3
	if absInt(dest.X-current.X) > offset || absInt(dest.Y-current.Y) > offset || dest.Z != current.Z {
		t.Fatalf("minion wander dest = %+v, want within ±%d of current %+v", dest, offset, current)
	}
	if absInt(dest.X-home.X) <= offset && absInt(dest.Y-home.Y) <= offset {
		t.Fatalf("minion wander dest = %+v, still within ±%d of spawn home %+v", dest, offset, home)
	}
	if !minion.IsMoving() {
		t.Fatal("IsMoving() = false after minion wander Think, want a live walk")
	}
}

// TestMinionIdleWanderContinuesWhenMasterDiesOffTerritory pins the corpse
// window: Go keeps the master pointer until decay. A dead master must not
// pull the private out of territory. The private keeps wandering around
// its current XY even when both actors are far from spawn home.
func TestMinionIdleWanderContinuesWhenMasterDiesOffTerritory(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	masterAt := location.Location{X: hostileX + 400, Y: hostileY, Z: hostileZ}
	current := location.Location{X: hostileX + 500, Y: hostileY, Z: hostileZ}
	master := srv.SpawnMovingHostileNPCAt(t, "Monster", home, masterAt)
	minion := srv.SpawnMovingHostileNPCAt(t, "Monster", home, current)
	master.AddMinion(minion)
	minion.SetMaster(master)
	if !master.MarkDead() {
		t.Fatal("MarkDead() = false, want a fresh death")
	}
	drainUntilQuiet(t, c)

	if err := minion.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	if got := minion.AI().CurrentIntention(); got != ai.IntentionWander {
		t.Fatalf("CurrentIntention() = %v, want wander after master death", got)
	}
	assertChangeMoveType(t, mustRead(t, c, "ChangeMoveType"), minion.ObjectID(), false)
	dest := moveToLocationDest(t, mustRead(t, c, "MoveToLocation"))
	offset := 60 * 3
	if absInt(dest.X-current.X) > offset || absInt(dest.Y-current.Y) > offset || dest.Z != current.Z {
		t.Fatalf("minion wander dest = %+v, want within ±%d of current %+v", dest, offset, current)
	}
}

// TestMinionOpeningHateUsesMasterTerritory pins a non-wander InTerritory
// consumer: a private 500 off its own spawn still gets the 300 opening
// hate while its living master is in territory.
func TestMinionOpeningHateUsesMasterTerritory(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	player := liveCombatant(t, srv)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	current := location.Location{X: hostileX + 500, Y: hostileY, Z: hostileZ}
	master := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
	minion := srv.SpawnMovingHostileNPCAt(t, "Monster", home, current)
	master.AddMinion(minion)
	minion.SetMaster(master)

	minion.AI().AddDefaultHate(player)
	if got := minion.AI().Hates().Hate(player); got != 300 {
		t.Fatalf("opening hate = %v, want 300 (master in territory)", got)
	}
}

// TestGuardDoesNotIdleWander pins hold-position kinds: a Guard with an empty
// desire queue stays idle instead of rolling a random walk.
func TestGuardDoesNotIdleWander(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Guard", home, home)
	drainUntilQuiet(t, c)

	if err := hostile.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle", got)
	}
	if hostile.IsMoving() {
		t.Fatal("IsMoving() = true for idle Guard, want no wander")
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
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
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
// miss: home sits inside this tiny triangle, and every walk offset leaves
// it, so the destination is the geo-validated triangle centroid
// (55+65+60)/3, (15+15+28)/3.
func TestMakerIdleWanderFallsBackToShapeCenter(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
	poly := wanderPoly(
		spawn.Node{X: 55, Y: 15},
		spawn.Node{X: 65, Y: 15},
		spawn.Node{X: 60, Y: 28},
	)
	hostile.Instance.Maker = &spawn.Maker{Territories: []*spawn.Territory{poly}}
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

// TestMakerIdleWanderOutOfTerritoryStaysIdle pins AttackableAI.thinkWander
// when the maker NPC is outside the polygon but still at spawn: returnHome
// is a no-op (inside 2D drift) and random walk is skipped, so intention
// drops to idle.
func TestMakerIdleWanderOutOfTerritoryStaysIdle(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
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
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle", got)
	}
	if hostile.IsMoving() {
		t.Fatal("IsMoving() = true for out-of-territory maker NPC at home, want idle")
	}
}

func wanderPoly(nodes ...spawn.Node) *spawn.Territory {
	return &spawn.Territory{Name: "wander", MinZ: 0, MaxZ: 100, Nodes: nodes}
}

func moveToLocationCoords(t *testing.T, frame []byte) (objectID int32, dest, origin location.Location) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeMoveToLocation, "MoveToLocation")
	r := wireReader(frame[1:])
	objectID = r.ReadInt32()
	dest = location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
	origin = location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
	return objectID, dest, origin
}

func moveToLocationDest(t *testing.T, frame []byte) location.Location {
	t.Helper()
	_, dest, _ := moveToLocationCoords(t, frame)
	return dest
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
