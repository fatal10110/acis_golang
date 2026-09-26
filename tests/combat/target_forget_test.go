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

// TestOwnRegionCrossingTargetClearReachesOldNeighborhood pins who sees the
// TargetUnselected when a player's own move carries it out of range of its
// selected monster. The reference assigns the mover's new region only after
// its forget and discover passes (WorldObject.setRegion), so the broadcast
// from the target clear resolves the pre-move 3x3 neighborhood: a watcher
// only in the old one receives it, a watcher only in the new one does not.
func TestOwnRegionCrossingTargetClearReachesOldNeighborhood(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	oldWatcher := placeWatcher(t, srv, "oldwatch", "OldWatch", location.Location{X: playerOrigin.X - 2048, Y: playerOrigin.Y, Z: playerOrigin.Z})
	newWatcher := placeWatcher(t, srv, "newwatch", "NewWatch", location.Location{X: playerOrigin.X + 3*2048, Y: playerOrigin.Y, Z: playerOrigin.Z})
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, oldWatcher)
	drainUntilQuiet(t, newWatcher)

	mover, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	// Two regions east: the monster's region and the old watcher's leave the
	// mover's neighborhood, the new watcher's enters it.
	if err := srv.State.Move(mover, playerOrigin.X+2*2048, playerOrigin.Y, playerOrigin.Z); err != nil {
		t.Fatalf("move: %v", err)
	}

	if !receivedTargetUnselected(t, c, objID) {
		t.Fatal("mover never received its own TargetUnselected")
	}
	if !receivedTargetUnselected(t, oldWatcher, objID) {
		t.Fatal("watcher in the old neighborhood never received TargetUnselected")
	}
	if receivedTargetUnselected(t, newWatcher, objID) {
		t.Fatal("watcher only in the new neighborhood received TargetUnselected")
	}
	if got := onlineTarget(t, srv, objID); got != nil {
		t.Fatalf("Target() = %d after the crossing, want none", got.ObjectID())
	}
}

// TestLogoutTargetClearReachesNeighborhoodBeforeDeleteObject pins the
// logout half: a player leaving the world with a monster selected clears it
// while still in its region (Player.cleanup's abortAll(true) runs before
// decayMe), so a watcher sharing its neighborhood receives the leaver's
// TargetUnselected, and before the leaver's DeleteObject.
func TestLogoutTargetClearReachesNeighborhoodBeforeDeleteObject(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	watcher := placeWatcher(t, srv, "watch", "Watch", location.Location{X: playerOrigin.X - 2048, Y: playerOrigin.Y, Z: playerOrigin.Z})
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, watcher)

	c.Send(encodeLogout())

	unselected := false
	for {
		frame := mustRead(t, watcher, "leaver's DeleteObject")
		if len(frame) < 5 || wireReader(frame[1:]).ReadInt32() != objID {
			continue
		}
		if frame[0] == serverpackets.OpcodeTargetUnselected {
			unselected = true
		}
		if frame[0] == serverpackets.OpcodeDeleteObject {
			break
		}
	}
	if !unselected {
		t.Fatal("watcher got the leaver's DeleteObject without its TargetUnselected first")
	}
}

// placeWatcher logs a player in on account and teleports it to at.
func placeWatcher(t *testing.T, srv *gameservertest.Server, account, name string, at location.Location) *scriptedClient {
	t.Helper()
	srv.SeedCharacterFor(t, account, name, 1, 0)
	w := srv.DialClient(t, account, 1)
	startInWorld(t, w)
	p, ok := srv.State.PlayerByName(name)
	if !ok {
		t.Fatalf("%s missing from world state", name)
	}
	if err := srv.State.Teleport(p, at.X, at.Y, at.Z); err != nil {
		t.Fatalf("teleport %s: %v", name, err)
	}
	drainUntilQuiet(t, w)
	return w
}

// receivedTargetUnselected drains c until quiet and reports whether a
// TargetUnselected for objID arrived.
func receivedTargetUnselected(t *testing.T, c *scriptedClient, objID int32) bool {
	t.Helper()
	got := false
	for range 100 {
		frame := c.ReadWithTimeout(readQuietWindow)
		if frame == nil {
			return got
		}
		if frame[0] == serverpackets.OpcodeTargetUnselected && wireReader(frame[1:]).ReadInt32() == objID {
			got = true
		}
	}
	t.Fatal("client kept receiving frames after 100 reads")
	return false
}
