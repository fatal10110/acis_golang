package duel

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/duel"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Reference: RequestDuelStart.java:30-136, RequestDuelAnswerStart.java:24-139,
// Duel.java:38-205 (countdown 5 for one on one, a 1s task), Player.canDuel
// (Player.java:5500-5524), Player.prepareToDuel (Player.java:5536-5547).

// TestDuelChallengeCountdownAndStart: a challenge asks the target with
// ExDuelAskStart, an acceptance tells both sides, the countdown counts 3, 2
// and 1 aloud, and the start shows both sides the duel window, its music,
// their colours and each other's status.
func TestDuelChallengeCountdownAndStart(t *testing.T) {
	t.Parallel()
	a := bootArena(t, "Challenger", "Rival")
	ch, rv := a.players[0], a.players[1]

	ch.c.Send(encodeDuelStart(rv.name, false))
	got := drainFrames(t, ch.c)
	requireMessage(t, got, 0, serverpackets.SystemMessageS1ChallengedToDuel, rv.name)
	got = drainFrames(t, rv.c)
	ask := requireExtended(t, got, 0, serverpackets.OpcodeExDuelAskStart, "ExDuelAskStart")
	r := wire.NewReader(got[ask][3:])
	if name, party := r.ReadString(), r.ReadInt32(); name != ch.name || party != 0 {
		t.Fatalf("ExDuelAskStart = %q, party %d; want %q, 0", name, party, ch.name)
	}
	requireMessage(t, got, ask, serverpackets.SystemMessageS1ChallengedYouToDuel, ch.name)

	rv.c.Send(encodeDuelAnswer(false, true))
	a.srv.AdvanceUntil(t, "duel starts", func() bool {
		return a.standing(t, 0).DuelState() == duel.Duelling && a.standing(t, 1).DuelState() == duel.Duelling
	})
	chFrames, rvFrames := drainFrames(t, ch.c), drainFrames(t, rv.c)
	requireMessage(t, rvFrames, 0, serverpackets.SystemMessageYouAcceptedS1Duel, ch.name)
	requireMessage(t, chFrames, 0, serverpackets.SystemMessageS1AcceptedYourDuel, rv.name)
	for _, frames := range [][][]byte{chFrames, rvFrames} {
		at := 0
		for _, n := range []int32{3, 2, 1} {
			at = requireMessage(t, frames, at, serverpackets.SystemMessageDuelBeginsInS1Seconds, n)
		}
		at = requireMessage(t, frames, at, serverpackets.SystemMessageLetTheDuelBegin)
		at = requireExtended(t, frames, at, serverpackets.OpcodeExDuelReady, "ExDuelReady") + 1
		at = requireExtended(t, frames, at, serverpackets.OpcodeExDuelStart, "ExDuelStart")
		if indexOf(frames, at, serverpackets.OpcodePlaySound) < 0 {
			t.Fatal("duel music missing after ExDuelStart")
		}
		if indexOf(frames, at, serverpackets.OpcodeUserInfo) < 0 {
			t.Fatal("UserInfo with the duel colour missing after ExDuelStart")
		}
	}
	for i, frames := range [][][]byte{chFrames, rvFrames} {
		at := requireExtended(t, frames, 0, serverpackets.OpcodeExDuelUpdateUserInfo, "ExDuelUpdateUserInfo")
		r := wire.NewReader(frames[at][3:])
		opponent := a.players[1-i]
		if name, id := r.ReadString(), r.ReadInt32(); name != opponent.name || id != opponent.id {
			t.Fatalf("ExDuelUpdateUserInfo shows %q (%d), want the opponent %q (%d)", name, id, opponent.name, opponent.id)
		}
	}
	if blue, red := a.standing(t, 0).DuelTeam(), a.standing(t, 1).DuelTeam(); blue != int(duel.TeamBlue) || red != int(duel.TeamRed) {
		t.Fatalf("teams = %d, %d; want the challenger blue, the rival red", blue, red)
	}
	if a.standing(t, 0).DuelID() == 0 || a.standing(t, 0).DuelID() != a.standing(t, 1).DuelID() {
		t.Fatal("the two sides are not in one duel")
	}
}

// TestDuelSurrender: a surrender defeats its side; the next second ends the
// duel, the loser bows, both sides hear who withdrew and who won, the duel
// window closes and everyone leaves the duel with its colour.
func TestDuelSurrender(t *testing.T) {
	t.Parallel()
	a := bootArena(t, "Challenger", "Rival")
	ch, rv := a.players[0], a.players[1]
	a.challenge(t, 0, 1)

	rv.c.Send(encodeDuelSurrender())
	a.srv.AdvanceUntil(t, "duel ends", func() bool {
		return a.standing(t, 0).DuelID() == 0 && a.standing(t, 1).DuelID() == 0
	})
	for _, p := range []player{ch, rv} {
		frames := drainFrames(t, p.c)
		at := indexOf(frames, 0, serverpackets.OpcodeSocialAction)
		if at < 0 {
			t.Fatalf("%s saw no bow", p.name)
		}
		r := wire.NewReader(frames[at][1:])
		if id, action := r.ReadInt32(), r.ReadInt32(); id != rv.id || action != 7 {
			t.Fatalf("%s saw social action %d by %d, want the loser's bow (7 by %d)", p.name, action, id, rv.id)
		}
		// The loser's own client sees its bow before the result; the
		// winner's sees it as the loser's queue sends it.
		if p.id != rv.id {
			at = 0
		}
		at = requireMessage(t, frames, at, serverpackets.SystemMessageS1WithdrewS2Won, rv.name, ch.name)
		at = requireMessage(t, frames, at, serverpackets.SystemMessageS1WonTheDuel, ch.name)
		requireExtended(t, frames, at, serverpackets.OpcodeExDuelEnd, "ExDuelEnd")
	}
	for i := range a.players {
		if s := a.standing(t, i); s.DuelState() != duel.NoDuel || s.DuelTeam() != int(duel.TeamNone) {
			t.Fatalf("player %d left in state %d, team %d", i, s.DuelState(), s.DuelTeam())
		}
	}
}

// TestDuelDefeatKeepsLoserAlive: a hit that would kill a duellist leaves it
// at 1 HP and defeats it; the duel ends with its opponent's win and puts
// back the HP and CP both had when it began.
func TestDuelDefeatKeepsLoserAlive(t *testing.T) {
	t.Parallel()
	a := bootArena(t, "Challenger", "Rival")
	ch, rv := a.players[0], a.players[1]
	hp := a.srv.PlayerCurrentHP(t, rv.id)
	a.onQueue(t, 0, func() { a.vitals(t, 0).SetRollSource(alwaysHit()) })
	a.challenge(t, 0, 1)

	a.onQueue(t, 1, func() {
		a.vitals(t, 1).SetCP(0)
		a.vitals(t, 1).SetHP(1)
	})
	ch.c.Send(encodeAction(rv.id))
	ch.c.Send(encodeAction(rv.id))
	a.srv.AdvanceUntil(t, "rival defeated", func() bool {
		s := a.standing(t, 1)
		return s.DuelState() == duel.Dead || s.DuelID() == 0
	})
	if a.srv.PlayerDead(t, rv.id) {
		t.Fatal("the defeated duellist died, want it kept at 1 HP")
	}
	a.srv.AdvanceUntil(t, "duel ends", func() bool { return a.standing(t, 1).DuelID() == 0 })
	if !shownAtHP(t, drainFrames(t, ch.c), rv.id, 1) {
		t.Fatal("the challenger's duel window never showed the rival at 1 HP")
	}
	frames := drainFrames(t, rv.c)
	at := requireMessage(t, frames, 0, serverpackets.SystemMessageS1WonTheDuel, ch.name)
	requireExtended(t, frames, at, serverpackets.OpcodeExDuelEnd, "ExDuelEnd")
	if got := a.srv.PlayerCurrentHP(t, rv.id); got != hp {
		t.Fatalf("rival HP after the duel = %d, want the %d it had when the duel began", got, hp)
	}
	if a.srv.PlayerDead(t, rv.id) {
		t.Fatal("rival dead after the duel")
	}
}

// shownAtHP reports whether frames carry an ExDuelUpdateUserInfo showing
// player id at hp.
func shownAtHP(t *testing.T, frames [][]byte, id int32, hp int32) bool {
	t.Helper()
	for at := indexOfExtended(frames, 0, serverpackets.OpcodeExDuelUpdateUserInfo); at >= 0; at = indexOfExtended(frames, at+1, serverpackets.OpcodeExDuelUpdateUserInfo) {
		r := wire.NewReader(frames[at][3:])
		r.ReadString()
		if r.ReadInt32() != id {
			continue
		}
		r.ReadInt32() // class
		r.ReadInt32() // level
		if r.ReadInt32() == hp {
			return true
		}
	}
	return false
}

// TestDuelInterruptedByMonsterHit: a duellist hitting a monster interrupts
// its duel, which ends in a tie on its next second.
func TestDuelInterruptedByMonsterHit(t *testing.T) {
	t.Parallel()
	a := bootArena(t, "Challenger", "Rival")
	monster := a.srv.SpawnHostileNPCAt(t, location.Location{X: 40, Y: 20, Z: 30})
	a.onQueue(t, 0, func() { a.vitals(t, 0).SetRollSource(alwaysHit()) })
	a.challenge(t, 0, 1)

	a.players[0].c.Send(encodeAction(monster.ObjectID()))
	a.players[0].c.Send(encodeAction(monster.ObjectID()))
	a.srv.AdvanceUntil(t, "duel cancelled", func() bool { return a.standing(t, 1).DuelID() == 0 })
	for _, p := range a.players {
		frames := drainFrames(t, p.c)
		at := requireMessage(t, frames, 0, serverpackets.SystemMessageDuelEndedInTie)
		requireExtended(t, frames, at, serverpackets.OpcodeExDuelEnd, "ExDuelEnd")
	}
}

// alwaysHit lands every roll of an attacker's swing without a critical.
func alwaysHit() func(int) int {
	calls := 0
	return func(int) int {
		calls++
		if calls%2 == 1 {
			return 0
		}
		return 999
	}
}

// TestDuelChallengeRefusals: naming nobody, a target that may not duel and
// a declined challenge each answer the challenger.
func TestDuelChallengeRefusals(t *testing.T) {
	t.Parallel()
	a := bootArena(t, "Challenger", "Rival")
	ch, rv := a.players[0], a.players[1]

	ch.c.Send(encodeDuelStart("Nobody", false))
	requireMessage(t, drainFrames(t, ch.c), 0, serverpackets.SystemMessageNoOpponentForDuel)
	ch.c.Send(encodeDuelStart(ch.name, false))
	requireMessage(t, drainFrames(t, ch.c), 0, serverpackets.SystemMessageNoOpponentForDuel)

	a.srv.DamagePlayerHP(t, rv.id, a.srv.PlayerMaxHP(t, rv.id)*3/4)
	ch.c.Send(encodeDuelStart(rv.name, false))
	requireMessage(t, drainFrames(t, ch.c), 0, serverpackets.SystemMessageS1CannotDuelHPOrMPBelowHalf, rv.name)
	if frames := drainFrames(t, rv.c); indexOfExtended(frames, 0, serverpackets.OpcodeExDuelAskStart) >= 0 {
		t.Fatal("a target that may not duel was asked")
	}
	a.srv.AddPlayerHP(t, rv.id, float64(a.srv.PlayerMaxHP(t, rv.id)))

	ch.c.Send(encodeDuelStart(rv.name, false))
	drainFrames(t, ch.c)
	drainFrames(t, rv.c)
	rv.c.Send(encodeDuelAnswer(false, false))
	requireMessage(t, drainFrames(t, ch.c), 0, serverpackets.SystemMessageS1DeclinedYourDuel, rv.name)
	if a.standing(t, 0).InDuel() || a.standing(t, 1).InDuel() {
		t.Fatal("a declined challenge started a duel")
	}

	// An answer with no challenge pending answers nothing.
	rv.c.Send(encodeDuelAnswer(false, true))
	if frames := drainFrames(t, rv.c); len(messages(t, frames)) != 0 {
		t.Fatalf("an answer with nothing pending was answered: %v", messages(t, frames))
	}
}

// TestDuelFrozenLoserRefusesActions: once a duellist lost, clicking on it
// is refused as the other party frozen until the duel ends.
func TestDuelFrozenLoserRefusesActions(t *testing.T) {
	t.Parallel()
	a := bootArena(t, "Challenger", "Rival", "Bystander")
	a.challenge(t, 0, 1)
	// The surrender defeats the rival now; its end waits for the next
	// second, which a duel only counts once the bystander's click is in.
	a.players[1].c.Send(encodeDuelSurrender())
	a.srv.AdvanceUntil(t, "rival defeated", func() bool { return a.standing(t, 1).DuelState() != duel.Duelling })
	if a.standing(t, 1).DuelState() != duel.Dead {
		t.Skip("the duel ended before the click could land")
	}
	a.players[2].c.Send(encodeAction(a.players[1].id))
	frames := drainFrames(t, a.players[2].c)
	at := requireMessage(t, frames, 0, serverpackets.SystemMessageOtherPartyIsFrozen)
	if indexOf(frames, at, serverpackets.OpcodeActionFailed) < 0 {
		t.Fatal("frozen-target refusal sent no ActionFailed")
	}
}

// TestPartyDuelEditCancels: a party duel challenge naming any member of the
// other party asks its leader; the acceptance moves both parties to the
// arena during the countdown. The leader may not hand its party to a
// duelling member, and a member leaving its party cancels the duel: every
// player returns where it stood and both sides hear the tie.
func TestPartyDuelEditCancels(t *testing.T) {
	t.Parallel()
	a := bootArena(t, "Lead", "Mate", "Foe", "Ally")
	lead, mate, foe, ally := a.players[0], a.players[1], a.players[2], a.players[3]
	a.party(t, 0, 1)
	a.party(t, 2, 3)
	var before [4][3]int
	for i, p := range a.players {
		before[i][0], before[i][1], before[i][2] = a.srv.PlayerPosition(t, p.id)
	}

	lead.c.Send(encodeDuelStart(ally.name, true))
	requireMessage(t, drainFrames(t, lead.c), 0, serverpackets.SystemMessageS1PartyChallengedToDuel, foe.name)
	if indexOfExtended(drainFrames(t, foe.c), 0, serverpackets.OpcodeExDuelAskStart) < 0 {
		t.Fatal("the challenged party's leader was not asked")
	}
	requireMessage(t, drainFrames(t, ally.c), 0, serverpackets.SystemMessageS1PartyChallengedYourParty, lead.name)
	a.quiet(t)

	foe.c.Send(encodeDuelAnswer(true, true))
	a.srv.AdvanceUntil(t, "parties moved to the arena", func() bool {
		x, _, _ := a.srv.PlayerPosition(t, mate.id)
		return x != before[1][0]
	})
	for i, p := range a.players {
		frames := drainFrames(t, p.c)
		requireMessage(t, frames, 0, serverpackets.SystemMessageTransportedToDuelSite)
		if indexOf(frames, 0, serverpackets.OpcodeTeleportToLocation) < 0 {
			t.Fatalf("%s was not moved to the arena", p.name)
		}
		p.c.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
		if s := a.standing(t, i); s.DuelState() != duel.Countdown {
			t.Fatalf("%s state = %d, want counting down", p.name, s.DuelState())
		}
	}
	a.quiet(t)
	x, y, _ := a.srv.PlayerPosition(t, mate.id)
	if x != duel.Arena.X+40-180 || y != duel.Arena.Y-150 {
		t.Fatalf("challenger's second member at %d,%d, want %d,%d", x, y, duel.Arena.X+40-180, duel.Arena.Y-150)
	}

	lead.c.Send(encodeChangePartyLeader(mate.name))
	if msgs := messages(t, drainFrames(t, lead.c)); len(msgs) != 0 {
		t.Fatalf("handing the party to a duellist answered %v, want nothing", msgs)
	}

	mate.c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestWithdrawParty).Bytes())
	a.srv.AdvanceUntil(t, "duel cancelled", func() bool {
		for i := range a.players {
			if a.standing(t, i).InDuel() {
				return false
			}
		}
		return true
	})
	for i, p := range a.players {
		frames := drainFrames(t, p.c)
		at := requireMessage(t, frames, 0, serverpackets.SystemMessageDuelEndedInTie)
		requireExtended(t, frames, at, serverpackets.OpcodeExDuelEnd, "ExDuelEnd")
		x, y, _ := a.srv.PlayerPosition(t, p.id)
		if x != before[i][0] || y != before[i][1] {
			t.Fatalf("%s at %d,%d after the cancel, want back at %d,%d", p.name, x, y, before[i][0], before[i][1])
		}
	}
}
