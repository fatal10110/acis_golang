package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestSelectedThroneClickInteractsWithoutSitting pins StaticObject.onAction
// (StaticObject.java:37-44): a second click on a selected throne is an
// interact, which PlayerAI.thinkInteract (PlayerAI.java:413-461) releases
// with ActionFailed and faces it in range with MoveToPawn(actor, target,
// 150), and StaticObject.onInteract (StaticObject.java:24-35) answers
// nothing for a throne. The click never sits on or claims it; the
// sit request still does.
func TestSelectedThroneClickInteractsWithoutSitting(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	enterWorld(t, c)
	drainQuiet(t, c)
	chair := spawnChair(t, srv, c, nil)

	c.Send(encodeAction(chair.ObjectID(), int32(spawnOrigin.X), int32(spawnOrigin.Y), int32(spawnOrigin.Z), false))
	mustReadOpcode(t, c, serverpackets.OpcodeMyTargetSelected, "select throne")
	drainQuiet(t, c)
	frames := clickStatic(t, c, chair, false)
	want := []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn}
	if got := staticInteractOrder(frames); string(got) != string(want) {
		t.Fatalf("throne click answered %x, want ActionFailed then MoveToPawn", got)
	}
	assertMoveToPawn(t, frameWithOpcode(t, frames, serverpackets.OpcodeMoveToPawn), srv.SoleObjectID(t), chair.ObjectID(), 150)
	if chair.Busy() {
		t.Fatal("a throne click claimed the throne")
	}

	c.Send(encodeRequestChangeWaitType(false))
	mustReadOpcode(t, c, serverpackets.OpcodeChangeWaitType, "sit request")
	mustReadOpcode(t, c, serverpackets.OpcodeChairSit, "sit request ChairSit")
	if !chair.Busy() {
		t.Fatal("the sit request did not claim the throne")
	}
}
