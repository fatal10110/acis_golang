package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// distantHostileSpot is three regions east of the fixture NPC: outside the
// player's 3x3 region neighborhood, so an NPC jumping there leaves the
// player's known list.
var distantHostileSpot = location.Location{X: hostileX + 3*2048, Y: hostileY, Z: hostileZ}

// TestSelectedTargetLeavingKnownListClearsSelection pins the forget path of a
// selected monster: the player gets ActionFailed, its own TargetUnselected,
// then DeleteObject for the monster, and the server-side selection is gone.
func TestSelectedTargetLeavingKnownListClearsSelection(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)

	hostile.TeleportTo(distantHostileSpot)
	// The teleport's own broadcast comes first; the target clear precedes
	// the monster's DeleteObject.
	for frame := mustRead(t, c, "target clear ActionFailed"); frame[0] != serverpackets.OpcodeActionFailed; frame = mustRead(t, c, "target clear ActionFailed") {
		if frame[0] == serverpackets.OpcodeDeleteObject {
			t.Fatal("DeleteObject arrived before the target clear")
		}
	}
	unselected := mustRead(t, c, "self TargetUnselected")
	assertFrameOpcode(t, unselected, serverpackets.OpcodeTargetUnselected, "self TargetUnselected")
	r := wireReader(unselected[1:])
	if id, x, y, z := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != objID ||
		(location.Location{X: int(x), Y: int(y), Z: int(z)}) != playerOrigin {
		t.Fatalf("TargetUnselected = id %d at (%d,%d,%d), want id %d at %+v", id, x, y, z, objID, playerOrigin)
	}
	deleted := mustRead(t, c, "DeleteObject")
	assertFrameOpcode(t, deleted, serverpackets.OpcodeDeleteObject, "DeleteObject")
	if id := wireReader(deleted[1:]).ReadInt32(); id != hostile.ObjectID() {
		t.Fatalf("DeleteObject object id = %d, want %d", id, hostile.ObjectID())
	}
	if got := onlineTarget(t, srv, objID); got != nil {
		t.Fatalf("Target() = %d after the target left the known list, want none", got.ObjectID())
	}
	drainUntilQuiet(t, c)
}

// TestUnselectedObjectLeavingKnownListKeepsSelection is the negative half:
// another monster leaving the known list only deletes that monster.
func TestUnselectedObjectLeavingKnownListKeepsSelection(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	selected := srv.SpawnHostileNPC(t)
	other := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 40, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	targetHostile(t, c, selected.ObjectID())
	drainUntilQuiet(t, c)

	other.TeleportTo(distantHostileSpot)
	for {
		frame := mustRead(t, c, "DeleteObject")
		if frame[0] == serverpackets.OpcodeActionFailed || frame[0] == serverpackets.OpcodeTargetUnselected {
			t.Fatalf("unselected object's departure sent opcode %#x", frame[0])
		}
		if frame[0] == serverpackets.OpcodeDeleteObject {
			if id := wireReader(frame[1:]).ReadInt32(); id != other.ObjectID() {
				t.Fatalf("DeleteObject object id = %d, want %d", id, other.ObjectID())
			}
			break
		}
	}
	if got := onlineTarget(t, srv, objID); got == nil || got.ObjectID() != selected.ObjectID() {
		t.Fatalf("Target() = %v after another object left, want %d kept", got, selected.ObjectID())
	}
	drainUntilQuiet(t, c)
}

func onlineTarget(t *testing.T, srv *gameservertest.Server, objID int32) interface{ ObjectID() int32 } {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	ch, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	if target := ch.Target(); target != nil {
		return target
	}
	return nil
}
