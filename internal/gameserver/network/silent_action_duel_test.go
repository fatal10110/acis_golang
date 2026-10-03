package network

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestDuelRequestsNeverGoSilent extends the guardrail of
// TestGameClientLinkNeverGoesSilentOnActionRequests to the duel challenge:
// one naming nobody, or the challenger itself, is refused aloud.
//
// RequestDuelAnswerStart with no challenge pending, or whose challenger has
// left, and RequestDuelSurrender outside a duel, answer nothing, as the
// reference does (RequestDuelAnswerStart.java:31-33,
// DuelManager.doSurrender): the challenge dialog closed when the client
// answered, and a surrender registers no pending client action — the duel's
// end answers it. tests/duel asserts the unanswered answer.
func TestDuelRequestsNeverGoSilent(t *testing.T) {
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

	challenge := func(name string) []byte {
		w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
		w.WriteUint16(clientpackets.OpcodeRequestDuelStart)
		w.WriteString(name)
		w.WriteInt32(0)
		return w.Bytes()
	}
	cases := []struct {
		name    string
		payload []byte
		want    []byte
	}{
		{"RequestDuelStart naming nobody online", challenge("Nobody"), []byte{serverpackets.OpcodeSystemMessage}},
		{"RequestDuelStart naming the challenger", challenge("Newbie"), []byte{serverpackets.OpcodeSystemMessage}},
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
