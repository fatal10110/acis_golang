package network

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// lifecycleNotices keeps the Formed and Dispersed notices of notices, the
// ones that start and stop a party's position reports.
func lifecycleNotices(notices []party.Notice) []party.Notice {
	var out []party.Notice
	for _, n := range notices {
		switch n.(type) {
		case party.Formed, party.Dispersed:
			out = append(out, n)
		}
	}
	return out
}

func positionQueues(l *GameClientLink) int {
	l.partyPositions.mu.Lock()
	defer l.partyPositions.mu.Unlock()
	return len(l.partyPositions.queues)
}

// A party formed on the target's queue can be dispersed on the leader's
// before the target applies Formed. The Dispersed applied first finds no
// queue to stop; the late Formed must not leave one running for a party
// that no longer exists.
func TestPartyPositionsDispersedBeforeFormedLeavesNoQueue(t *testing.T) {
	in := sim.NewInline(time.Unix(0, 0))
	link := &GameClientLink{log: zerolog.Nop(), queues: in, parties: party.NewRegistry[*livePlayer](in.Now)}
	leader := newTestLivePlayer(t, 1, &testsupport.FrameCapture{})
	target := newTestLivePlayer(t, 2, &testsupport.FrameCapture{})

	for round := range 3 {
		if status, _ := link.parties.BeginInvite(leader.ObjectID(), 0); status != party.InviteReady {
			t.Fatalf("round %d BeginInvite = %v", round, status)
		}
		formed := lifecycleNotices(link.parties.Answer(leader, target, true))
		dispersed := lifecycleNotices(link.parties.Leave(leader, party.Left))
		if len(formed) != 1 || len(dispersed) != 1 {
			t.Fatalf("round %d notices formed %v dispersed %v, want one each", round, formed, dispersed)
		}
		link.applyPartyNotices(dispersed)
		link.applyPartyNotices(formed)
		if n := positionQueues(link); n != 0 {
			t.Fatalf("round %d: %d position queues left for dispersed parties, want none", round, n)
		}
	}

	// A party still standing keeps its queue until it disperses.
	link.parties.BeginInvite(leader.ObjectID(), 0)
	link.applyPartyNotices(lifecycleNotices(link.parties.Answer(leader, target, true)))
	if n := positionQueues(link); n != 1 {
		t.Fatalf("position queues for a standing party = %d, want 1", n)
	}
	link.applyPartyNotices(lifecycleNotices(link.parties.Leave(leader, party.Left)))
	if n := positionQueues(link); n != 0 {
		t.Fatalf("position queues after the party dispersed = %d, want none", n)
	}
}
