package network

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// RewardParty hands the kill reward a command channel's members as
// characters, party by party, with the channel's level.
func TestRewardPartyCommandChannel(t *testing.T) {
	link := &GameClientLink{parties: party.NewRegistry[*livePlayer](func() time.Time { return time.Unix(0, 0) })}
	ps := make([]*livePlayer, 5)
	for i, lvl := range []int{40, 30, 35, 52, 20} {
		ps[i] = newTestLivePlayer(t, int32(i+1), &testsupport.FrameCapture{})
		ps[i].CharLevel = lvl
	}
	form := func(leader, target *livePlayer) {
		if status, _ := link.parties.BeginInvite(leader.ObjectID(), 0); status != party.InviteReady {
			t.Fatalf("BeginInvite = %v", status)
		}
		link.parties.Answer(leader, target, true)
	}
	form(ps[0], ps[1])
	form(ps[2], ps[3])

	if _, ok := link.RewardParty(ps[4].ObjectID()); ok {
		t.Fatal("a player in no party has a reward party")
	}
	g, ok := link.RewardParty(ps[1].ObjectID())
	if !ok || g.InChannel || len(g.Members) != 2 || g.Members[0] != ps[0].Character || g.Members[1] != ps[1].Character {
		t.Fatalf("party group = %+v %v, want Leader and Member outside a channel", g, ok)
	}

	link.parties.JoinChannel(ps[0], ps[2])
	g, ok = link.RewardParty(ps[1].ObjectID())
	if !ok || !g.InChannel || g.ChannelLevel != 52 || len(g.Members) != 4 {
		t.Fatalf("channel group = %+v %v, want four members at level 52", g, ok)
	}
	for i, m := range g.Members {
		if m != ps[i].Character {
			t.Fatalf("channel member %d = %d, want %d", i, m.ObjectID(), ps[i].ObjectID())
		}
	}
}
