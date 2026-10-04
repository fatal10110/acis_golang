package party

import (
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	playermodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Seats of the loot-rights suite: Leader leads the channel Member fights
// in, Rival fights alone and deals the most damage, and Other is in a
// second channel.
const (
	ccLeader = iota
	ccMember
	ccRival
	ccOther
)

// ccLootAdena is the one guaranteed drop of ccLootBoss.
const ccLootAdena = 77

// The on-screen texts a regular raid boss and Queen Ant announce the loot
// rights with.
const (
	regularRightsText   = "# Leader's Command Channel has looting rights."
	regularNoRightsText = "Looting rules are no longer active."
	queenAntRightsText  = "(Queen Ant)  # Leader's Command Channel has looting rights."
)

// ccLootBoss is a level-40 raid boss of npc id id dropping ccLootAdena.
func ccLootBoss(id int) *npc.Template {
	tmpl := raidBoss(id)
	tmpl.Drops = []item.DropCategory{{Kind: item.DropCurrency, Chance: 100, Drops: []item.Drop{{ItemID: item.AdenaID, Min: ccLootAdena, Max: ccLootAdena, Chance: 100}}}}
	return tmpl
}

// fakeChannel is one command channel of a given size; channels are told
// apart by pointer.
type fakeChannel struct {
	mu      sync.Mutex
	members int
	leader  *playermodel.Character
}

func (c *fakeChannel) MembersCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.members
}

func (c *fakeChannel) Leader() *playermodel.Character { return c.leader }

func (c *fakeChannel) resize(members int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.members = members
}

// fakeChannels answers for the players put in a channel. Forming a real
// channel of more than 18 members takes a dialed client and a clan for
// each, so the suite's resolver stands in for the link's, whose own answer
// is pinned in the network package.
type fakeChannels struct {
	mu       sync.Mutex
	byPlayer map[int32]*fakeChannel
}

func (f *fakeChannels) LootChannel(playerID int32) (gamemanager.LootChannel, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.byPlayer[playerID]
	if !ok {
		return nil, false
	}
	return c, true
}

// join puts the players ids into a new channel of members members led by
// leader.
func (f *fakeChannels) join(leader *playermodel.Character, members int, ids ...int32) *fakeChannel {
	c := &fakeChannel{members: members, leader: leader}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range ids {
		f.byPlayer[id] = c
	}
	return c
}

// bootCCLoot boots the suite's four players beside each other with the
// stand-in channel resolver.
func bootCCLoot(t *testing.T) (*group, *fakeChannels) {
	t.Helper()
	channels := &fakeChannels{byPlayer: map[int32]*fakeChannel{}}
	g := bootGroup(t, []seat{{"Leader", 40}, {"Member", 40}, {"Rival", 40}, {"Other", 40}},
		gameservertest.WithLootChannels(func(gamemanager.LootChannels) gamemanager.LootChannels { return channels }))
	return g, channels
}

// screenTexts returns the on-screen message texts queued to seat i,
// without letting time pass.
func (g *group) screenTexts(t *testing.T, i int) []string {
	t.Helper()
	var out []string
	for _, f := range g.srv.ReadQueued(t, g.players[i].c) {
		if f[0] != serverpackets.OpcodeExtended {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadUint16() != serverpackets.OpcodeExShowScreenMessage {
			continue
		}
		fields := make([]int32, 10)
		for j := range fields {
			fields[j] = r.ReadInt32()
		}
		if fields[0] != 1 || fields[1] != -1 || fields[2] != 2 || fields[8] != 10000 {
			t.Fatalf("on-screen message fields = %v, want a top-center custom text for 10 s", fields)
		}
		out = append(out, r.ReadString())
	}
	return out
}

// expectScreen checks that every seat was shown exactly want.
func (g *group) expectScreen(t *testing.T, want ...string) {
	t.Helper()
	for i, p := range g.players {
		got := g.screenTexts(t, i)
		if len(got) != len(want) {
			t.Fatalf("%s was shown %q, want %q", p.name, got, want)
		}
		for j := range want {
			if got[j] != want[j] {
				t.Fatalf("%s was shown %q, want %q", p.name, got, want)
			}
		}
	}
}

// dropOwner returns the player the boss's adena drop is reserved to.
func (g *group) dropOwner(t *testing.T) int32 {
	t.Helper()
	g.srv.Settle(t)
	drops := g.srv.GroundItems.Snapshots(nil)
	if len(drops) != 1 || drops[0].TemplateID != item.AdenaID || drops[0].Count != ccLootAdena {
		t.Fatalf("ground drops = %+v, want the boss's %d adena", drops, ccLootAdena)
	}
	return drops[0].OwnerID
}

// TestRaidChannelRightsTakeDrops gives the loot rights to the first
// channel larger than the regular threshold of 18 to hit a raid boss: every
// observer is shown its leader's rights, and the drops go to that leader
// though Rival, outside the channel, dealt most of the damage and the
// killing blow, and Leader never hit it.
func TestRaidChannelRightsTakeDrops(t *testing.T) {
	g, channels := bootCCLoot(t)
	channels.join(g.character(t, ccLeader), 19, g.players[ccLeader].id, g.players[ccMember].id)
	boss := g.spawnBesideLeader(t, ccLootBoss(raidBossID))

	boss.TakeDamage(100, g.combatant(t, ccMember))
	g.expectScreen(t, regularRightsText)
	boss.TakeDamage(500, g.combatant(t, ccRival))
	if !boss.TakeDamage(1_000_000, g.combatant(t, ccRival)) {
		t.Fatal("Rival's hit did not kill the boss")
	}
	if owner := g.dropOwner(t); owner != g.players[ccLeader].id {
		t.Fatalf("drop reserved to %d, want the channel leader %d", owner, g.players[ccLeader].id)
	}
}

// TestRaidChannelAtThresholdGetsNoRights leaves a channel no larger than
// the boss's threshold without rights: 18 members on a regular raid boss,
// then 36 on Queen Ant, whose threshold is 36, show nothing and leave the
// drops to the top damage dealer; 37 on Queen Ant win them, announced with
// Queen Ant's own text.
func TestRaidChannelAtThresholdGetsNoRights(t *testing.T) {
	g, channels := bootCCLoot(t)
	channel := channels.join(g.character(t, ccLeader), 18, g.players[ccLeader].id, g.players[ccMember].id)
	boss := g.spawnBesideLeader(t, ccLootBoss(raidBossID))

	boss.TakeDamage(100, g.combatant(t, ccMember))
	boss.TakeDamage(500, g.combatant(t, ccRival))
	if !boss.TakeDamage(1_000_000, g.combatant(t, ccRival)) {
		t.Fatal("Rival's hit did not kill the boss")
	}
	g.expectScreen(t)
	if owner := g.dropOwner(t); owner != g.players[ccRival].id {
		t.Fatalf("drop reserved to %d, want the top dealer %d", owner, g.players[ccRival].id)
	}

	channel.resize(36)
	queenAnt := g.spawnBesideLeader(t, ccLootBoss(29001))
	queenAnt.TakeDamage(100, g.combatant(t, ccMember))
	g.expectScreen(t)
	channel.resize(37)
	queenAnt.TakeDamage(100, g.combatant(t, ccMember))
	g.expectScreen(t, queenAntRightsText)
}

// TestRaidChannelRightsLapse drops the rights once their channel goes five
// minutes without a hit: a hit by the holding channel restarts the five
// minutes, a second qualifying channel neither takes nor refreshes them,
// and the boss announces the lapse to every observer on the first second
// past the five minutes. The drops then go to the top dealer again.
func TestRaidChannelRightsLapse(t *testing.T) {
	g, channels := bootCCLoot(t)
	channels.join(g.character(t, ccLeader), 19, g.players[ccLeader].id, g.players[ccMember].id)
	channels.join(g.character(t, ccOther), 19, g.players[ccOther].id)
	boss := g.spawnBesideLeader(t, ccLootBoss(raidBossID))

	boss.TakeDamage(10, g.combatant(t, ccMember))
	g.expectScreen(t, regularRightsText)
	g.srv.Advance(t, 200*time.Second)
	boss.TakeDamage(10, g.combatant(t, ccMember))
	boss.TakeDamage(10, g.combatant(t, ccOther))
	g.expectScreen(t)

	g.srv.Advance(t, 250*time.Second)
	boss.TakeDamage(10, g.combatant(t, ccOther))
	g.srv.Advance(t, 50*time.Second)
	g.expectScreen(t)
	g.srv.Advance(t, time.Second)
	g.expectScreen(t, regularNoRightsText)

	boss.TakeDamage(500, g.combatant(t, ccRival))
	if !boss.TakeDamage(1_000_000, g.combatant(t, ccRival)) {
		t.Fatal("Rival's hit did not kill the boss")
	}
	if owner := g.dropOwner(t); owner != g.players[ccRival].id {
		t.Fatalf("drop reserved to %d, want the top dealer %d", owner, g.players[ccRival].id)
	}
	g.srv.Advance(t, 10*time.Minute)
	g.expectScreen(t)
}
