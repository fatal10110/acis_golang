package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Reference: ItemInstance.onAction refuses a castle guard item's click with
// ActionFailed alone when the clicker is in a party, when no castle's siege
// zone holds the clicker (CastleManager.getCastle(player) == null), and when
// the item is no ticket of that castle (castle.getTicket(itemId) == null);
// none of them picks the item up.

// TestMercenaryTicketClickRefusals has Gludio's lord click a ticket it may
// not pick up: one it placed while it is in a party, one it dropped where no
// castle's grounds lie, and a Dion ticket it dropped on Gludio's grounds.
// Each click answers ActionFailed alone; the ticket stays on the ground and
// Gludio's count of tickets does not move.
func TestMercenaryTicketClickRefusals(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		setup  ticketSetup
		ticket int32
		// place puts the leader's ticket on the ground and returns the
		// ground object's id.
		place func(w *castleWorld, t *testing.T, ticket int32) int32
	}{
		{"lord in a party", ticketSetup{ownsCastle: 1}, gludioSwordTicket, func(w *castleWorld, t *testing.T, ticket int32) int32 {
			ground, _, ok := droppedGround(w.useTicket(t, ticket))
			if !ok {
				t.Fatal("no ticket placed")
			}
			drainUntilQuiet(t, w.gm)
			w.joinParty(t)
			return ground
		}},
		{"outside every castle", ticketSetup{noField: true, ownsCastle: 1}, gludioSwordTicket, (*castleWorld).dropTicket},
		{"another castle's ticket", ticketSetup{ownsCastle: 1}, dionSwordTicket, (*castleWorld).dropTicket},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := bootTicketWorld(t, tc.setup)
			ticket := w.srv.GiveItem(t, w.leaderID, tc.ticket, 2)
			w.enter(t)
			ground := tc.place(w, t, ticket)
			drainUntilQuiet(t, w.leader)
			before := gludioDropped(t, w.srv)

			w.leader.Send(encodeAction(ground, 0, 0, 0, false))
			frames := collectFrames(w.leader)
			if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
				t.Fatalf("lord's click: opcodes %x, want ActionFailed alone", opcodesOf(frames))
			}
			if _, ok := w.srv.State.Object(ground); !ok {
				t.Fatal("ticket left the world after a refused click")
			}
			if got := gludioDropped(t, w.srv); got != before {
				t.Fatalf("Gludio dropped tickets after a refused click = %d, want %d", got, before)
			}
		})
	}
}

// dropTicket has the leader drop one ticket of its stack ticket at its
// feet through RequestDropItem and returns the ground object's id.
func (w *castleWorld) dropTicket(t *testing.T, ticket int32) int32 {
	t.Helper()
	x, y, z := w.srv.PlayerPosition(t, w.leaderID)
	w.leader.Send(encodeRequestDropItem(ticket, 1, int32(x), int32(y), int32(z)))
	frames := collectFrames(w.leader)
	ground, _, ok := droppedGround(frames)
	if !ok {
		t.Fatalf("ticket drop: opcodes %x, want a DropItem", opcodesOf(frames))
	}
	drainUntilQuiet(t, w.gm)
	return ground
}

// joinParty has the leader invite the game master into a party, which it
// accepts.
func (w *castleWorld) joinParty(t *testing.T) {
	t.Helper()
	invite := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinParty)
	invite.WriteString("Admin")
	invite.WriteInt32(0)
	w.leader.Send(invite.Bytes())
	if asked := collectFrames(w.gm); findFrame(asked, 0, serverpackets.OpcodeAskJoinParty, 0) < 0 {
		t.Fatalf("game master not asked to join: opcodes %x", opcodesOf(asked))
	}
	answer := wire.NewPacketWriter(clientpackets.OpcodeRequestAnswerJoinParty)
	answer.WriteInt32(1)
	w.gm.Send(answer.Bytes())
	if joined := collectFrames(w.leader); findFrame(joined, 0, serverpackets.OpcodePartySmallWindowAdd, 0) < 0 {
		t.Fatalf("leader saw no party form: opcodes %x", opcodesOf(joined))
	}
	drainUntilQuiet(t, w.gm)
}
