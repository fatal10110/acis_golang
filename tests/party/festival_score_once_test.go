package party

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/festival"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// A festival run registers its score once (owner-approved divergence,
// #5112): the leader coming back with more blood offerings each time is
// refused as a leader of no party waiting to register (page 3b), keeps
// its offerings, and Dawn's festival score stays at one run's worth.
func TestFestivalScoreRegistersOncePerRun(t *testing.T) {
	t.Parallel()
	f := bootFestival(t, []seat{{"Leader", 20}, {"Member", 20}}, 1000, 2)
	leader := f.players[0]
	f.invite(t, 0, 1, 0)
	f.signUp(t)
	f.begin(t, 0, 1)
	for _, d := range []time.Duration{2 * time.Minute, 3 * time.Minute, 4 * time.Minute, 3 * time.Minute, 3 * time.Minute, 3 * time.Minute} {
		f.timers.fire(t, d)
	}
	for _, p := range f.players {
		p.c.Send(encodeSingle(clientpackets.OpcodeAppearing))
	}
	f.quiet(t)

	offering := int32(festival.BloodOfferingID)
	f.pickUp(t, 0, leader.id, offering, 1)
	answer := f.command(t, 0, "3")
	requireMessage(t, "registration", answer, serverpackets.SystemMessageContribScoreIncreasedS1, int32(5))
	if got := festivalPage(t, answer); got != "page 3c" {
		t.Fatalf("registration = %q, want page 3c", got)
	}
	held := 0
	for n := int32(2); n <= 5; n++ {
		f.pickUp(t, 0, leader.id, offering, n)
		held += int(n)
		answer := f.command(t, 0, "3")
		if got := festivalPage(t, answer); got != "page 3b" {
			t.Fatalf("registration with %d offerings again = %q, want page 3b", n, got)
		}
		for _, m := range systemMessages(answer) {
			if m.id == serverpackets.SystemMessageContribScoreIncreasedS1 {
				t.Fatalf("registration with %d offerings again raised the contribution: %v", n, m.params)
			}
		}
	}
	if got := f.carried(t, 0, offering); got != held {
		t.Fatalf("leader holds %d offerings, want the %d given after the registration", got, held)
	}
	var score, dawnScore int
	if err := f.srv.DB.QueryRow(`SELECT score FROM seven_signs_festival WHERE festivalId = 0 AND cabal = 'DAWN' AND cycle = ?`, f.srv.SevenSigns.CurrentCycle()).Scan(&score); err != nil || score != 5 {
		t.Fatalf("Dawn's record = %d (%v), want 5", score, err)
	}
	if err := f.srv.DB.QueryRow(`SELECT dawn_festival_score FROM seven_signs_status WHERE id = 0`).Scan(&dawnScore); err != nil || dawnScore != festival.MaxScores[0] {
		t.Fatalf("dawn festival score = %d (%v), want %d", dawnScore, err, festival.MaxScores[0])
	}
}
