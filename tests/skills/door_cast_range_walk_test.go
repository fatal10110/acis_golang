package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// A cast on a door takes the same approach as one on a creature
// (PlayerAI.thinkCast PlayerAI.java:259-271 calls PlayerMove.maybeMoveToPawn
// for every final target, and Door extends Creature): in range is within
// cast range plus the player's radius plus the door's, which DoorData fixes
// at 16 for every door (DoorData.java:160).
const (
	doorWalkRange = 300
	doorWalkX     = 800
	doorWalkMP    = 5
)

// doorWalkSpawn is where the booted player stands.
var doorWalkSpawn = location.Location{X: 10, Y: 20, Z: 30}

// doorWalkCaster boots a player who knows a guaranteed UNLOCK_SPECIAL with a
// 300 cast range, with an unlockable door dx units east of it, selected.
func doorWalkCaster(t *testing.T, dx int) (srv *gameservertest.Server, c *testsupport.ScriptedClient, objID int32, def modelskill.Definition, origin, at location.Location, doorObjID int32) {
	t.Helper()
	def = unlockSkill(2235, "UNLOCK_SPECIAL", unlockSkillLevel, 100)
	def.CastRange = doorWalkRange
	def.MPConsume = doorWalkMP
	tmpl := unlockDoorTemplate(false)
	tmpl.Position = location.Location{X: doorWalkSpawn.X + dx, Y: doorWalkSpawn.Y, Z: doorWalkSpawn.Z}
	for i := range tmpl.Coordinates {
		tmpl.Coordinates[i].X += tmpl.Position.X - unlockDoorX
		tmpl.Coordinates[i].Y += tmpl.Position.Y
	}
	// The approach walk keeps to the spawn's floor.
	srv, objID = bootUnlockOpts(t, []gameservertest.Option{gameservertest.WithGeo(gameservertest.FlatGeo{Z: doorWalkSpawn.Z})}, def, tmpl)
	c = srv.Client
	gate, ok := srv.WorldObjects.Door(unlockDoorID)
	if !ok {
		t.Fatal("door not spawned")
	}
	x, y, z := srv.PlayerPosition(t, objID)
	origin = location.Location{X: x, Y: y, Z: z}
	if origin != doorWalkSpawn {
		t.Fatalf("player spawned at %+v, want %+v", origin, doorWalkSpawn)
	}
	at = tmpl.Position
	selectTarget(t, c, gate.ObjectID())
	return srv, c, objID, def, origin, at, gate.ObjectID()
}

// TestDoorCastOutOfRangeWalksThenCasts pins the door approach: an
// UNLOCKABLE skill on a door beyond cast range plus both footprints walks to
// it with MoveToPawn at the cast range, paying nothing, and the arrival
// thinks the CAST again, which casts on the door from there.
func TestDoorCastOutOfRangeWalksThenCasts(t *testing.T) {
	t.Parallel()
	srv, c, objID, def, origin, at, doorObjID := doorWalkCaster(t, doorWalkX)
	if location.In3DRadius(origin.X, origin.Y, origin.Z, at.X, at.Y, at.Z, doorWalkRange+100) {
		t.Fatalf("player at %+v already within reach of the door at %+v", origin, at)
	}
	mpBefore := srv.PlayerCurrentMP(t, objID)

	c.Send(encodeRequestMagicSkillUse(int32(def.ID), false, false))
	assertMoveToPawn(t, c.Read(), objID, doorObjID, doorWalkRange, origin)
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("casting before the approach walk arrived")
	}
	if got := srv.PlayerCurrentMP(t, objID); got != mpBefore {
		t.Fatalf("MP after the approach started = %d, want %d untouched", got, mpBefore)
	}

	srv.AdvanceUntil(t, "cast on arrival", func() bool { return srv.PlayerCastingNow(t, objID) })
	log := readFrameLog(c)
	cast := log.index(isMagicSkillUseOf(objID, int32(def.ID)))
	if cast < 0 {
		t.Fatal("no MagicSkillUse once the approach walk arrived")
	}
	r := wireReader(log[cast][1:])
	if _, target := r.ReadInt32(), r.ReadInt32(); target != doorObjID {
		t.Fatalf("MagicSkillUse target = %d, want the door %d", target, doorObjID)
	}
	x, y, z := srv.PlayerPosition(t, objID)
	if !location.In3DRadius(x, y, z, at.X, at.Y, at.Z, doorWalkRange+100) {
		t.Fatalf("cast started at (%d,%d,%d), want within cast range of the door at %+v", x, y, z, at)
	}
}

// TestShiftDoorCastOutOfRangeRefusedTooFar pins the shift-held door cast out
// of range: TARGET_TOO_FAR, no MoveToPawn, no MagicSkillUse, no MP spent,
// and the player stays where it stood.
func TestShiftDoorCastOutOfRangeRefusedTooFar(t *testing.T) {
	t.Parallel()
	srv, c, objID, def, origin, _, _ := doorWalkCaster(t, doorWalkX)
	mpBefore := srv.PlayerCurrentMP(t, objID)

	c.Send(encodeRequestMagicSkillUse(int32(def.ID), false, true))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageTargetTooFar)
	srv.Advance(t, 2*time.Second)
	for _, frame := range readFrameLog(c) {
		if frame[0] == serverpackets.OpcodeMoveToPawn || frame[0] == serverpackets.OpcodeMoveToLocation || frame[0] == serverpackets.OpcodeMagicSkillUse {
			t.Fatalf("frame %#x after the shift-held refusal, want no walk and no cast", frame[0])
		}
	}
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("casting after TARGET_TOO_FAR")
	}
	if got := srv.PlayerCurrentMP(t, objID); got != mpBefore {
		t.Fatalf("MP after TARGET_TOO_FAR = %d, want %d untouched", got, mpBefore)
	}
	if x, y, z := srv.PlayerPosition(t, objID); (location.Location{X: x, Y: y, Z: z}) != origin {
		t.Fatalf("player at (%d,%d,%d) after TARGET_TOO_FAR, want still at %+v", x, y, z, origin)
	}
}

// TestDoorCastRangeCountsDoorRadius pins the door's fixed 16 radius in the
// range check: a door just inside cast range plus the player's radius plus
// 16, but beyond cast range plus the player's radius alone, casts at once
// with no approach, shift held or not.
func TestDoorCastRangeCountsDoorRadius(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		shift bool
	}{{"plain", false}, {"shift", true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// The door sits 8 units past cast range plus the player's radius,
			// so it is in reach only when the door's own radius counts.
			srv, c, objID, def, origin, at, doorObjID := doorWalkCaster(t, doorWalkRange+8)
			var playerRadius float64
			onPlayerQueue(t, srv, objID, func(pc *player.Character) { playerRadius = pc.CollisionRadius() })
			reach := float64(doorWalkRange) + playerRadius
			if dx := float64(at.X - origin.X); dx <= reach || dx > reach+door.CollisionRadius {
				t.Fatalf("door %v units away, want within (%v, %v]", dx, reach, reach+door.CollisionRadius)
			}
			c.Send(encodeRequestMagicSkillUse(int32(def.ID), false, tc.shift))
			readCastStartFrames(t, c, objID, int32(def.ID), int32(def.Level), unlockHitTime, unlockReuse, doorObjID)
			if !srv.PlayerCastingNow(t, objID) {
				t.Fatal("not casting after an in-range request")
			}
		})
	}
}
