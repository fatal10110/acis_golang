package character

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestWalkAcrossRegionsUpdatesKnownObjects pins that a player's own walk,
// step by step, moves it through the world grid: crossing into a region whose
// neighborhood holds an NPC shows the walker that NPC while it is still
// walking, and leaving a standing player's neighborhood removes the walker
// from that player's view.
func TestWalkAcrossRegionsUpdatesKnownObjects(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Walker", 1, 0), gameservertest.WithWantChars(1))
	if !srv.DrivesClock() {
		t.Skip("stepping a walk tick by tick needs the driven clock")
	}
	walker := srv.Client
	walkerID := srv.SoleObjectID(t)
	srv.SeedCharacterFor(t, "watcher", "Watcher", 1, 0)
	watcher := srv.DialClient(t, "watcher", 1)
	enterWorld(t, walker)
	enterWorld(t, watcher)

	// Regions are 2048 units wide and a player sees its region's 3x3
	// neighborhood: from X=10 the walker sees up to X=4095. The NPC's region
	// starts at X=6144, so the walker only sees it from X=4096 on, which is
	// also where the walker leaves the watcher's neighborhood.
	monster := srv.SpawnHostileNPCAt(t, location.Location{X: 6200, Y: 20, Z: 30})

	spawn := location.Location{X: 10, Y: 20, Z: 30}
	walker.Send(encodeMoveBackwardToLocation(location.Location{X: 9_000, Y: 20, Z: 30}, spawn, 1))
	if !readUntilObjectFrame(t, walker, serverpackets.OpcodeMoveToLocation, walkerID) {
		t.Fatal("walk not started")
	}
	mover := srv.PlayerMove(t, walkerID)
	for mover.Position().X < 4_096 {
		if !mover.Moving() {
			t.Fatalf("walk stopped at %v before crossing X=4096", mover.Position())
		}
		srv.TickPositions()
	}
	if !mover.Moving() {
		t.Fatal("walker arrived instead of crossing mid-walk")
	}

	if !readUntilObjectFrame(t, walker, serverpackets.OpcodeNPCInfo, monster.ObjectID()) {
		t.Fatalf("walker crossing into the NPC's neighborhood mid-walk got no NpcInfo for it")
	}
	if !readUntilObjectFrame(t, watcher, serverpackets.OpcodeDeleteObject, walkerID) {
		t.Fatalf("watcher got no DeleteObject for the walker leaving its neighborhood")
	}
}

// readUntilObjectFrame reads c's frames until one with opcode names objectID
// first, reporting false when the frames run out first.
func readUntilObjectFrame(t *testing.T, c *testsupport.ScriptedClient, opcode byte, objectID int32) bool {
	t.Helper()
	for {
		frame := c.ReadWithTimeout(time.Second)
		if frame == nil {
			return false
		}
		if frame[0] == opcode && wire.NewReader(frame[1:]).ReadInt32() == objectID {
			return true
		}
	}
}
