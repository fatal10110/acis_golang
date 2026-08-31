package character

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// spawnOrigin is the class template spawn both clients share, so a chair
// placed here is in known range and within chair-sit distance.
var spawnOrigin = location.Location{X: 10, Y: 20, Z: 30}

// TestLogoutDeleteObjectSeatedFlag pins Forget's DeleteObject animation
// flag: a throne-seated player's removal uses stand-then-delete (0), while
// a standing or ground-sitting player uses delete-outright (1).
func TestLogoutDeleteObjectSeatedFlag(t *testing.T) {
	t.Run("throne seated", func(t *testing.T) {
		srv, c, observer, objID := bootObserverPair(t)
		sitOnChair(t, c, observer, spawnChair(t, srv, c, observer))
		assertObserverDeleteObjectFlag(t, c, observer, objID, 0)
	})
	t.Run("ground sitting", func(t *testing.T) {
		_, c, observer, objID := bootObserverPair(t)
		sitOnGround(t, c, observer)
		assertObserverDeleteObjectFlag(t, c, observer, objID, 1)
	})
	t.Run("standing", func(t *testing.T) {
		_, c, observer, objID := bootObserverPair(t)
		assertObserverDeleteObjectFlag(t, c, observer, objID, 1)
	})
}

func bootObserverPair(t *testing.T) (*gameservertest.Server, *testsupport.ScriptedClient, *testsupport.ScriptedClient, int32) {
	t.Helper()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	enterWorld(t, c)
	drainQuiet(t, c)
	objID := srv.SoleObjectID(t)

	srv.SeedCharacterFor(t, "player2", "Second", 1, 0)
	observer := srv.DialClient(t, "player2", 1)
	enterWorld(t, observer)
	drainQuiet(t, observer)
	drainQuiet(t, c)
	return srv, c, observer, objID
}

func spawnChair(t *testing.T, srv *gameservertest.Server, c, observer *testsupport.ScriptedClient) *staticobject.Object {
	t.Helper()
	chair, err := staticobject.NewObject(srv.NewObjectID(), &staticobject.Template{
		ID:       24180017,
		Location: spawnOrigin,
		Type:     staticobject.ChairType,
	})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}
	srv.State.Spawn(chair, spawnOrigin.X, spawnOrigin.Y, spawnOrigin.Z, 0)
	mustReadOpcode(t, c, serverpackets.OpcodeStaticObjectInfo, "sitter StaticObjectInfo")
	mustReadOpcode(t, observer, serverpackets.OpcodeStaticObjectInfo, "observer StaticObjectInfo")
	drainQuiet(t, c)
	drainQuiet(t, observer)
	return chair
}

func sitOnChair(t *testing.T, c, observer *testsupport.ScriptedClient, chair *staticobject.Object) {
	t.Helper()
	c.Send(encodeAction(chair.ObjectID(), int32(spawnOrigin.X), int32(spawnOrigin.Y), int32(spawnOrigin.Z), false))
	mustReadOpcode(t, c, serverpackets.OpcodeMyTargetSelected, "chair MyTargetSelected")
	drainQuiet(t, c)
	drainQuiet(t, observer)

	c.Send(encodeRequestChangeWaitType(false))
	mustReadOpcode(t, c, serverpackets.OpcodeChangeWaitType, "sitter ChangeWaitType")
	mustReadOpcode(t, c, serverpackets.OpcodeChairSit, "sitter ChairSit")
	mustReadOpcode(t, observer, serverpackets.OpcodeChangeWaitType, "observer ChangeWaitType")
	mustReadOpcode(t, observer, serverpackets.OpcodeChairSit, "observer ChairSit")
	drainQuiet(t, c)
	drainQuiet(t, observer)
}

func sitOnGround(t *testing.T, c, observer *testsupport.ScriptedClient) {
	t.Helper()
	c.Send(encodeRequestChangeWaitType(false))
	mustReadOpcode(t, c, serverpackets.OpcodeChangeWaitType, "sitter ChangeWaitType")
	mustReadOpcode(t, observer, serverpackets.OpcodeChangeWaitType, "observer ChangeWaitType")
	drainQuiet(t, c)
	drainQuiet(t, observer)
}

func assertObserverDeleteObjectFlag(t *testing.T, c, observer *testsupport.ScriptedClient, objID int32, wantFlag int32) {
	t.Helper()
	c.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeLeaveWorld {
		t.Fatalf("logout opcode = %#x, want LeaveWorld (%#x)", reply[0], serverpackets.OpcodeLeaveWorld)
	}
	got := readDeleteObjectFlag(t, observer, objID)
	if got != wantFlag {
		t.Fatalf("DeleteObject seated flag = %d, want %d", got, wantFlag)
	}
}

func readDeleteObjectFlag(t *testing.T, c *testsupport.ScriptedClient, objectID int32) int32 {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		frame := c.ReadWithTimeout(500 * time.Millisecond)
		if frame == nil {
			continue
		}
		if frame[0] != serverpackets.OpcodeDeleteObject {
			continue
		}
		r := wire.NewReader(frame[1:])
		id := r.ReadInt32()
		flag := r.ReadInt32()
		if err := r.Err(); err != nil {
			t.Fatalf("read DeleteObject: %v", err)
		}
		if id == objectID {
			return flag
		}
	}
	t.Fatalf("no DeleteObject for object %d within 5s", objectID)
	return 0
}

func mustReadOpcode(t *testing.T, c *testsupport.ScriptedClient, opcode byte, what string) []byte {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		frame := c.ReadWithTimeout(500 * time.Millisecond)
		if frame == nil {
			continue
		}
		if frame[0] == opcode {
			return frame
		}
	}
	t.Fatalf("%s: no opcode %#x within 5s", what, opcode)
	return nil
}
