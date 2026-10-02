package network

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestPartyMatchRequestsNeverGoSilent extends the guardrail of
// TestGameClientLinkNeverGoesSilentOnActionRequests to the party-matching
// requests that answer a refusal: an invitation or entry naming nothing,
// and an answer with no invitation pending. Opening the window always
// answers with the room list.
//
// The rest answer their refusals with nothing, as the reference does, and
// each comes from a room or waiting-list window that holds no pending
// action: a room revision by anyone but its leader, a room opened by a
// player off the waiting list, an entry by such a player, an oust by
// anyone but the leader or naming nobody in a room, a dismissal of a room
// the player does not lead, a withdrawal from an unknown room or by a
// member of the leader's party, and leaving the waiting list.
// RequestListPartyMatchingWaitingRoom always answers, with an extended
// packet this barrier cannot tell apart. tests/party asserts every one.
func TestPartyMatchRequestsNeverGoSilent(t *testing.T) {
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

	extended := func(second uint16, write func(*wire.Writer)) []byte {
		w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
		w.WriteUint16(second)
		write(w)
		return w.Bytes()
	}
	joinRoom := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinPartyRoom)
	joinRoom.WriteInt32(42)
	joinRoom.WriteInt32(-1)
	joinRoom.WriteInt32(1)
	listRooms := wire.NewPacketWriter(clientpackets.OpcodeRequestListPartyWaiting)
	listRooms.WriteInt32(0)
	listRooms.WriteInt32(-1)
	listRooms.WriteInt32(1)

	cases := []struct {
		name    string
		payload []byte
		want    []byte
	}{
		{"RequestAskJoinPartyRoom naming nobody online", extended(clientpackets.OpcodeRequestAskJoinPartyRoom, func(w *wire.Writer) { w.WriteString("Nobody") }), []byte{serverpackets.OpcodeSystemMessage}},
		{"AnswerJoinPartyRoom with no invitation pending", extended(clientpackets.OpcodeAnswerJoinPartyRoom, func(w *wire.Writer) { w.WriteInt32(1) }), []byte{serverpackets.OpcodeSystemMessage}},
		{"RequestJoinPartyRoom naming no room", joinRoom.Bytes(), []byte{serverpackets.OpcodeSystemMessage}},
		{"RequestListPartyWaiting", listRooms.Bytes(), []byte{serverpackets.OpcodePartyMatchList}},
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
