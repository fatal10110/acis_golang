package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

func encodeJoinParty(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinParty)
	w.WriteString(name)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodeAnswerJoinParty(response int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAnswerJoinParty)
	w.WriteInt32(response)
	return w.Bytes()
}

// joinMate brings a second character, Mate, into the world and into a
// party the scenario's character leads.
func (w *warriorWorld) joinMate(t *testing.T) {
	t.Helper()
	w.srv.SeedCharacterFor(t, "mate", "Mate", 1, 0)
	mate := w.srv.DialClient(t, "mate", 1)
	mate.Send(encodeGameStart())
	for i := 0; ; i++ {
		if i == 100 {
			t.Fatal("no CharSelected for Mate within 100 frames")
		}
		if mate.Read()[0] == serverpackets.OpcodeCharSelected {
			break
		}
	}
	mate.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	w.srv.ReadQueued(t, mate)
	w.srv.ReadQueued(t, w.srv.Client)

	w.srv.Client.Send(encodeJoinParty("Mate"))
	w.srv.ReadQueued(t, mate)
	mate.Send(encodeAnswerJoinParty(1))
	w.srv.ReadQueued(t, mate)
	w.srv.ReadQueued(t, w.srv.Client)
	inParty := make(chan bool, 1)
	if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() { inParty <- script.PlayerOf(w.player).InParty() }) {
		t.Fatal("the character's queue is closed")
	}
	if !<-inParty {
		t.Fatal("the character is in no party after Mate accepted")
	}
}
