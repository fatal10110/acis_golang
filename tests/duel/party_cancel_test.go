package duel

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/duel"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestCancelledPartyDuelLeavesTheNextDuelAlone pins #5150: a party duel a
// member's departure cancels takes each player out of it once, in the
// player's own part of the end, so a party duel accepted right after it
// keeps its duel and colours for every player.
func TestCancelledPartyDuelLeavesTheNextDuelAlone(t *testing.T) {
	t.Parallel()
	a := bootArena(t, "Lead", "Mate", "Third", "Foe", "Ally")
	lead, third, foe, ally := a.players[0], a.players[2], a.players[3], a.players[4]
	a.party(t, 0, 1)
	a.party(t, 0, 2)
	a.party(t, 3, 4)
	fighters := []int{0, 1, 3, 4}

	// partyDuel has Lead challenge Foe's party and Foe accept, and returns
	// the duel once every player of both parties counts down in the arena.
	partyDuel := func(want []int) int32 {
		t.Helper()
		lead.c.Send(encodeDuelStart(ally.name, true))
		a.quiet(t)
		foe.c.Send(encodeDuelAnswer(true, true))
		a.srv.AdvanceUntil(t, "parties in the duel", func() bool {
			for _, i := range want {
				if a.standing(t, i).DuelState() != duel.Countdown {
					return false
				}
			}
			return true
		})
		for _, i := range want {
			a.players[i].c.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
		}
		a.quiet(t)
		return a.standing(t, 0).DuelID()
	}

	first := partyDuel([]int{0, 1, 2, 3, 4})
	third.c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestWithdrawParty).Bytes())
	a.srv.AdvanceUntil(t, "duel cancelled", func() bool {
		for i := range a.players {
			if s := a.standing(t, i); s.InDuel() || s.DuelState() != duel.NoDuel || s.DuelTeam() != int(duel.TeamNone) {
				return false
			}
		}
		return true
	})
	for _, i := range []int{0, 1, 2, 3, 4} {
		frames := drainFrames(t, a.players[i].c)
		at := requireMessage(t, frames, 0, serverpackets.SystemMessageDuelEndedInTie)
		requireExtended(t, frames, at, serverpackets.OpcodeExDuelEnd, "ExDuelEnd")
	}

	second := partyDuel(fighters)
	if second == first {
		t.Fatalf("the next duel reused id %d", first)
	}
	a.srv.Advance(t, 30*time.Second)
	a.srv.AdvanceUntil(t, "next duel starts", func() bool {
		for _, i := range fighters {
			if a.standing(t, i).DuelState() != duel.Duelling {
				return false
			}
		}
		return true
	})
	for _, i := range fighters {
		s := a.standing(t, i)
		if s.DuelID() != second || s.DuelTeam() == int(duel.TeamNone) {
			t.Fatalf("%s in duel %d team %d, want duel %d with a colour", a.players[i].name, s.DuelID(), s.DuelTeam(), second)
		}
		frames := drainFrames(t, a.players[i].c)
		at := requireMessage(t, frames, 0, serverpackets.SystemMessageLetTheDuelBegin)
		requireExtended(t, frames, at, serverpackets.OpcodeExDuelStart, "ExDuelStart")
		if end := indexOfExtended(frames, 0, serverpackets.OpcodeExDuelEnd); end >= 0 {
			t.Fatalf("%s got ExDuelEnd at %d in the next duel", a.players[i].name, end)
		}
	}
	if a.standing(t, 0).DuelTeam() != a.standing(t, 1).DuelTeam() || a.standing(t, 3).DuelTeam() == a.standing(t, 0).DuelTeam() {
		t.Fatalf("teams Lead %d Mate %d Foe %d, want Lead and Mate on one side, Foe on the other",
			a.standing(t, 0).DuelTeam(), a.standing(t, 1).DuelTeam(), a.standing(t, 3).DuelTeam())
	}
	if s := a.standing(t, 2); s.InDuel() {
		t.Fatalf("%s, out of the party, is in duel %d", third.name, s.DuelID())
	}
}
