package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestSelectedThroneClickInteractsWithoutSitting pins StaticObject.onAction
// (StaticObject.java:37-44): a second click on a selected throne is an
// interact, which PlayerAI.thinkInteract (PlayerAI.java:413-461) releases
// with ActionFailed, and StaticObject.onInteract (StaticObject.java:24-35)
// answers nothing for a throne. The click never sits on or claims it; the
// sit request still does.
func TestSelectedThroneClickInteractsWithoutSitting(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	enterWorld(t, c)
	drainQuiet(t, c)
	chair := spawnChair(t, srv, c, nil)

	c.Send(encodeAction(chair.ObjectID(), int32(spawnOrigin.X), int32(spawnOrigin.Y), int32(spawnOrigin.Z), false))
	mustReadOpcode(t, c, serverpackets.OpcodeMyTargetSelected, "select throne")
	drainQuiet(t, c)
	c.Send(encodeAction(chair.ObjectID(), int32(spawnOrigin.X), int32(spawnOrigin.Y), int32(spawnOrigin.Z), false))
	var opcodes []byte
	for _, frame := range testsupport.SyncBarrierFrames(t, c, func() {
		c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	}, serverpackets.OpcodeItemList) {
		opcodes = append(opcodes, frame[0])
	}
	if len(opcodes) != 1 || opcodes[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("throne click answered opcodes %x, want ActionFailed alone", opcodes)
	}
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
