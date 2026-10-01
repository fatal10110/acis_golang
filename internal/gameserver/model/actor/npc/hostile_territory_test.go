package npc

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestInTerritoryIsStrict3D(t *testing.T) {
	// Spawn.isInMyTerritory uses Location.isIn3DRadius: distance3D < MAX_DRIFT_RANGE.
	// Axis-aligned integer offsets make sqrt(d²) == d, so 199 / 200 / 201 are
	// the exact representable neighbors of the boundary. Pure-Z cases fail
	// if InTerritory drops to 2D.
	home := location.Location{X: 100, Y: 0, Z: 0}
	cases := []struct {
		axis   string
		offset int
		want   bool
	}{
		{axis: "x", offset: defaultDriftRange - 1, want: true},
		{axis: "x", offset: defaultDriftRange, want: false},
		{axis: "x", offset: defaultDriftRange + 1, want: false},
		{axis: "z", offset: defaultDriftRange - 1, want: true},
		{axis: "z", offset: defaultDriftRange, want: false},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s=%d", tc.axis, tc.offset), func(t *testing.T) {
			hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
			hostile.Instance.HasHome = true
			hostile.Instance.Home = home
			x, y, z := home.X, home.Y, home.Z
			switch tc.axis {
			case "x":
				x += tc.offset
			case "z":
				z += tc.offset
			}
			world.New().Spawn(hostile, x, y, z, 0)

			if got := hostile.InTerritory(); got != tc.want {
				t.Fatalf("InTerritory() = %v at 3D %s distance %d, want %v", got, tc.axis, tc.offset, tc.want)
			}
		})
	}
}

// MultiSpawn.isInMyTerritory uses banned-then-allowed polygon containment,
// not Spawn's MAX_DRIFT_RANGE sphere. A point 250 from home is outside the
// sphere and still inside this rectangle (Nightmare Lord / dion13_2222_01).
func TestInTerritoryMakerUsesPolygonNotHomeSphere(t *testing.T) {
	home := location.Location{X: 100, Y: 0, Z: 0}
	movement := &hostileMove{}
	hostile := newTestHostile(t, movement, &hostileAttack{})
	hostile.Instance.HasHome = true
	hostile.Instance.Home = home
	hostile.Instance.Maker = &spawn.Maker{Territories: []*spawn.Territory{makerPoly()}}
	world.New().Spawn(hostile, home.X+250, home.Y, home.Z, 0)

	if !hostile.InTerritory() {
		t.Fatal("InTerritory() = false at home+250 inside maker polygon, want true")
	}
	if hostile.ReturnHome() {
		t.Fatal("ReturnHome() = true inside maker polygon, want no-op")
	}
	if movement.home != (location.Location{}) {
		t.Fatalf("MoveHome destination = %#v, want no walk-back", movement.home)
	}
}

func TestInTerritoryNilMakerKeepsHomeSphere(t *testing.T) {
	home := location.Location{X: 100, Y: 0, Z: 0}
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	hostile.Instance.HasHome = true
	hostile.Instance.Home = home
	world.New().Spawn(hostile, home.X+250, home.Y, home.Z, 0)

	if hostile.InTerritory() {
		t.Fatal("InTerritory() = true at home+250 with nil Maker, want false")
	}
}

func TestInTerritoryEmptyMakerTerritoriesKeepsHomeSphere(t *testing.T) {
	home := location.Location{X: 100, Y: 0, Z: 0}
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	hostile.Instance.HasHome = true
	hostile.Instance.Home = home
	hostile.Instance.Maker = &spawn.Maker{}
	world.New().Spawn(hostile, home.X+100, home.Y, home.Z, 0)

	if !hostile.InTerritory() {
		t.Fatal("InTerritory() = false at home+100 with empty maker territories, want true (home sphere)")
	}
}

func TestInTerritoryMakerOutsidePolygon(t *testing.T) {
	home := location.Location{X: 100, Y: 0, Z: 0}
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	hostile.Instance.HasHome = true
	hostile.Instance.Home = home
	hostile.Instance.Maker = &spawn.Maker{Territories: []*spawn.Territory{makerPoly()}}
	world.New().Spawn(hostile, 5000, 0, 0, 0)

	if hostile.InTerritory() {
		t.Fatal("InTerritory() = true outside maker polygon, want false")
	}
}

func TestInTerritoryMakerBannedOverridesAllowed(t *testing.T) {
	home := location.Location{X: 100, Y: 0, Z: 0}
	allowed := makerPoly()
	banned := &spawn.Territory{
		Name: "banned",
		MinZ: -1000,
		MaxZ: 1000,
		Nodes: []spawn.Node{
			{X: 300, Y: -50},
			{X: 400, Y: -50},
			{X: 400, Y: 50},
			{X: 300, Y: 50},
		},
	}
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	hostile.Instance.HasHome = true
	hostile.Instance.Home = home
	hostile.Instance.Maker = &spawn.Maker{
		Territories:       []*spawn.Territory{allowed},
		BannedTerritories: []*spawn.Territory{banned},
	}
	world.New().Spawn(hostile, home.X+250, home.Y, home.Z, 0)

	if hostile.InTerritory() {
		t.Fatal("InTerritory() = true inside banned territory, want false")
	}
}

// A private with a living master uses the master's territory, not its own
// spawn sphere. ReturnHome's walk-back still requires the minion to be
// outside its own 2D drift range, so that assertion cannot share a
// placement with the "follows master's false" pin.
func TestInTerritoryMinionFollowsMaster(t *testing.T) {
	home := location.Location{X: 100, Y: 0, Z: 0}
	cases := []struct {
		name            string
		masterOff       int
		minionOff       int
		deadMaster      bool
		wantInTerritory bool
		wantReturnHome  bool
	}{
		{
			name:            "master in sphere, minion at exclusive edge",
			minionOff:       defaultDriftRange,
			wantInTerritory: true,
		},
		{
			name:      "master outside, minion at own home",
			masterOff: defaultDriftRange,
		},
		{
			name:           "both outside own sphere",
			masterOff:      defaultDriftRange,
			minionOff:      defaultDriftRange,
			wantReturnHome: true,
		},
		{
			name:            "dead master, minion at exclusive edge",
			masterOff:       defaultDriftRange,
			minionOff:       defaultDriftRange,
			deadMaster:      true,
			wantInTerritory: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			minionMove := &hostileMove{}
			master := newTestHostile(t, &hostileMove{}, &hostileAttack{})
			minion := newTestHostile(t, minionMove, &hostileAttack{})
			minion.Instance.ObjectID = 102
			master.Instance.HasHome = true
			master.Instance.Home = home
			minion.Instance.HasHome = true
			minion.Instance.Home = home
			master.AddMinion(minion)
			minion.SetMaster(master)

			w := world.New()
			w.Spawn(master, home.X+tc.masterOff, home.Y, home.Z, 0)
			w.Spawn(minion, home.X+tc.minionOff, home.Y, home.Z, 0)
			if tc.deadMaster {
				if !master.MarkDead() {
					t.Fatal("MarkDead() = false, want a fresh death")
				}
			}

			if got := minion.InTerritory(); got != tc.wantInTerritory {
				t.Fatalf("InTerritory() = %v, want %v", got, tc.wantInTerritory)
			}
			if got := minion.ReturnHome(); got != tc.wantReturnHome {
				t.Fatalf("ReturnHome() = %v, want %v", got, tc.wantReturnHome)
			}
			if tc.wantReturnHome {
				if minionMove.home != home {
					t.Fatalf("MoveHome destination = %#v, want %#v", minionMove.home, home)
				}
			} else if minionMove.home != (location.Location{}) {
				t.Fatalf("MoveHome destination = %#v, want no walk-back", minionMove.home)
			}
		})
	}
}

func makerPoly() *spawn.Territory {
	return &spawn.Territory{
		Name: "maker",
		MinZ: -1000,
		MaxZ: 1000,
		Nodes: []spawn.Node{
			{X: -1000, Y: -1000},
			{X: 2000, Y: -1000},
			{X: 2000, Y: 2000},
			{X: -1000, Y: 2000},
		},
	}
}

func TestReturnHomeAtExactTerritoryBoundaryWalksBack(t *testing.T) {
	home := location.Location{X: 100, Y: 0, Z: 0}
	movement := &hostileMove{}
	hostile := newTestHostile(t, movement, &hostileAttack{})
	hostile.Instance.Kind = "Monster"
	hostile.Instance.HasHome = true
	hostile.Instance.Home = home
	world.New().Spawn(hostile, home.X+defaultDriftRange, home.Y, home.Z, 0)

	if hostile.InTerritory() {
		t.Fatal("InTerritory() = true at 3D distance 200, want false")
	}
	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false at same-Z offset 200, want walk-back")
	}
	if movement.home != home {
		t.Fatalf("MoveHome destination = %#v, want %#v", movement.home, home)
	}
}

func TestReturnHomeDriftRangeIsStrict2D(t *testing.T) {
	// Point2D.isIn2DRadius is distance2D < radius. Axis-aligned integer
	// offsets make hypot(d, 0) == d, so d-1 / d / d+1 are the exact
	// representable neighbors of the boundary.
	//
	// Ordinary Attackable and Guard also gate on 3D territory first. Lift
	// Z so that check is already false and the 2D drift predicate is the
	// one under test. SiegeGuard skips territory.
	const aboveTerritory = 1000
	home := location.Location{X: 100, Y: 0, Z: 0}

	cases := []struct {
		kind     InstanceKind
		offset   int
		z        int
		wantHome bool
	}{
		{kind: "Monster", offset: defaultDriftRange - 1, z: aboveTerritory, wantHome: false},
		{kind: "Monster", offset: defaultDriftRange, z: aboveTerritory, wantHome: true},
		{kind: "Monster", offset: defaultDriftRange + 1, z: aboveTerritory, wantHome: true},
		{kind: "Guard", offset: 19, z: aboveTerritory, wantHome: false},
		{kind: "Guard", offset: 20, z: aboveTerritory, wantHome: true},
		{kind: "Guard", offset: 21, z: aboveTerritory, wantHome: true},
		{kind: "SiegeGuard", offset: 19, z: 0, wantHome: false},
		{kind: "SiegeGuard", offset: 20, z: 0, wantHome: true},
		{kind: "SiegeGuard", offset: 21, z: 0, wantHome: true},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s/%d", tc.kind, tc.offset), func(t *testing.T) {
			movement := &hostileMove{}
			hostile := newTestHostile(t, movement, &hostileAttack{})
			hostile.Instance.Kind = tc.kind
			hostile.Instance.HasHome = true
			hostile.Instance.Home = home
			world.New().Spawn(hostile, home.X+tc.offset, home.Y, tc.z, 0)

			got := hostile.ReturnHome()
			if got != tc.wantHome {
				t.Fatalf("ReturnHome() = %v, want %v", got, tc.wantHome)
			}
			if tc.wantHome {
				if tc.kind == "SiegeGuard" {
					if movement.home != (location.Location{}) {
						t.Fatalf("MoveHome destination = %#v, want no walk until RunAI", movement.home)
					}
					if err := hostile.RunAI(); err != nil {
						t.Fatalf("RunAI() error: %v", err)
					}
				}
				if movement.home != home {
					t.Fatalf("MoveHome destination = %#v, want %#v", movement.home, home)
				}
				return
			}
			if movement.home != (location.Location{}) {
				t.Fatalf("MoveHome destination = %#v, want no walk-back", movement.home)
			}
		})
	}
}

func TestSiegeGuardReturnHomeBypassesTerritoryGate(t *testing.T) {
	movement := &hostileMove{}
	hostile := newTestHostile(t, movement, &hostileAttack{})
	hostile.Instance.Kind = "SiegeGuard"
	hostile.Instance.HasHome = true
	hostile.Instance.Home = location.Location{}
	world.New().Spawn(hostile, 100, 0, 0, 0)

	if !hostile.InTerritory() {
		t.Fatal("InTerritory() = false at 100 units, want true for the global 200-unit territory")
	}
	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false, want SiegeGuard to return outside its 20-unit drift range")
	}
	if movement.home != (location.Location{}) {
		t.Fatalf("MoveHome destination = %#v, want no walk until RunAI", movement.home)
	}
	if err := hostile.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := movement.home; got != hostile.Instance.Home {
		t.Fatalf("MoveHome destination = %#v, want %#v", got, hostile.Instance.Home)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionMoveTo {
		t.Fatalf("CurrentIntention() = %v, want %v", got, ai.IntentionMoveTo)
	}
}

func TestSiegeGuardUnreachableHomeTeleportsAfterFailLimit(t *testing.T) {
	movement := &hostileMove{denyMove: true}
	hostile := newTestHostile(t, movement, &hostileAttack{})
	hostile.Instance.Kind = "SiegeGuard"
	hostile.Instance.HasHome = true
	home := location.Location{X: 100, Y: 0, Z: 0}
	hostile.Instance.Home = home
	world.New().Spawn(hostile, home.X+100, home.Y, home.Z, 0)

	for i := 1; i <= move.HomeGeoFailLimit; i++ {
		if !hostile.ReturnHome() {
			t.Fatalf("ReturnHome() = false on attempt %d, want true", i)
		}
		if movement.home != (location.Location{}) {
			t.Fatalf("MoveHome on attempt %d = %#v, want no walk", i, movement.home)
		}
		if got := hostile.GeoPathFailCount(); got != i {
			t.Fatalf("GeoPathFailCount() = %d after attempt %d, want %d", got, i, i)
		}
	}
	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false after fail limit, want teleport via MoveHome")
	}
	if movement.home != home {
		t.Fatalf("MoveHome destination = %#v, want %#v", movement.home, home)
	}
}

func TestSiegeGuardMovementDisabledDoesNotCountGeoFail(t *testing.T) {
	movement := &hostileMove{}
	hostile := newTestHostile(t, movement, &hostileAttack{})
	hostile.Instance.Kind = "SiegeGuard"
	hostile.Instance.HasHome = true
	hostile.Instance.Template.CanMove = false
	home := location.Location{X: 100, Y: 0, Z: 0}
	hostile.Instance.Home = home
	world.New().Spawn(hostile, home.X+100, home.Y, home.Z, 0)

	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false, want true outside drift range")
	}
	if got := hostile.GeoPathFailCount(); got != 0 {
		t.Fatalf("GeoPathFailCount() = %d, want 0 when movement disabled", got)
	}
	if movement.home != (location.Location{}) {
		t.Fatalf("MoveHome destination = %#v, want no walk", movement.home)
	}
}

func TestAddGeoPathFailCountOverflowResetsWithoutIncrement(t *testing.T) {
	const max = 2
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	hostile.SetMaxGeoPathFailCount(max)
	for i := 1; i <= max; i++ {
		hostile.AddGeoPathFailCount()
		if got := hostile.GeoPathFailCount(); got != i {
			t.Fatalf("GeoPathFailCount() = %d after %d fails, want %d", got, i, i)
		}
	}
	hostile.AddGeoPathFailCount()
	if got := hostile.GeoPathFailCount(); got != max+1 {
		t.Fatalf("GeoPathFailCount() after max+1 = %d, want %d", got, max+1)
	}
	hostile.AddGeoPathFailCount()
	if got := hostile.GeoPathFailCount(); got != 0 {
		t.Fatalf("GeoPathFailCount() after overflow reset = %d, want 0", got)
	}
	hostile.AddGeoPathFailCount()
	if got := hostile.GeoPathFailCount(); got != 1 {
		t.Fatalf("GeoPathFailCount() after reset increment = %d, want 1", got)
	}
}

func TestHostileTeleportToClearsGeoPathFailCount(t *testing.T) {
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	w := world.New()
	w.Spawn(hostile, 0, 0, 0, 0)
	hostile.Attach(Runtime{World: w})
	for range 7 {
		hostile.AddGeoPathFailCount()
	}
	hostile.TeleportTo(location.Location{X: 50, Y: 0, Z: 0})
	if got := hostile.GeoPathFailCount(); got != 0 {
		t.Fatalf("GeoPathFailCount() after TeleportTo = %d, want 0", got)
	}
	x, y, z := hostile.Position()
	if got := (location.Location{X: x, Y: y, Z: z}); got != (location.Location{X: 50, Y: 0, Z: 0}) {
		t.Fatalf("Position() = %+v, want teleported cell", got)
	}
}

func TestReturnHomeForceWalkStanceBroadcast(t *testing.T) {
	movement := &hostileMove{}
	hostile := newTestHostile(t, movement, &hostileAttack{})
	rec := &event.Recorder{}
	w := world.New()
	w.Spawn(hostile, 100, 0, 0, 0)
	hostile.Attach(Runtime{World: w, Sink: rec})
	hostile.Instance.HasHome = true
	hostile.Instance.Home = location.Location{X: 100, Y: 0, Z: 0}
	hostile.Instance.Template.RunSpeed = 120
	hostile.Instance.Template.WalkSpeed = 60
	hostile.SetXYZ(100, 500, 0)

	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false, want true outside drift range")
	}
	if hostile.Running() {
		t.Fatal("Running() = true after ordinary ReturnHome, want walk stance")
	}
	if got, want := rec.Events(), []event.Event{event.MoveTypeChanged{Running: false}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
}

func TestRestoreSpawnHeadingIfAtHome(t *testing.T) {
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	w := world.New()
	w.Spawn(hostile, 100, 0, 0, 0)
	hostile.Attach(Runtime{World: w})
	hostile.Instance.HasHome = true
	hostile.Instance.Home = location.Location{X: 100, Y: 0, Z: 0}
	hostile.Instance.SpawnHeading = 40000
	hostile.SetHeading(1)

	hostile.RestoreSpawnHeadingIfAtHome()
	if got := hostile.Heading(); got != 40000 {
		t.Fatalf("Heading() at home = %d, want spawn heading 40000", got)
	}

	hostile.SetXYZ(200, 0, 0)
	hostile.SetHeading(1)
	hostile.RestoreSpawnHeadingIfAtHome()
	if got := hostile.Heading(); got != 1 {
		t.Fatalf("Heading() off home = %d, want unchanged 1", got)
	}
}

func TestReturnHomeRechecksWanderBehindActor(t *testing.T) {
	movement := &hostileMove{moved: make(chan location.Location, 1)}
	hostile := newTestHostile(t, movement, &hostileAttack{})
	hostile.Instance.HasHome = true
	hostile.Instance.Home = location.Location{X: 100, Y: 0, Z: 0}
	hostile.Instance.Template.WalkSpeed = 100
	hostile.Instance.Template.DEX = 30 // run-speed DEX bonus 1.1: walks at 110
	hostile.Instance.Template.CollisionRadius = 30
	hostile.roll = func(int) int { return 0 }
	world.New().Spawn(hostile, 100, 500, 0, 0)
	clock := driveHostile(hostile)
	hostile.SetHeading(0)
	hostile.AI().Desires().AddOrUpdate(&ai.Desire{Kind: ai.IntentionWander, Timer: 5, Weight: 5})
	if err := hostile.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if movement.home != (location.Location{X: 100, Y: 0, Z: 0}) {
		t.Fatalf("MoveHome destination = %#v, want spawn home", movement.home)
	}

	clock.Advance(1500 * time.Millisecond) // (int)((1500+roll) * (100/moveSpeed)) = 1363 ms
	select {
	case got := <-movement.moved:
		if want := (location.Location{X: 50, Y: 500, Z: 0}); got != want {
			t.Fatalf("wander recheck target = %#v, want %#v", got, want)
		}
	default:
		t.Fatal("wander recheck did not move behind the actor")
	}
}

// wanderGeo accepts every requested wander destination, unlike hostileGeo
// which reflects the origin.
type wanderGeo struct{ hostileGeo }

func (wanderGeo) ValidLocation(_, _, _, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz}
}

// A movement-disabled NPC keeps its WANDER intention but starts no walk:
// the wander destination is only followed when the actor can move. Fear is
// not a movement lock, so a feared NPC still walks.
func TestMoveFromSpawnUsingRandomOffsetHonorsMovementLock(t *testing.T) {
	cases := []struct {
		name     string
		disable  func(*testing.T, *Hostile)
		wantMove bool
	}{
		{"free", func(*testing.T, *Hostile) {}, true},
		{"fear", func(t *testing.T, h *Hostile) { addHostileEffect(t, h, "Fear") }, true},
		{"root", func(t *testing.T, h *Hostile) { addHostileEffect(t, h, "Root") }, false},
		{"sleep", func(t *testing.T, h *Hostile) { addHostileEffect(t, h, "Sleep") }, false},
		{"stun", func(t *testing.T, h *Hostile) { addHostileEffect(t, h, "Stun") }, false},
		{"paralyze", func(t *testing.T, h *Hostile) { addHostileEffect(t, h, "Paralyze") }, false},
		{"canMove false", func(_ *testing.T, h *Hostile) { h.Instance.Template.CanMove = false }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			movement := &hostileMove{}
			live, err := creature.NewLive(location.Location{}, 100, wanderGeo{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			live.SetQueue(idleQueue())
			hostile, err := NewHostile(&Instance{
				ObjectID: 101,
				Template: &Template{ID: 9001, Type: "Monster", CanMove: true},
				Kind:     "Monster",
				HasHome:  true,
			}, live, movement, &hostileAttack{})
			if err != nil {
				t.Fatal(err)
			}
			world.New().Spawn(hostile, 100, 500, 500, 0)
			tc.disable(t, hostile)

			hostile.MoveFromSpawnUsingRandomOffset(120)

			if got := len(movement.locations) > 0; got != tc.wantMove {
				t.Fatalf("wander move issued = %v (moves %v), want %v", got, movement.locations, tc.wantMove)
			}
		})
	}
}

func TestReturnHomeWanderRecheckSkippedWhileRooted(t *testing.T) {
	movement := &hostileMove{moved: make(chan location.Location, 1)}
	hostile := newTestHostile(t, movement, &hostileAttack{})
	hostile.Instance.HasHome = true
	hostile.Instance.Home = location.Location{X: 100, Y: 0, Z: 0}
	hostile.Instance.Template.WalkSpeed = 100
	hostile.Instance.Template.DEX = 30 // run-speed DEX bonus 1.1: walks at 110
	hostile.Instance.Template.CollisionRadius = 30
	hostile.roll = func(int) int { return 0 }
	world.New().Spawn(hostile, 100, 500, 0, 0)
	clock := driveHostile(hostile)
	hostile.SetHeading(0)
	hostile.AI().Desires().AddOrUpdate(&ai.Desire{Kind: ai.IntentionWander, Timer: 5, Weight: 5})
	if err := hostile.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	addHostileEffect(t, hostile, "Root")

	clock.Advance(1500 * time.Millisecond)
	select {
	case got := <-movement.moved:
		t.Fatalf("rooted NPC's wander recheck moved to %#v, want no movement", got)
	default:
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionWander {
		t.Fatalf("CurrentIntention() = %v, want WANDER kept", got)
	}
}

func TestGrandBossReturnHomeNeverWalksBack(t *testing.T) {
	// GrandBoss.returnHome is unconditionally false. A boss spawned
	// outside drift range must not MoveHome or take the Attackable
	// delayed backward nudge.
	movement := &hostileMove{moved: make(chan location.Location, 1)}
	hostile := newTestHostile(t, movement, &hostileAttack{})
	hostile.Instance.Kind = "GrandBoss"
	hostile.Instance.HasHome = true
	hostile.Instance.Home = location.Location{X: 100, Y: 0, Z: 0}
	hostile.Instance.Template.WalkSpeed = 100
	hostile.roll = func(int) int { return 0 }
	world.New().Spawn(hostile, 100, 500, 0, 0)
	clock := driveHostile(hostile)

	if hostile.ReturnHome() {
		t.Fatal("ReturnHome() = true, want false for GrandBoss")
	}
	if movement.home != (location.Location{}) {
		t.Fatalf("MoveHome destination = %#v, want no walk-back", movement.home)
	}
	clock.Advance(2 * time.Second)
	select {
	case <-movement.moved:
		t.Fatal("GrandBoss wander recheck moved behind the actor")
	default:
	}
}

func TestSiegeGuardReturnHomeDoesNotRecheckWander(t *testing.T) {
	movement := &hostileMove{moved: make(chan location.Location, 1)}
	hostile := newTestHostile(t, movement, &hostileAttack{})
	hostile.Instance.Kind = "SiegeGuard"
	hostile.Instance.HasHome = true
	hostile.Instance.Home = location.Location{X: 100, Y: 0, Z: 0}
	hostile.Instance.Template.RunSpeed = 100
	hostile.roll = func(int) int { return 0 }
	world.New().Spawn(hostile, 100, 500, 0, 0)
	clock := driveHostile(hostile)

	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false, want true outside drift range")
	}
	clock.Advance(2 * time.Second)
	select {
	case <-movement.moved:
		t.Fatal("SiegeGuard wander recheck moved behind the actor")
	default:
	}
}

func TestReturnHomeScalesWanderRecheckDelayForFastNPC(t *testing.T) {
	movement := &hostileMove{moved: make(chan location.Location, 1)}
	hostile := newTestHostile(t, movement, &hostileAttack{})
	hostile.Instance.HasHome = true
	hostile.Instance.Home = location.Location{X: 100, Y: 0, Z: 0}
	hostile.Instance.Template.WalkSpeed = 200
	hostile.Instance.Template.DEX = 30 // run-speed DEX bonus 1.1: walks at 220
	hostile.roll = func(int) int { return 0 }
	world.New().Spawn(hostile, 100, 500, 0, 0)
	clock := driveHostile(hostile)
	hostile.AI().Desires().AddOrUpdate(&ai.Desire{Kind: ai.IntentionWander, Timer: 5, Weight: 5})
	if err := hostile.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}

	// The delay reads the stat-finalized walk speed in float32:
	// (int)((1500+roll) * (100f/220f)) = 681 ms.
	clock.Advance(680 * time.Millisecond)
	select {
	case <-movement.moved:
		t.Fatal("wander recheck fired before the scaled delay")
	default:
	}
	clock.Advance(time.Millisecond)
	select {
	case <-movement.moved:
	default:
		t.Fatal("wander recheck did not fire after the scaled delay")
	}
}

func TestSiegeGuardReturnHomeForceRunStanceBroadcast(t *testing.T) {
	movement := &hostileMove{}
	hostile := newTestHostile(t, movement, &hostileAttack{})
	rec := &event.Recorder{}
	hostile.Instance.Kind = "SiegeGuard"
	w := world.New()
	w.Spawn(hostile, 100, 0, 0, 0)
	hostile.Attach(Runtime{World: w, Sink: rec})
	hostile.Instance.HasHome = true
	hostile.Instance.Home = location.Location{X: 100, Y: 0, Z: 0}
	hostile.Instance.Template.RunSpeed = 120
	hostile.Instance.Template.WalkSpeed = 60
	hostile.SetRunning(false)
	hostile.SetXYZ(100, 50, 0)

	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false, want true outside SiegeGuard drift range")
	}
	if !hostile.Running() {
		t.Fatal("Running() = false after SiegeGuard ReturnHome, want run stance")
	}
	if got, want := rec.Events(), []event.Event{event.MoveTypeChanged{Running: true}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
}
