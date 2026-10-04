package network

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// LootChannel hands the raid loot rights the command channel a player's
// party is in: the same value for every member of it, with the channel's
// member count and its leader's character; a party outside any channel
// has none.
func TestLootChannelCommandChannel(t *testing.T) {
	link := &GameClientLink{parties: party.NewRegistry[*livePlayer](func() time.Time { return time.Unix(0, 0) })}
	ps := make([]*livePlayer, 5)
	for i := range ps {
		ps[i] = newTestLivePlayer(t, int32(i+1), &testsupport.FrameCapture{})
	}
	form := func(leader, target *livePlayer) {
		if status, _ := link.parties.BeginInvite(leader.ObjectID(), 0); status != party.InviteReady {
			t.Fatalf("BeginInvite = %v", status)
		}
		link.parties.Answer(leader, target, true)
	}
	form(ps[0], ps[1])
	form(ps[2], ps[3])
	form(ps[2], ps[4])

	if _, ok := link.LootChannel(ps[1].ObjectID()); ok {
		t.Fatal("a party outside any channel has a loot channel")
	}
	link.parties.JoinChannel(ps[0], ps[2])
	a, ok := link.LootChannel(ps[1].ObjectID())
	if !ok {
		t.Fatal("a channel member has no loot channel")
	}
	if b, _ := link.LootChannel(ps[4].ObjectID()); b != a {
		t.Fatal("two members of one channel got different loot channels")
	}
	if got := a.MembersCount(); got != 5 {
		t.Fatalf("MembersCount = %d, want 5", got)
	}
	if got := a.Leader(); got != ps[0].Character {
		t.Fatalf("Leader = %d, want %d", got.ObjectID(), ps[0].ObjectID())
	}
}
