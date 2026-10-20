package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// mountedStriderRider boots a strider owner, mounts it, and brings a watcher
// in sight of the rider, returning both quiet.
func mountedStriderRider(t *testing.T) (*petWorld, secondPlayer) {
	t.Helper()
	h, _ := bootStriderOwner(t, striderRow5, striderRow5.maxMeal)
	h.mountStrider(t, encodeUserCommand(cmdMount))
	watcher := h.joinSecondPlayer(t, "Watcher")
	if !h.character(t).Mounted() {
		t.Fatal("owner not mounted after /mount")
	}
	return h, watcher
}

// requireRiderUnmoved checks the rider is still mounted and standing and
// that the watcher saw no posture change nor stop.
func requireRiderUnmoved(t *testing.T, h *petWorld, watcher secondPlayer, what string) {
	t.Helper()
	rider := h.character(t)
	if !rider.Mounted() || !rider.Standing() {
		t.Fatalf("%s: rider mounted=%v standing=%v, want mounted and standing", what, rider.Mounted(), rider.Standing())
	}
	for _, f := range drainFrames(t, watcher.client) {
		if f[0] == serverpackets.OpcodeChangeWaitType || f[0] == serverpackets.OpcodeStopMove {
			t.Fatalf("%s: watcher got opcode %#x", what, f[0])
		}
	}
}

// TestMountedRiderSitRequestRefused pins #4035: the sit key and the
// action-bar sit/stand button from a standing rider are answered with
// ActionFailed alone; the rider stays mounted and standing and no one sees
// a ChangeWaitType or StopMove.
func TestMountedRiderSitRequestRefused(t *testing.T) {
	t.Parallel()
	h, watcher := mountedStriderRider(t)
	for _, tc := range []struct {
		what    string
		request []byte
	}{
		{"RequestChangeWaitType(sit)", encodeRequestChangeWaitType(false)},
		{"RequestChangeWaitType(stand)", encodeRequestChangeWaitType(true)},
		{"RequestActionUse(0)", encodeRequestActionUse(0, false)},
	} {
		h.client.Send(tc.request)
		frames := drainFrames(t, h.client)
		if got, want := frameOpcodes(frames), []byte{serverpackets.OpcodeActionFailed}; string(got) != string(want) {
			t.Fatalf("%s from a rider = % x, want % x", tc.what, got, want)
		}
		requireRiderUnmoved(t, h, watcher, tc.what)
	}
}

// TestRidingRiderSitRequestStopsTheRide pins the idle a rider's refused sit
// goes to: a ride under way stops, the StopMove ahead of the ActionFailed,
// and the rider stays mounted and standing.
func TestRidingRiderSitRequestStopsTheRide(t *testing.T) {
	t.Parallel()
	h, watcher := mountedStriderRider(t)
	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	h.client.Send(encodeMoveBackwardToLocation(int32(x+1000), int32(y), int32(z)))
	readUntilOpcode(t, h.client, serverpackets.OpcodeMoveToLocation, "ride MoveToLocation")
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, watcher.client)
	if !h.srv.PlayerMove(t, h.ownerID).Moving() {
		t.Fatal("rider not moving after the walk request")
	}

	h.client.Send(encodeRequestChangeWaitType(false))
	frames := drainFrames(t, h.client)
	if got, want := frameOpcodes(frames), []byte{serverpackets.OpcodeStopMove, serverpackets.OpcodeActionFailed}; string(got) != string(want) {
		t.Fatalf("sit from a riding rider = % x, want % x", got, want)
	}
	if h.srv.PlayerMove(t, h.ownerID).Moving() {
		t.Fatal("rider still moving after the refused sit")
	}
	rider := h.character(t)
	if !rider.Mounted() || !rider.Standing() {
		t.Fatalf("rider mounted=%v standing=%v, want mounted and standing", rider.Mounted(), rider.Standing())
	}
	if !hasOpcode(drainFrames(t, watcher.client), serverpackets.OpcodeStopMove) {
		t.Fatal("watcher saw no StopMove for the stopped ride")
	}
}
