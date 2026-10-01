package character

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestMovementUpdatesWorldState drives a walk over the real wire protocol
// and requires the resulting position to become observable in world state,
// not just in the client's own packet stream.
func TestMovementUpdatesWorldState(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)
	objID := srv.SoleObjectID(t)

	target := location.Location{X: 80, Y: 70, Z: 30}
	spawn := location.Location{X: 10, Y: 20, Z: 30}
	c.Send(encodeMoveBackwardToLocation(target, spawn, 1))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation (%#x)", reply[0], serverpackets.OpcodeMoveToLocation)
	}
	waitForWorldPosition(t, srv, objID, target)
}

// TestSwimmingPlayerRisesPastWaterSurface pins PlayerMove.updatePosition
// (PlayerMove.java:224-257): the water surface caps a player's height only
// while it flies (or rides a boat), so a swimmer over deep water climbs past
// the surface to the height it asked for.
func TestSwimmingPlayerRisesPastWaterSurface(t *testing.T) {
	t.Parallel()
	form, err := zone.NewCuboid(-1_000, 1_000, -1_000, 1_000, -1_000, 150)
	if err != nil {
		t.Fatal(err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewWater(1, form))
	if _, ok := zone.FindAt[*zone.Water](zones, 10, 20, 30); !ok {
		t.Fatal("water zone missing at player spawn")
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(zones),
	)
	c := srv.Client

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	// Spawning in the water adds its entry UserInfo to the login burst.
	drainQuiet(t, c)
	objID := srv.SoleObjectID(t)
	target := location.Location{X: 300, Y: 200, Z: 200}
	c.Send(encodeMoveBackwardToLocation(target, target, 1))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation (%#x)", reply[0], serverpackets.OpcodeMoveToLocation)
	}
	waitForWorldPosition(t, srv, objID, target)
}

// TestMoveBackwardToLocationRejectsBeyond9900Units pins
// MoveBackwardToLocation.java:109-114: a target farther than 9900 units
// from the packet's own origin is rejected with ActionFailed instead of
// starting a walk, regardless of how far the server-authoritative position
// actually is from either coordinate.
func TestMoveBackwardToLocationRejectsBeyond9900Units(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)

	origin := location.Location{X: 0, Y: 0, Z: 0}
	target := location.Location{X: 10000, Y: 0, Z: 0} // 10000 > 9900 cap
	c.Send(encodeMoveBackwardToLocation(target, origin, 1))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("opcode = %#x, want ActionFailed (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}
}

// TestBlockedWalkBroadcastsSameCellMoveToLocation pins the player MOVE_TO
// blocked-arrival branch: observers get MoveToLocation to the cell the
// walk actually stopped on, not StopMove.
func TestBlockedWalkBroadcastsSameCellMoveToLocation(t *testing.T) {
	t.Parallel()
	geo := &gameservertest.GateGeo{}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithGeo(geo),
	)
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	c.Read()
	c.Read()
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)
	objID := srv.SoleObjectID(t)

	spawn := location.Location{X: 10, Y: 20, Z: 30}
	target := location.Location{X: 80, Y: 20, Z: 30}
	c.Send(encodeMoveBackwardToLocation(target, spawn, 1))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation (%#x)", reply[0], serverpackets.OpcodeMoveToLocation)
	}

	advanced := srv.TickPlayerBlocked(t, objID, geo)
	frame := c.Read()
	if frame[0] == serverpackets.OpcodeStopMove {
		t.Fatalf("blocked arrival opcode = StopMove (%#x), want MoveToLocation (%#x)", frame[0], serverpackets.OpcodeMoveToLocation)
	}
	if frame[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("blocked arrival opcode = %#x, want MoveToLocation (%#x)", frame[0], serverpackets.OpcodeMoveToLocation)
	}
	objectID, dest, origin := gameservertest.ReadMoveToLocationCoords(t, frame)
	if objectID != objID {
		t.Fatalf("MoveToLocation object id = %d, want %d", objectID, objID)
	}
	if dest != advanced || origin != advanced {
		t.Fatalf("MoveToLocation dest/origin = %+v/%+v, want advanced cell %+v", dest, origin, advanced)
	}
}

// TestRunStartsAtWalkSpeed pins PlayerMove.updatePosition's start phase
// (PlayerMove.java:228,246 over PlayerStatus.getRealMoveSpeed,
// PlayerStatus.java:958-982): a running player's first five position
// updates of a move advance at its walk speed, the following ones at its run
// speed.
func TestRunStartsAtWalkSpeed(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)
	objID := srv.SoleObjectID(t)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	character, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	if !character.Running() || character.WalkSpeed() >= character.RunSpeed() {
		t.Fatalf("fixture running %v at walk %v / run %v, want a running player walking slower", character.Running(), character.WalkSpeed(), character.RunSpeed())
	}

	spawn := location.Location{X: 10, Y: 20, Z: 30}
	c.Send(encodeMoveBackwardToLocation(location.Location{X: 3_000, Y: 20, Z: 30}, spawn, 1))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation (%#x)", reply[0], serverpackets.OpcodeMoveToLocation)
	}
	mover := srv.PlayerMove(t, objID)
	accurate := float64(spawn.X)
	for update := 1; update <= 8; update++ {
		srv.TickPositions()
		speed := character.RunSpeed()
		if update <= 5 {
			speed = character.WalkSpeed()
		}
		accurate += speed / 10
		// The player's cell is the accurate position rounded half up
		// (PlayerMove.updatePosition's Math.round, PlayerMove.java:272).
		// Each update walks the time since the last one: exactly the tick
		// interval on the driven clock. On the real pool the updates since
		// the walk started cover at least as many intervals of wall time,
		// but each walks only that time's whole milliseconds, so the walk
		// phase gets at least walkPhaseFloor; how much of the wall-clock
		// time falls in each phase is not pinned there.
		got, want := mover.Position().X, int(math.Floor(accurate+0.5))
		if !srv.DrivesClock() {
			if floor := walkPhaseFloor(spawn.X, character.WalkSpeed(), update); update <= 5 && got < floor {
				t.Fatalf("X after update %d = %d, want at least %d (walk %v)", update, got, floor, character.WalkSpeed())
			}
			continue
		}
		if got != want {
			t.Fatalf("X after update %d = %d, want %d (walk %v, run %v)", update, got, want, character.WalkSpeed(), character.RunSpeed())
		}
	}
}

// walkPhaseFloor is the least X a player walking +X from x at walk speed
// reaches after its first updates position updates, when those span at
// least as many tick intervals of wall time. Each update walks the whole
// milliseconds since the last one (PlayerMove.updatePosition's
// Duration.toMillis, PlayerMove.java:215-222), dropping up to a millisecond
// each, so the updates walk more than a millisecond less per interval in
// all; the cell is that position rounded half up.
func walkPhaseFloor(x int, walk float64, updates int) int {
	walked := time.Duration(updates) * (move.PositionUpdateInterval - time.Millisecond)
	return int(math.Floor(float64(x) + walk*walked.Seconds() + 0.5))
}

// TestRunStartWalkFloorAllowsMillisecondTruncation pins walkPhaseFloor
// against production stepping: five walk-phase updates spanning just over
// five intervals of uneven length, as the real pool's scheduling lands them,
// walk only 498 whole milliseconds (PlayerMove.java:215-222), short of the
// nominal five intervals' cell but not of walkPhaseFloor.
func TestRunStartWalkFloorAllowsMillisecondTruncation(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	if !srv.DrivesClock() {
		t.Skip("landing position updates at chosen instants needs the driven clock")
	}
	c := srv.Client

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c)
	objID := srv.SoleObjectID(t)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	character, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	walk := character.WalkSpeed()

	spawn := location.Location{X: 10, Y: 20, Z: 30}
	c.Send(encodeMoveBackwardToLocation(location.Location{X: 3_000, Y: 20, Z: 30}, spawn, 1))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("walk opcode = %#x, want MoveToLocation (%#x)", reply[0], serverpackets.OpcodeMoveToLocation)
	}
	mover := srv.PlayerMove(t, objID)
	// 500.2 ms in all, but 100+99+100+99+100 = 498 whole milliseconds.
	gaps := []time.Duration{100_500 * time.Microsecond, 99_600 * time.Microsecond, 100_300 * time.Microsecond, 99_800 * time.Microsecond, 100 * time.Millisecond}
	var span time.Duration
	for _, gap := range gaps {
		srv.TickPositionsAfter(t, gap)
		span += gap
	}
	if span < time.Duration(len(gaps))*move.PositionUpdateInterval {
		t.Fatalf("gaps span %v, want at least %d intervals", span, len(gaps))
	}
	got := mover.Position().X
	if want := int(math.Floor(float64(spawn.X) + walk*0.498 + 0.5)); got != want {
		t.Fatalf("X after %d uneven updates = %d, want %d (498 ms at walk %v)", len(gaps), got, want, walk)
	}
	nominal := int(math.Floor(float64(spawn.X) + walk*0.5 + 0.5))
	if got >= nominal {
		t.Fatalf("X after %d uneven updates = %d reaches the nominal cell %d at walk %v; pick gaps whose truncation crosses a cell", len(gaps), got, nominal, walk)
	}
	if floor := walkPhaseFloor(spawn.X, walk, len(gaps)); got < floor {
		t.Fatalf("X after %d uneven updates = %d, below walkPhaseFloor %d (walk %v)", len(gaps), got, floor, walk)
	}
}
