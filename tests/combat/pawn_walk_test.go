package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// flatGeo is gameservertest.Geo on flat ground at the fixture height, so a
// walk stopping mid-leg keeps its Z.
type flatGeo struct{ gameservertest.Geo }

func (flatGeo) Height(int, int, int) int16 { return hostileZ }

// TestAttackApproachTracksMovedTarget pins the player's pawn walk
// (PlayerMove.updatePosition, PlayerMove.java:234 and :323): each position
// update re-aims the approach at where the target stands now, and the walk
// ends at the first step strictly within the attack range of it (2D), not
// on the cell the target stood on when the approach started.
func TestAttackApproachTracksMovedTarget(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithGeo(flatGeo{}),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 500, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertFrameOpcode(t, mustRead(t, c, "MoveToPawn"), serverpackets.OpcodeMoveToPawn, "approach")

	mover := srv.PlayerMove(t, objID)
	for range 3 {
		srv.TickPositions()
	}
	moved := location.Location{X: hostileX + 500, Y: hostileY + 300, Z: hostileZ}
	hostile.TeleportTo(moved)
	for i := 0; mover.Moving(); i++ {
		if i == 300 {
			t.Fatalf("approach still under way at %+v after 30 s of position updates", mover.Position())
		}
		srv.TickPositions()
	}

	reach := physicalAttackRange(t, srv, objID)
	pos := mover.Position()
	// One position update covers well under 30 units at the fixture's speed.
	if !pos.In2DRadius(moved, reach) || pos.In2DRadius(moved, reach-30) {
		t.Fatalf("approach ended at %+v, %.1f from the moved target; want the first step within the attack range %d",
			pos, pos.Distance2D(moved), reach)
	}
	drainUntilQuiet(t, c)
}

// rethinkApproach boots a level-5 player (seeded with seed before it enters
// the world), starts its attack on a stationary hostile 500 units away and
// lets the approach walk run three position updates. It returns the
// server, client, player id and hostile id, with the client drained.
func rethinkApproach(t *testing.T, seed func(srv *gameservertest.Server, objID int32)) (*gameservertest.Server, *scriptedClient, int32, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithGeo(flatGeo{}),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	if seed != nil {
		seed(srv, objID)
	}
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 500, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertFrameOpcode(t, mustRead(t, c, "MoveToPawn"), serverpackets.OpcodeMoveToPawn, "approach")
	for range 3 {
		srv.TickPositions()
	}
	drainUntilQuiet(t, c)
	if !srv.PlayerMove(t, objID).Moving() {
		t.Fatal("the approach walk is no longer under way")
	}
	return srv, c, objID, hostile.ObjectID()
}

// physicalAttackRange reads the online player's physical attack range.
func physicalAttackRange(t *testing.T, srv *gameservertest.Server, objID int32) int {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	player, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	return player.PhysicalAttackRange()
}

// pawnMoveFrom finds the first MoveToPawn at or after from that moves mover
// toward target at distance from origin, or -1.
func pawnMoveFrom(frames [][]byte, from int, mover, target int32, distance int, origin location.Location) int {
	for i := from; i < len(frames); i++ {
		if frames[i][0] != serverpackets.OpcodeMoveToPawn {
			continue
		}
		r := wireReader(frames[i][1:])
		if r.ReadInt32() != mover || r.ReadInt32() != target || r.ReadInt32() != int32(distance) {
			continue
		}
		if r.ReadInt32() == int32(origin.X) && r.ReadInt32() == int32(origin.Y) && r.ReadInt32() == int32(origin.Z) {
			return i
		}
	}
	return -1
}

// TestAttackClickMidApproachResendsMoveToPawn pins a repeated attack click
// on the target being chased (PlayableAI.tryToAttack -> doAttackIntention ->
// PlayerAI.thinkAttack, PlayerAI.java:170-197): PlayerMove.maybeMoveToPawn
// (PlayerMove.java:335-352) re-runs moveToPawn (PlayerMove.java:49-109),
// which broadcasts a fresh MoveToPawn from where the player stands now,
// although the target has not moved. Standing now includes the walk the
// time since the last position update covered (updatePosition(true),
// PlayerMove.java:60-61).
func TestAttackClickMidApproachResendsMoveToPawn(t *testing.T) {
	t.Parallel()
	srv, c, objID, hostileID := rethinkApproach(t, nil)
	before := srv.PlayerMove(t, objID).Position()

	c.Send(encodeAction(hostileID, int32(before.X), int32(before.Y), int32(before.Z), false))
	srv.Settle(t)
	here := srv.PlayerMove(t, objID).Position()
	if here.X <= before.X || here.Y != before.Y {
		t.Fatalf("re-think walk starts at %+v, want ahead of %+v on the approach line", here, before)
	}
	frames := readQuiet(c)
	if pawnMoveFrom(frames, 0, objID, hostileID, physicalAttackRange(t, srv, objID), here) < 0 {
		t.Fatalf("no fresh MoveToPawn from %+v after the repeated attack click: opcodes %v", here, opcodes(frames))
	}
}

// TestWeaponEquipMidApproachResendsMoveToPawn puts a sword on while the
// attack approach walks toward a stationary target (#2877):
// PlayerAI.thinkUseItem (PlayerAI.java:505-519) equips it, then re-runs the
// ATTACK, whose thinkAttack re-sends MoveToPawn from the current position
// at the sword's attack range, after S1_EQUIPPED.
func TestWeaponEquipMidApproachResendsMoveToPawn(t *testing.T) {
	t.Parallel()
	var sword int32
	srv, c, objID, hostileID := rethinkApproach(t, func(srv *gameservertest.Server, objID int32) {
		sword = srv.GiveItem(t, objID, 30, 1)
	})

	c.Send(encodeUseItem(sword, false))
	srv.Settle(t)
	here := srv.PlayerMove(t, objID).Position()
	frames := readQuiet(c)
	equipped := indexOfSystemMessage(frames, 0, serverpackets.SystemMessageS1Equipped)
	if equipped < 0 {
		t.Fatalf("no S1_EQUIPPED for the sword: opcodes %v", opcodes(frames))
	}
	if pawnMoveFrom(frames, equipped+1, objID, hostileID, physicalAttackRange(t, srv, objID), here) < 0 {
		t.Fatalf("no fresh MoveToPawn from %+v after S1_EQUIPPED: opcodes %v", here, opcodes(frames))
	}
}

// wallX is where pawnWallGeo's wall stands, between playerOrigin and a
// target 500 units east of the fixture spawn.
const wallX = 300

// pawnWallGeo is flat ground with a wall along X = wallX: a straight line
// crossing it is closed, and the pathfinder offers a detour around it.
type pawnWallGeo struct{ flatGeo }

// pawnWallDetour is the detour pawnWallGeo's pathfinder offers.
var pawnWallDetour = []location.Location{
	{X: playerOrigin.X, Y: hostileY + 400, Z: hostileZ},
	{X: hostileX + 500, Y: hostileY + 400, Z: hostileZ},
	{X: hostileX + 500, Y: hostileY, Z: hostileZ},
}

func (pawnWallGeo) CanMove(ox, _, _, tx, _, _ int) bool { return (ox < wallX) == (tx < wallX) }

func (pawnWallGeo) FindPath(_, _ location.Location) ([]location.Location, bool) {
	return append([]location.Location(nil), pawnWallDetour...), true
}

// TestAttackApproachStopsAtWall pins the player's attack approach against a
// closed line (#2965): PlayerAI.thinkAttack (PlayerAI.java:188) walks by
// PlayerMove.maybeMoveToPawn, whose moveToPawn (PlayerMove.java:49-108)
// clears the geo path and heads straight for the target, never
// path-finding; the first step into the wall ends the walk blocked
// (PlayerMove.java:285-289), and ARRIVED_BLOCKED on an ATTACK only sends the
// same-cell MoveToLocation correction (PlayerAI.java:95-96, CreatureAI.java:
// 72-75). Nothing walks the player on: the player has no follow task
// (offensiveFollowTask is only reached through maybeStartOffensiveFollow,
// which PlayerAI's thinkAttack/thinkCast never call).
func TestAttackApproachStopsAtWall(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithGeo(pawnWallGeo{}),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 500, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	approach := mustRead(t, c, "MoveToPawn")
	assertFrameOpcode(t, approach, serverpackets.OpcodeMoveToPawn, "approach")
	if i := pawnMoveFrom([][]byte{approach}, 0, objID, hostile.ObjectID(), physicalAttackRange(t, srv, objID), playerOrigin); i < 0 {
		t.Fatalf("approach is not the player's MoveToPawn toward the hostile from %+v", playerOrigin)
	}

	mover := srv.PlayerMove(t, objID)
	for i := 0; mover.Moving(); i++ {
		if i == 300 {
			t.Fatalf("approach still under way at %+v after 30 s of position updates", mover.Position())
		}
		srv.TickPositions()
	}
	stop := mover.Position()
	// One position update covers well under 30 units at the fixture's speed.
	if stop.X >= wallX || stop.X < wallX-30 || stop.Y != playerOrigin.Y {
		t.Fatalf("approach stopped at %+v, want on the straight line just short of the wall at X %d", stop, wallX)
	}
	for range 20 {
		srv.TickPositions()
	}
	if got := mover.Position(); got != stop || mover.Moving() {
		t.Fatalf("player at %+v (moving %v) after more position updates, want left at %+v", got, mover.Moving(), stop)
	}

	frames := readQuiet(c)
	corrections := 0
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeMoveToPawn:
			t.Fatalf("MoveToPawn sent again after the approach: opcodes %v", opcodes(frames))
		case serverpackets.OpcodeMoveToLocation:
			id, dest, origin := moveToLocationCoords(t, f)
			if id != objID {
				continue
			}
			if dest != stop || origin != stop {
				t.Fatalf("player MoveToLocation %+v -> %+v, want only the same-cell correction at %+v (no detour)", origin, dest, stop)
			}
			corrections++
		}
	}
	if corrections != 1 {
		t.Fatalf("blocked corrections = %d, want 1: opcodes %v", corrections, opcodes(frames))
	}
}
