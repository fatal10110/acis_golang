package boat

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func encodeChangeWaitType(stand bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestChangeWaitType)
	w.WriteInt32(wire.BoolInt32(stand))
	return w.Bytes()
}

// TestPostureBusyClickAcrossBoatBoundaryRefused pins Player.onAction
// checking the boat boundary before any tryTo*: a player on the dock,
// standing up, that clicks its selected target aboard a boat is answered
// ActionFailed and queues nothing. The sit it queued earlier in the
// stand-up stays queued and runs when the stand-up ends; no follow does.
func TestPostureBusyClickAcrossBoatBoundaryRefused(t *testing.T) {
	t.Parallel()
	srv, c, me, b := bootPassenger(t, runeBoarding, 0)
	if !srv.DrivesClock() {
		t.Skip("holding a stand-up open needs the driven clock")
	}
	board(t, srv, c, b)
	other, otherID := enterSecondClient(t, srv, runeBoarding)
	srv.ReadQueued(t, c)

	other.Send(encodeAction(me, false))
	if !slices.Contains(testsupport.FrameOpcodes(srv.ReadQueued(t, other)), serverpackets.OpcodeMyTargetSelected) {
		t.Fatal("click on the passenger did not select it")
	}
	other.Send(encodeChangeWaitType(false))
	srv.ReadQueued(t, other)
	srv.SettlePosture(t, otherID)
	srv.ReadQueued(t, other)
	other.Send(encodeChangeWaitType(true))
	if !slices.Contains(testsupport.FrameOpcodes(srv.ReadQueued(t, other)), serverpackets.OpcodeChangeWaitType) {
		t.Fatal("stand request did not start the stand-up")
	}
	// A sit asked for mid stand-up is queued behind it.
	other.Send(encodeChangeWaitType(false))
	srv.ReadQueued(t, other)

	other.Send(encodeAction(me, false))
	if got := testsupport.FrameOpcodes(srv.ReadQueued(t, other)); string(got) != string([]byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("click across the boat boundary mid stand-up = %x, want ActionFailed alone", got)
	}

	srv.SettlePosture(t, otherID)
	got := testsupport.FrameOpcodes(srv.ReadQueued(t, other))
	if !slices.Contains(got, serverpackets.OpcodeChangeWaitType) {
		t.Fatalf("stand-up end = %x, want the queued sit to run", got)
	}
	if slices.Contains(got, serverpackets.OpcodeMoveToPawn) {
		t.Fatalf("stand-up end = %x, want no follow of the passenger", got)
	}
}
