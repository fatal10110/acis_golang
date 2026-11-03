package party

import (
	"testing"

	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestRaidChannelRightsOnSpawnedBoss is TestRaidChannelRightsTakeDrops for
// a raid boss the NPC population spawned: its rewards and the loot rights
// watching its hits are wired at its spawn, so the first hit already wins
// the channel its rights and the drops go to that channel's leader.
func TestRaidChannelRightsOnSpawnedBoss(t *testing.T) {
	t.Parallel()
	channels := &fakeChannels{byPlayer: map[int32]*fakeChannel{}}
	g := bootGroup(t, []seat{{"Leader", 40}, {"Member", 40}, {"Rival", 40}, {"Other", 40}},
		gameservertest.WithLootChannels(func(gamemanager.LootChannels) gamemanager.LootChannels { return channels }),
		gameservertest.WithNpcDropRates(item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1}))
	channels.join(g.character(t, ccLeader), 19, g.players[ccLeader].id, g.players[ccMember].id)
	x, y, z := g.srv.PlayerPosition(t, g.players[0].id)
	boss, err := g.srv.NpcSpawns.SpawnFixedHostile(ccLootBoss(raidBossID), x+60, y+20, z, 0)
	if err != nil {
		t.Fatalf("spawn the raid boss: %v", err)
	}
	g.quiet(t)

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
