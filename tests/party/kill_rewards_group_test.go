package party

import (
	"sync/atomic"
	"testing"

	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPartyKillSkipsDeadMember leaves a member dead at kill time out of the
// party altogether: its damage is not the party's, its level 51 neither
// raises the party's level nor cuts Member 30, and it is no third member
// for the bonus. It gets no gain, and its own entry, left for a later pass
// that finds no party damage, tells it nothing either.
func TestPartyKillSkipsDeadMember(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 40}, {"Member", 30}, {"Dead", 51}})
	g.invite(t, 0, 1, 0)
	g.invite(t, 0, 2, 0)
	x, y, z := g.srv.PlayerPosition(t, g.players[0].id)
	monster := g.srv.SpawnHostileNPCTemplateAt(t, partyRewardMonster(), location.Location{X: x + 60, Y: y + 20, Z: z})
	g.quiet(t)

	monster.TakeDamage(400, g.combatant(t, 0))
	monster.TakeDamage(300, g.combatant(t, 2))
	g.srv.MarkPlayerDead(t, g.players[2].id)
	if !monster.TakeDamage(300, g.combatant(t, 0)) {
		t.Fatal("leader's hit did not kill the monster")
	}

	frames := make([][][]byte, len(g.players))
	for i, p := range g.players {
		frames[i] = drainFrames(t, p.c)
	}
	// The party's 700 of 1000 at level 40, scaled twice, split 1600:900
	// with the two-member bonus.
	if exp, sp := gainIn(t, frames[0]); exp != 2038 || sp != 101 {
		t.Fatalf("leader gained %d/%d, want 2038/101", exp, sp)
	}
	if exp, sp := gainIn(t, frames[1]); exp != 1147 || sp != 56 {
		t.Fatalf("member gained %d/%d, want 1147/56", exp, sp)
	}
	if hasStaticMessage(frames[2], serverpackets.SystemMessageYouEarnedS1ExpAndS2SP) || hasStaticMessage(frames[2], serverpackets.SystemMessageOverHit) {
		t.Fatal("the dead member was told of a gain or an overhit")
	}
}

// TestChannelKillSharesAcrossParties shares a kill in a command channel
// with every party in it: the other party's member in range is paid by
// squared level, and the level the split is cut and penalized at is the
// channel's 52, set by a member out of range, not the 40 of the rewarded
// members. Member 30, 22 levels under it, gets a zero gain.
//
// No packet forms a channel before clans exist, so the link's resolver is
// wrapped to answer for the two parties as one channel; the link's own
// channel answer is pinned in the network package.
func TestChannelKillSharesAcrossParties(t *testing.T) {
	var leaders atomic.Pointer[[2]int32]
	g := bootGroup(t, []seat{{"Leader", 40}, {"Member", 30}, {"Other", 35}, {"Far", 52}},
		gameservertest.WithRewardParties(func(link gamemanager.RewardParties) gamemanager.RewardParties {
			return channelParties{link: link, leaders: &leaders}
		}))
	g.invite(t, 0, 1, 0)
	g.invite(t, 2, 3, 0)
	leaders.Store(&[2]int32{g.players[0].id, g.players[2].id})
	x, y, z := g.srv.PlayerPosition(t, g.players[0].id)
	obj, _ := g.srv.State.Player(g.players[3].id)
	far, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("Far is %T", obj)
	}
	far.TeleportTo(x+3000, y, z, 0)
	monster := g.srv.SpawnHostileNPCTemplateAt(t, partyRewardMonster(), location.Location{X: x + 60, Y: y + 20, Z: z})
	g.quiet(t)

	if !monster.TakeDamage(1000, g.combatant(t, 0)) {
		t.Fatal("leader's hit did not kill the monster")
	}
	// 5000/250 at 12 levels over the monster, with the two-member bonus,
	// split 1600:1225 between Leader and Other.
	want := []gain{{1027, 50, true}, {0, 0, true}, {786, 38, true}, {}}
	if got := g.earned(t); !equalGains(got, want) {
		t.Fatalf("gains = %+v, want %+v", got, want)
	}
}

// channelParties answers for the members of two parties, named by their
// leaders, as one command channel at its highest member's level, the way
// the registry levels a channel; anyone else gets the link's answer.
type channelParties struct {
	link    gamemanager.RewardParties
	leaders *atomic.Pointer[[2]int32]
}

func (c channelParties) RewardParty(playerID int32) (gamemanager.RewardParty, bool) {
	own, ok := c.link.RewardParty(playerID)
	leaders := c.leaders.Load()
	if !ok || leaders == nil {
		return own, ok
	}
	var channel gamemanager.RewardParty
	in := false
	for _, leader := range leaders {
		group, ok := c.link.RewardParty(leader)
		if !ok {
			return own, true
		}
		for _, m := range group.Members {
			in = in || m.ObjectID() == playerID
			channel.ChannelLevel = max(channel.ChannelLevel, m.Level())
		}
		channel.Members = append(channel.Members, group.Members...)
	}
	if !in {
		return own, true
	}
	channel.InChannel = true
	return channel, true
}
