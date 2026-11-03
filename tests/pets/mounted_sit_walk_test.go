package pets

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// riderPostureOpcodes keeps the opcodes a rider's sit request can answer
// with.
func riderPostureOpcodes(frames [][]byte) []byte {
	var got []byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeStopMove, serverpackets.OpcodeChangeWaitType, serverpackets.OpcodeActionFailed:
			got = append(got, f[0])
		}
	}
	return got
}

// TestRidingRiderSitRequestWaitsForArrival pins #4063 for a rider: a sit
// request during a ride waits as the next intention, so the ride goes on
// with no answer; on arrival the queued sit is refused with ActionFailed
// alone (the rider has already stopped, so no StopMove), and the rider stays
// mounted and standing.
func TestRidingRiderSitRequestWaitsForArrival(t *testing.T) {
	t.Parallel()
	h, watcher := mountedStriderRider(t)
	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	h.client.Send(encodeMoveBackwardToLocation(int32(x+300), int32(y), int32(z)))
	readUntilOpcode(t, h.client, serverpackets.OpcodeMoveToLocation, "ride MoveToLocation")
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, watcher.client)

	h.client.Send(encodeRequestChangeWaitType(false))
	if got := riderPostureOpcodes(drainFrames(t, h.client)); len(got) != 0 {
		t.Fatalf("sit during the ride answered % x, want nothing", got)
	}
	if !h.srv.PlayerMove(t, h.ownerID).Moving() {
		t.Fatal("the sit request stopped the ride")
	}

	h.srv.AdvanceUntil(t, "ride end", func() bool { return !h.srv.PlayerMove(t, h.ownerID).Moving() })
	if got, want := riderPostureOpcodes(drainFrames(t, h.client)), []byte{serverpackets.OpcodeActionFailed}; !slices.Equal(got, want) {
		t.Fatalf("arrival frames = % x, want % x", got, want)
	}
	requireRiderUnmoved(t, h, watcher, "arrival with a queued sit")
}
