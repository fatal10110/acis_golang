package boat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestPassengerAndShorePlayer pins what passes between a player ashore and
// one aboard: selecting the passenger sends no ValidateLocation ahead of
// MyTargetSelected; clicking it again, which would follow it, is answered
// ActionFailed alone; and a duel challenge names it riding a boat.
func TestPassengerAndShorePlayer(t *testing.T) {
	t.Parallel()
	srv, c, me, b := bootPassenger(t, runeBoarding, 0)
	board(t, srv, c, b)
	other, _ := enterSecondClient(t, srv, runeShore)

	other.Send(encodeAction(me, false))
	var got []byte
	for _, f := range srv.ReadQueued(t, other) {
		got = append(got, f[0])
	}
	if len(got) == 0 || got[0] != serverpackets.OpcodeMyTargetSelected {
		t.Fatalf("selecting the passenger: opcodes %x, want MyTargetSelected first", got)
	}

	other.Send(encodeAction(me, false))
	assertLog(t, "second click", passengerLog(t, srv, other), af)

	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestDuelStart)
	w.WriteString("Sailor")
	w.WriteInt32(0)
	other.Send(w.Bytes())
	assertLog(t, "duel challenge", passengerLog(t, srv, other), sm(serverpackets.SystemMessageS1CannotDuelRiding))
}
