package duel

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/duel"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// partyDuelArena is two parties of two: Lead and Mate against Foe, the
// leader of Ally.
func partyDuelArena(t *testing.T) *arena {
	t.Helper()
	a := bootArena(t, "Lead", "Mate", "Foe", "Ally")
	a.party(t, 0, 1)
	a.party(t, 2, 3)
	return a
}

// startPartyDuel has Lead's party challenge Foe's, Foe accept, and returns
// once every player counts down. The arena teleport comes 2 s into the
// countdown, and each client answers it with Appearing.
func (a *arena) startPartyDuel(t *testing.T) {
	t.Helper()
	a.players[0].c.Send(encodeDuelStart(a.players[3].name, true))
	a.quiet(t)
	a.players[2].c.Send(encodeDuelAnswer(true, true))
	a.srv.AdvanceUntil(t, "parties in the duel", func() bool { return a.allIn(t, duel.Countdown) })
	a.srv.Advance(t, 3*time.Second)
	for _, p := range a.players {
		p.c.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
	}
	a.quiet(t)
}

// allIn reports whether every player is in state s.
func (a *arena) allIn(t *testing.T, s duel.State) bool {
	t.Helper()
	for i := range a.players {
		if a.standing(t, i).DuelState() != s {
			return false
		}
	}
	return true
}

// defeat has player from, which hits whatever it strikes, take player to
// down to 1 HP: the second Action of the pair is the strike.
func (a *arena) defeat(t *testing.T, from, to int) {
	t.Helper()
	a.onQueue(t, to, func() {
		a.vitals(t, to).SetCP(0)
		a.vitals(t, to).SetHP(1)
	})
	a.players[from].c.Send(encodeAction(a.players[to].id))
	a.players[from].c.Send(encodeAction(a.players[to].id))
	a.srv.AdvanceUntil(t, a.players[to].name+" defeated", func() bool {
		s := a.standing(t, to)
		return s.DuelState() == duel.Dead || s.DuelID() == 0
	})
}

// TestPartyDuelEndsWhenTheWholeSideIsDefeated: a party duel goes on while a
// member of the beaten party still fights. Once its last member is
// defeated the other party wins: every player is told so, the losing party
// bows member by member, the duel window closes and each leaves the duel
// and its colour.
func TestPartyDuelEndsWhenTheWholeSideIsDefeated(t *testing.T) {
	t.Parallel()
	a := partyDuelArena(t)
	lead, foe, ally := a.players[0], a.players[2], a.players[3]
	a.startPartyDuel(t)
	a.srv.Advance(t, 30*time.Second)
	a.srv.AdvanceUntil(t, "duel begins", func() bool { return a.allIn(t, duel.Duelling) })
	a.quiet(t)
	a.onQueue(t, 0, func() { a.vitals(t, 0).SetRollSource(alwaysHit()) })
	a.quiet(t)

	a.defeat(t, 0, 2)
	a.srv.Advance(t, 3*time.Second)
	if s := a.standing(t, 2); s.DuelState() != duel.Dead {
		t.Fatalf("Foe's state = %d, want Dead", s.DuelState())
	}
	for i := range a.players {
		if !a.standing(t, i).InDuel() {
			t.Fatalf("%s left the duel while Ally still fights", a.players[i].name)
		}
		frames := drainFrames(t, a.players[i].c)
		if indexOfMessage(t, frames, 0, serverpackets.SystemMessageS1PartyWonTheDuel) >= 0 {
			t.Fatalf("%s was told a party won with a member of the other still fighting", a.players[i].name)
		}
	}

	a.defeat(t, 0, 3)
	a.srv.AdvanceUntil(t, "duel ends", func() bool {
		for i := range a.players {
			if a.standing(t, i).InDuel() {
				return false
			}
		}
		return true
	})
	for i, p := range a.players {
		frames := drainFrames(t, p.c)
		at := requireMessage(t, frames, 0, serverpackets.SystemMessageS1PartyWonTheDuel, lead.name)
		requireExtended(t, frames, at, serverpackets.OpcodeExDuelEnd, p.name+" ExDuelEnd")
		bowed := map[int32]bool{}
		for _, f := range frames {
			if f[0] != serverpackets.OpcodeSocialAction {
				continue
			}
			r := wire.NewReader(f[1:])
			id, action := r.ReadInt32(), r.ReadInt32()
			if action == 7 {
				bowed[id] = true
			}
		}
		if len(bowed) != 2 || !bowed[foe.id] || !bowed[ally.id] {
			t.Fatalf("%s saw bows by %v, want the losing party's two members %d and %d", p.name, bowed, foe.id, ally.id)
		}
		if s := a.standing(t, i); s.DuelState() != duel.NoDuel || s.DuelTeam() != int(duel.TeamNone) {
			t.Fatalf("%s left in state %d, team %d", p.name, s.DuelState(), s.DuelTeam())
		}
	}
}
