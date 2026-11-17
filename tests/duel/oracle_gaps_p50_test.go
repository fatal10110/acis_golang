package duel

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/duel"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// Evidence for the duel endings the first parity sweep left without a
// scenario (Duel.checkEndDuelCondition, Duel.java:395-496; Duel.endDuel,
// Duel.java:352-389).

// duelEnded waits until neither player is in a duel.
func (a *arena) duelEnded(t *testing.T, what string) {
	t.Helper()
	a.srv.AdvanceUntil(t, what, func() bool {
		return a.standing(t, 0).DuelID() == 0 && a.standing(t, 1).DuelID() == 0
	})
}

// wantTie fails unless frames end a duel as a tie: the tie message, then
// the end of the duel.
func wantTie(t *testing.T, who string, frames [][]byte) {
	t.Helper()
	at := requireMessage(t, frames, 0, serverpackets.SystemMessageDuelEndedInTie)
	requireExtended(t, frames, at, serverpackets.OpcodeExDuelEnd, who+" ExDuelEnd")
	if indexOfMessage(t, frames, 0, serverpackets.SystemMessageS1WonTheDuel) >= 0 {
		t.Fatalf("%s was told a duellist won a tied duel", who)
	}
}

// TestDuelTimesOutAsATie lets a duel run its two minutes: both duellists
// are told it ended in a tie, with the end of the duel after it, and
// neither is left in the duel.
func TestDuelTimesOutAsATie(t *testing.T) {
	t.Parallel()
	a := bootArena(t, "Challenger", "Rival")
	a.challenge(t, 0, 1)

	a.srv.Advance(t, 100*time.Second)
	for _, p := range a.players {
		if indexOfMessage(t, drainFrames(t, p.c), 0, serverpackets.SystemMessageDuelEndedInTie) >= 0 {
			t.Fatalf("%s was told the duel ended before its two minutes", p.name)
		}
	}
	a.srv.Advance(t, 30*time.Second)
	a.duelEnded(t, "duel times out")
	for _, p := range a.players {
		wantTie(t, p.name, drainFrames(t, p.c))
	}
}

// The cancellation by a peace, siege or PvP zone is not covered: a
// duellist enters a zone only by walking into it, which no fixture of the
// suite drives for a player yet.

// TestDuelCancelledWhenItsPlayersAreOutOfRangeOrFlagged cancels a duel as
// a tie: a duellist more than 2000 away from the other, one carrying a PvP
// flag.
func TestDuelCancelledWhenItsPlayersAreOutOfRangeOrFlagged(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		// cancel disturbs the duel of player 1 from player 0.
		cancel func(t *testing.T, a *arena)
	}{
		{"out of range", func(t *testing.T, a *arena) {
			x, y, z := a.srv.PlayerPosition(t, a.players[1].id)
			a.onQueue(t, 1, func() {
				a.character(t, 1).SetXYZ(x+2500, y, z)
			})
		}},
		{"pvp flag", func(t *testing.T, a *arena) {
			a.onQueue(t, 1, func() { a.character(t, 1).UpdatePvPFlag(task.PvPFlagOn) })
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := bootArena(t, "Challenger", "Rival")
			a.challenge(t, 0, 1)
			tc.cancel(t, a)
			a.duelEnded(t, "duel cancelled")
			for _, p := range a.players {
				wantTie(t, p.name, drainFrames(t, p.c))
			}
		})
	}
}

// TestDuelLeaderLogoutSurrenders has the challenger leave the world in a
// duel: the check finds it offline and it loses, its rival told it
// withdrew and the rival won, then the duel ends.
func TestDuelLeaderLogoutSurrenders(t *testing.T) {
	t.Parallel()
	a := bootArena(t, "Challenger", "Rival")
	ch, rv := a.players[0], a.players[1]
	a.challenge(t, 0, 1)

	ch.c.Send(wire.NewPacketWriter(clientpackets.OpcodeLogout).Bytes())
	a.srv.AdvanceUntil(t, "duel ends for the rival", func() bool { return a.standing(t, 1).DuelID() == 0 })
	frames := drainFrames(t, rv.c)
	at := requireMessage(t, frames, 0, serverpackets.SystemMessageS1WithdrewS2Won, ch.name, rv.name)
	at = requireMessage(t, frames, at, serverpackets.SystemMessageS1WonTheDuel, rv.name)
	requireExtended(t, frames, at, serverpackets.OpcodeExDuelEnd, "ExDuelEnd")
}

// TestPartyDuelLeaderLogoutCancels has the leader of a party duelling in
// a party of three leave the world: its party's edit cancels the duel for
// everyone left (Party.removePartyMember, DuelManager.onPartyEdit), each
// told it ended in a tie and put back where it stood.
func TestPartyDuelLeaderLogoutCancels(t *testing.T) {
	t.Parallel()
	a := bootArena(t, "Lead", "Mate", "Third", "Foe", "Ally")
	lead, foe, ally := a.players[0], a.players[3], a.players[4]
	a.party(t, 0, 1)
	a.party(t, 0, 2)
	a.party(t, 3, 4)
	var before [5][3]int
	for i, p := range a.players {
		before[i][0], before[i][1], before[i][2] = a.srv.PlayerPosition(t, p.id)
	}

	lead.c.Send(encodeDuelStart(ally.name, true))
	a.quiet(t)
	foe.c.Send(encodeDuelAnswer(true, true))
	a.srv.AdvanceUntil(t, "parties in the duel", func() bool {
		for i := range a.players {
			if a.standing(t, i).DuelState() != duel.Countdown {
				return false
			}
		}
		return true
	})
	for _, p := range a.players {
		p.c.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
	}
	a.quiet(t)

	lead.c.Send(wire.NewPacketWriter(clientpackets.OpcodeLogout).Bytes())
	a.srv.AdvanceUntil(t, "duel cancelled", func() bool {
		for i := 1; i < len(a.players); i++ {
			if a.standing(t, i).InDuel() {
				return false
			}
		}
		return true
	})
	for i, p := range a.players[1:] {
		wantTie(t, p.name, drainFrames(t, p.c))
		if s := a.standing(t, i+1); s.DuelState() != duel.NoDuel || s.DuelTeam() != int(duel.TeamNone) {
			t.Fatalf("%s left in state %d, team %d", p.name, s.DuelState(), s.DuelTeam())
		}
		if x, y, z := a.srv.PlayerPosition(t, p.id); [3]int{x, y, z} != before[i+1] {
			t.Fatalf("%s put back at %d,%d,%d, want %v", p.name, x, y, z, before[i+1])
		}
	}
}

// character is player i's online character.
func (a *arena) character(t *testing.T, i int) interface {
	SetXYZ(x, y, z int)
	UpdatePvPFlag(task.PvPFlagState)
} {
	t.Helper()
	obj, ok := a.srv.State.Player(a.players[i].id)
	if !ok {
		t.Fatalf("player %d not in world", a.players[i].id)
	}
	return obj.(interface {
		SetXYZ(x, y, z int)
		UpdatePvPFlag(task.PvPFlagState)
	})
}
