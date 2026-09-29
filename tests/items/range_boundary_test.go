package items

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	// dropInteractionDistance is the drop distance gate: coordinates must lie
	// strictly inside it.
	dropInteractionDistance = 150
	// pickupDistance is the distance inside which a ground item click collects
	// without an approach walk.
	pickupDistance = 36
)

// TestDropAtExactInteractionDistanceRejected pins the drop gate's boundary:
// the radius is strict, so coordinates exactly dropInteractionDistance away
// are too far and one unit closer are accepted.
func TestDropAtExactInteractionDistanceRejected(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	adena := srv.GiveItem(t, objID, item.AdenaID, 100)
	startInWorld(t, c)

	c.Send(encodeRequestDropItem(adena, 40, spawnX+dropInteractionDistance, spawnY, spawnZ))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCannotDiscardDistanceTooFar)
	barrier(t, c)
	if inst := mustFindItem(t, srv, objID, adena); inst.Count != 100 {
		t.Fatalf("adena count after boundary drop rejection = %d, want 100", inst.Count)
	}

	c.Send(encodeRequestDropItem(adena, 40, spawnX+dropInteractionDistance-1, spawnY, spawnZ))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeDropItem, "DropItem one unit inside the drop radius")
}

// TestPickupAtExactPickupDistanceNeedsApproach pins the pickup range's
// boundary: an item exactly pickupDistance away is out of reach, so a
// shift-click (which never walks) only releases the pending action and the
// item stays on the ground; one unit closer collects immediately.
func TestPickupAtExactPickupDistanceNeedsApproach(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	adena := srv.GiveItem(t, objID, item.AdenaID, 100)
	startInWorld(t, c)

	dropAt := func(x int32) int32 {
		t.Helper()
		c.Send(encodeRequestDropItem(adena, 10, x, spawnY, spawnZ))
		frame := c.Read()
		assertFrameOpcode(t, frame, serverpackets.OpcodeDropItem, "DropItem")
		r := wire.NewReader(frame[1:])
		r.ReadInt32() // dropper id
		groundID := r.ReadInt32()
		drainUntilQuiet(t, c)
		return groundID
	}

	onBoundary := dropAt(spawnX + pickupDistance)
	c.Send(encodeAction(onBoundary, spawnX, spawnY, spawnZ, true))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "boundary pickup pending-action release")
	if frame := c.ReadWithTimeout(500 * time.Millisecond); frame != nil {
		t.Fatalf("boundary shift-click pickup sent opcode 0x%02x, want nothing after ActionFailed", frame[0])
	}
	if _, ok := srv.State.Object(onBoundary); !ok {
		t.Fatalf("ground item %d exactly %d units away was collected", onBoundary, pickupDistance)
	}

	inside := dropAt(spawnX + pickupDistance - 1)
	c.Send(encodeAction(inside, spawnX, spawnY, spawnZ, true))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "inside pickup pending-action release")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeGetItem, "GetItem one unit inside the pickup range")
}
