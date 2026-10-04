package network

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestBoatRequestsNeverGoSilent extends the guardrail of
// TestGameClientLinkNeverGoesSilentOnActionRequests to the boat passenger
// requests: boarding without leave, stepping off a boat one does not ride,
// and a deck click on a boat that does not exist are refused with
// ActionFailed; a deck click whose target is its origin is answered with
// the deck stop alone, as the reference answers it.
//
// CannotMoveAnymoreInVehicle from a player ashore, or about a boat it does
// not ride, answers nothing, as the reference does
// (CannotMoveAnymoreInVehicle.java:34-39): it reports the end of a deck walk
// the client made by itself, no action it waits on. tests/boat asserts that
// silence.
func TestBoatRequestsNeverGoSilent(t *testing.T) {
	c, chars, _, _ := newLinkedGameClient(t)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	chars.soleObjectID(t)
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)
	testsupport.SyncBarrierFrames(t, c, func() { c.Send(encodeRequestManorList()) }, serverpackets.OpcodeExtended)

	const missingBoat = 999999
	point := func(opcode byte, values ...int32) []byte {
		w := wire.NewPacketWriter(opcode)
		for _, v := range values {
			w.WriteInt32(v)
		}
		return w.Bytes()
	}
	cases := []struct {
		name    string
		payload []byte
		want    []byte
	}{
		{"RequestGetOnVehicle without leave to board", point(clientpackets.OpcodeRequestGetOnVehicle, missingBoat, 0, -100, -40), []byte{serverpackets.OpcodeActionFailed}},
		{"RequestGetOffVehicle ashore", point(clientpackets.OpcodeRequestGetOffVehicle, missingBoat, 0, 0, 0), []byte{serverpackets.OpcodeActionFailed}},
		{"RequestMoveToLocationInVehicle on no boat", point(clientpackets.OpcodeRequestMoveInVehicle, missingBoat, 0, -100, -40, 10, -100, -40), []byte{serverpackets.OpcodeActionFailed}},
		{"RequestGetOnVehicle with leave, on no boat", point(clientpackets.OpcodeRequestGetOnVehicle, missingBoat, 0, -100, -40), []byte{serverpackets.OpcodeActionFailed}},
		{"RequestMoveToLocationInVehicle going nowhere", point(clientpackets.OpcodeRequestMoveInVehicle, missingBoat, 0, -100, -40, 0, -100, -40), []byte{serverpackets.OpcodeStopMoveInVehicle}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			frames := testsupport.SyncBarrierFrames(t, c, func() {
				c.Send(tc.payload)
				c.Send(encodeRequestManorList())
			}, serverpackets.OpcodeExtended)
			got := make([]byte, len(frames))
			for i, f := range frames {
				got[i] = f[0]
			}
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("%s: reply opcodes = %x, want %x", tc.name, got, tc.want)
			}
		})
	}
}
