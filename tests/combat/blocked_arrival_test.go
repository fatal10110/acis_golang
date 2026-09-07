package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestBlockedArrivalBroadcastsSameCellMoveToLocation pins CreatureAI's
// arrived-blocked correction: observers get MoveToLocation to the actor's
// current cell, not StopMove.
func TestBlockedArrivalBroadcastsSameCellMoveToLocation(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	geo := &gameservertest.GateGeo{}
	hostile := srv.SpawnMovingHostileNPCAtGeo(t, "SiegeGuard", home, home, geo)
	drainUntilQuiet(t, c)

	stranded := location.Location{X: hostileX, Y: hostileY + 50, Z: hostileZ}
	hostile.SetRunning(false)
	hostile.SetXYZ(stranded.X, stranded.Y, stranded.Z)
	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false, want true outside SiegeGuard drift range")
	}
	assertChangeMoveType(t, mustRead(t, c, "ChangeMoveType"), hostile.ObjectID(), true)
	if err := hostile.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	assertFrameOpcode(t, mustRead(t, c, "MoveToLocation"), serverpackets.OpcodeMoveToLocation, "start walk")

	for i := 0; i < 2; i++ {
		if _, moving := hostile.Move().UpdatePosition(move.PositionUpdateInterval); !moving {
			t.Fatalf("UpdatePosition() tick %d moving = false, want origin to leave start", i+1)
		}
	}
	advanced := hostile.Move().Position()
	if advanced == stranded {
		t.Fatal("Move().Position() still at walk start after interpolation ticks")
	}

	geo.Block()
	if _, moving := hostile.Move().UpdatePosition(move.PositionUpdateInterval); moving {
		t.Fatal("UpdatePosition() moving = true after path closed, want blocked stop")
	}

	frame := mustRead(t, c, "blocked-arrival correction")
	if frame[0] == serverpackets.OpcodeStopMove {
		t.Fatalf("blocked arrival opcode = StopMove (%#x), want MoveToLocation (%#x)", frame[0], serverpackets.OpcodeMoveToLocation)
	}
	objectID, dest, origin := moveToLocationCoords(t, frame)
	if objectID != hostile.ObjectID() {
		t.Fatalf("MoveToLocation object id = %d, want %d", objectID, hostile.ObjectID())
	}
	if dest != advanced || origin != advanced {
		t.Fatalf("MoveToLocation dest/origin = %+v/%+v, want advanced cell %+v (not start %+v)", dest, origin, advanced, stranded)
	}
}

// routedGateGeo is a straight-line wall until FindPath supplies a multi-leg
// route, then a closable gate so a suite can block the current leg while
// later waypoints remain.
type routedGateGeo struct {
	gameservertest.Geo
	blocked bool
	path    []location.Location
	routed  bool
}

func (g *routedGateGeo) Block() { g.blocked = true }

func (g *routedGateGeo) CanMove(int, int, int, int, int, int) bool {
	if g.blocked {
		return false
	}
	return g.routed
}

func (g *routedGateGeo) FindPath(_, _ location.Location) ([]location.Location, bool) {
	g.routed = true
	return g.path, true
}

func TestBlockedMidRouteBroadcastsNextLegMoveToLocation(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	// Past the 200-unit home sphere so a non-siege ReturnHome calls
	// MoveHome immediately (SiegeGuard queues MOVE_TO only when the
	// straight line is already open, which would skip this geopath).
	stranded := location.Location{X: hostileX, Y: hostileY + 250, Z: hostileZ}
	firstLeg := location.Location{X: hostileX + 80, Y: hostileY + 125, Z: hostileZ}
	geo := &routedGateGeo{path: []location.Location{firstLeg, home}}
	hostile := srv.SpawnMovingHostileNPCAtGeo(t, "Monster", home, home, geo)
	drainUntilQuiet(t, c)

	hostile.SetXYZ(stranded.X, stranded.Y, stranded.Z)
	if !hostile.ReturnHome() {
		t.Fatal("ReturnHome() = false, want true outside home drift range")
	}
	assertChangeMoveType(t, mustRead(t, c, "ChangeMoveType"), hostile.ObjectID(), false)
	startID, startDest, _ := moveToLocationCoords(t, mustRead(t, c, "start walk"))
	if startID != hostile.ObjectID() {
		t.Fatalf("start MoveToLocation object id = %d, want %d", startID, hostile.ObjectID())
	}
	if startDest != firstLeg {
		t.Fatalf("start MoveToLocation dest = %+v, want first geopath leg %+v", startDest, firstLeg)
	}

	for i := 0; i < 2; i++ {
		if _, moving := hostile.Move().UpdatePosition(move.PositionUpdateInterval); !moving {
			t.Fatalf("UpdatePosition() tick %d moving = false, want origin to leave start", i+1)
		}
	}
	blockedCell := hostile.Move().Position()
	if blockedCell == stranded {
		t.Fatal("Move().Position() still at walk start after interpolation ticks")
	}

	geo.Block()
	event, moving := hostile.Move().UpdatePosition(move.PositionUpdateInterval)
	if !moving {
		t.Fatal("UpdatePosition() moving = false after mid-route block, want next-leg walk")
	}
	if event.Destination != home {
		t.Fatalf("next-leg event dest = %+v, want remaining waypoint %+v", event.Destination, home)
	}

	frame := mustRead(t, c, "next-leg MoveToLocation")
	if frame[0] == serverpackets.OpcodeStopMove {
		t.Fatalf("mid-route block opcode = StopMove (%#x), want MoveToLocation (%#x)", frame[0], serverpackets.OpcodeMoveToLocation)
	}
	objectID, dest, origin := moveToLocationCoords(t, frame)
	if objectID != hostile.ObjectID() {
		t.Fatalf("MoveToLocation object id = %d, want %d", objectID, hostile.ObjectID())
	}
	if dest != home {
		t.Fatalf("MoveToLocation dest = %+v, want remaining waypoint %+v (not same-cell %+v)", dest, home, blockedCell)
	}
	if origin != blockedCell {
		t.Fatalf("MoveToLocation origin = %+v, want blocked cell %+v", origin, blockedCell)
	}
}
