package items

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// itemCastApproach boots a player holding three unlockable keys with a
// monster 900 units east selected, forces a key onto it (Ctrl), and reads the
// MoveToPawn of the approach. A non-nil geo replaces the walk's geodata.
func itemCastApproach(t *testing.T, geo *gameservertest.GateGeo) (srv *gameservertest.Server, c *testsupport.ScriptedClient, objID, key, hostileID int32, origin location.Location) {
	t.Helper()
	opts := []gameservertest.Option{
		gameservertest.WithSkills(ctrlKeySkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	}
	if geo != nil {
		opts = append(opts, gameservertest.WithGeo(geo))
	}
	srv = gameservertest.Boot(t, opts...)
	c, objID = srv.Client, srv.SoleObjectID(t)
	key = srv.GiveItem(t, objID, gameservertest.UnlockableKeyID, 3)
	startInWorld(t, c)
	x, y, z := srv.PlayerPosition(t, objID)
	origin = location.Location{X: x, Y: y, Z: z}
	at := location.Location{X: x + 900, Y: y, Z: z}
	hostile := srv.SpawnHostileNPCAt(t, at)
	drainUntilQuiet(t, c)
	c.Send(encodeAction(hostile.ObjectID(), int32(at.X), int32(at.Y), int32(at.Z), false))
	drainUntilQuiet(t, c)

	c.Send(encodeUseItem(key, true))
	walk := c.Read()
	assertFrameOpcode(t, walk, serverpackets.OpcodeMoveToPawn, "item cast approach MoveToPawn")
	r := wire.NewReader(walk[1:])
	if obj, target, distance := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); obj != objID || target != hostile.ObjectID() || distance != 400 {
		t.Fatalf("MoveToPawn = object %d target %d distance %d, want %d/%d/400", obj, target, distance, objID, hostile.ObjectID())
	}
	assertItemCount(t, srv, objID, key, 3)
	return srv, c, objID, key, hostile.ObjectID(), origin
}

// readFramesFor advances the clock by d in 100ms steps and returns every
// frame the client received meanwhile.
func readFramesFor(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, d time.Duration) [][]byte {
	t.Helper()
	var frames [][]byte
	for passed := time.Duration(0); passed < d; passed += 100 * time.Millisecond {
		srv.Advance(t, 100*time.Millisecond)
		for frame := c.ReadWithTimeout(20 * time.Millisecond); frame != nil; frame = c.ReadWithTimeout(20 * time.Millisecond) {
			frames = append(frames, frame)
		}
	}
	return frames
}

// TestUseItemSkillOutOfRangeWalksThenCasts pins an item's CAST intention
// taking the same approach as a skill-bar cast (ItemSkills.java:80 into
// PlayerAI.thinkCast, PlayerAI.java:259-271): forced onto a monster beyond
// the skill's 400 cast range, the key walks to it with MoveToPawn, still
// unspent, and its arrival casts it and spends the key.
func TestUseItemSkillOutOfRangeWalksThenCasts(t *testing.T) {
	t.Parallel()
	srv, c, objID, key, hostileID, _ := itemCastApproach(t, nil)

	var cast []byte
	for passed := time.Duration(0); cast == nil && passed < 20*time.Second; passed += 100 * time.Millisecond {
		srv.Advance(t, 100*time.Millisecond)
		for frame := c.ReadWithTimeout(20 * time.Millisecond); frame != nil; frame = c.ReadWithTimeout(20 * time.Millisecond) {
			if frame[0] == serverpackets.OpcodeMagicSkillUse {
				cast = frame
				break
			}
		}
	}
	if cast == nil {
		t.Fatal("no MagicSkillUse once the item cast approach arrived")
	}
	if caster, target, skillID, _, _, _ := decodeMagicSkillUse(cast); caster != objID || target != hostileID || skillID != 2236 {
		t.Fatalf("MagicSkillUse = caster %d target %d skill %d, want %d/%d/2236", caster, target, skillID, objID, hostileID)
	}
	drainUntilQuiet(t, c)
	assertItemCount(t, srv, objID, key, 2)
}

// TestThinkMidItemCastApproachWalksOnAfresh pins a THINK on an item cast's
// approach (#2925) running PlayerAI.thinkCast (PlayerAI.java:219-) as the
// arrival does: a fresh MoveToPawn to the monster at the cast range, no
// ActionFailed, the key still unspent; the arrival casts it and spends one
// key.
func TestThinkMidItemCastApproachWalksOnAfresh(t *testing.T) {
	t.Parallel()
	srv, c, objID, key, hostileID, _ := itemCastApproach(t, nil)
	srv.Advance(t, 500*time.Millisecond)
	drainUntilQuiet(t, c)

	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.WakeAI() })
	srv.Settle(t)
	walk := c.Read()
	assertFrameOpcode(t, walk, serverpackets.OpcodeMoveToPawn, "fresh item cast approach MoveToPawn")
	r := wire.NewReader(walk[1:])
	if obj, target, distance := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); obj != objID || target != hostileID || distance != 400 {
		t.Fatalf("fresh MoveToPawn = object %d target %d distance %d, want %d/%d/400", obj, target, distance, objID, hostileID)
	}
	assertItemCount(t, srv, objID, key, 3)

	casts := 0
	for _, frame := range readFramesFor(t, srv, c, 20*time.Second) {
		switch frame[0] {
		case serverpackets.OpcodeActionFailed:
			t.Fatal("ActionFailed after the THINK on the item cast approach")
		case serverpackets.OpcodeMagicSkillUse:
			casts++
		}
	}
	if casts != 1 {
		t.Fatalf("MagicSkillUse frames = %d, want 1: the arrival casts the key once", casts)
	}
	assertItemCount(t, srv, objID, key, 2)
}

// TestBlockedItemCastApproachStopsCasting pins the CAST arm of
// PlayerAI.onEvtArrivedBlocked (PlayerAI.java:90-94) for an item cast: the
// key's approach walk blocked by geodata answers
// DIST_TOO_FAR_CASTING_STOPPED, and the dropped cast never fires or spends
// the key.
func TestBlockedItemCastApproachStopsCasting(t *testing.T) {
	t.Parallel()
	geo := &gameservertest.GateGeo{}
	srv, c, objID, key, _, _ := itemCastApproach(t, geo)

	srv.TickPlayerBlocked(t, objID, geo)
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageDistTooFarCastingStopped)
	for _, frame := range readFramesFor(t, srv, c, 20*time.Second) {
		if frame[0] == serverpackets.OpcodeMagicSkillUse || frame[0] == serverpackets.OpcodeMoveToPawn {
			t.Fatalf("frame %#x after the blocked item cast approach, want no cast and no new approach", frame[0])
		}
	}
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("casting after the blocked item cast approach")
	}
	assertItemCount(t, srv, objID, key, 3)
}

// TestMoveDropsItemCastApproach pins a click-to-move replacing the item
// cast's approach (AbstractAI.doMoveToIntention replacing the CAST
// intention): the new walk's arrival neither walks back to the monster nor
// casts, and the key stays unspent.
func TestMoveDropsItemCastApproach(t *testing.T) {
	t.Parallel()
	srv, c, objID, key, _, origin := itemCastApproach(t, nil)

	c.Send(encodeMoveBackwardToLocation(int32(origin.X), int32(origin.Y+100), int32(origin.Z), int32(origin.X), int32(origin.Y), int32(origin.Z)))
	for _, frame := range readFramesFor(t, srv, c, 20*time.Second) {
		if frame[0] == serverpackets.OpcodeMagicSkillUse || frame[0] == serverpackets.OpcodeMoveToPawn {
			t.Fatalf("frame %#x after a move replaced the item cast approach, want no cast and no new approach", frame[0])
		}
	}
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("casting after a move replaced the item cast approach")
	}
	assertItemCount(t, srv, objID, key, 3)
}
