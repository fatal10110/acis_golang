package duel

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/duel"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestDuelPartyAnswerToOneOnOneChallenge: an answer naming a party duel to
// a one-on-one challenge, between two players who both lead a party, still
// starts the one-on-one duel the challenge asked for. Nobody else joins it
// and nobody is moved to the arena.
func TestDuelPartyAnswerToOneOnOneChallenge(t *testing.T) {
	t.Parallel()
	a := bootArena(t, "Challenger", "Rival", "Mate", "Ally")
	ch, rv := a.players[0], a.players[1]
	a.party(t, 0, 2)
	a.party(t, 1, 3)
	var before [4][3]int
	for i, p := range a.players {
		before[i][0], before[i][1], before[i][2] = a.srv.PlayerPosition(t, p.id)
	}

	ch.c.Send(encodeDuelStart(rv.name, false))
	a.quiet(t)
	rv.c.Send(encodeDuelAnswer(true, true))
	// A party duel counts 35 seconds down, past AdvanceUntil's limit.
	a.srv.AdvanceUntil(t, "one-on-one duel starts", func() bool {
		return a.standing(t, 0).DuelState() == duel.Duelling && a.standing(t, 1).DuelState() == duel.Duelling
	})
	var frames [4][][]byte
	for i, p := range a.players {
		frames[i] = drainFrames(t, p.c)
	}

	if a.standing(t, 0).DuelID() != a.standing(t, 1).DuelID() {
		t.Fatal("the challenger and the rival are in different duels")
	}
	for _, i := range []int{2, 3} {
		if a.standing(t, i).InDuel() {
			t.Fatalf("party member %s joined a one-on-one duel", a.players[i].name)
		}
	}
	requireMessage(t, frames[1], 0, serverpackets.SystemMessageYouAcceptedS1Duel, ch.name)
	requireMessage(t, frames[0], 0, serverpackets.SystemMessageS1AcceptedYourDuel, rv.name)
	for i, p := range a.players {
		if at := indexOfMessage(t, frames[i], 0, serverpackets.SystemMessageTransportedToDuelSite); at >= 0 {
			t.Fatalf("%s was told it is moved to the duel site", p.name)
		}
		if indexOf(frames[i], 0, serverpackets.OpcodeTeleportToLocation) >= 0 {
			t.Fatalf("%s was teleported", p.name)
		}
		x, y, z := a.srv.PlayerPosition(t, p.id)
		if x != before[i][0] || y != before[i][1] || z != before[i][2] {
			t.Fatalf("%s moved from %v to %d,%d,%d", p.name, before[i], x, y, z)
		}
	}
}
