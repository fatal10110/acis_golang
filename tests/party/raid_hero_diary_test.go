package party

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network"
)

// diaryEntries returns objectID's heroes_diary rows as action/param pairs.
func diaryEntries(t *testing.T, g *group, objectID int32) [][2]int {
	t.Helper()
	rows, err := g.srv.DB.Query("SELECT action, param FROM heroes_diary WHERE char_id = ? ORDER BY time", objectID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out [][2]int
	for rows.Next() {
		var e [2]int
		if err := rows.Scan(&e[0], &e[1]); err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// makeNoble gives player i noblesse status.
func (g *group) makeNoble(t *testing.T, i int) {
	t.Helper()
	obj, _ := g.srv.State.Player(g.players[i].id)
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("%s is %T", g.players[i].name, obj)
	}
	c.SetNoble(true)
}

// TestRaidBossKillGoesToNoblesHeroDiaries pins the hero diary entry of a
// raid boss kill: every noble credited with the kill, a party member or
// the partyless killer, gets a "raid boss defeated" entry (action 1)
// naming the boss; anyone else none.
func TestRaidBossKillGoesToNoblesHeroDiaries(t *testing.T) {
	t.Run("party", func(t *testing.T) {
		g := bootGroup(t, []seat{{"Leader", 40}, {"Noble", 40}})
		g.invite(t, 0, 1, 0)
		g.makeNoble(t, 1)
		boss := g.spawnBesideLeader(t, raidBoss(raidBossID))
		if !boss.TakeDamage(5000, g.combatant(t, 0)) {
			t.Fatal("the leader's hit did not kill the boss")
		}
		g.quiet(t)
		g.srv.FlushPersistence(t)
		if got := diaryEntries(t, g, g.players[1].id); len(got) != 1 || got[0] != [2]int{1, raidBossID} {
			t.Fatalf("noble member's diary = %v, want one raid entry for %d", got, raidBossID)
		}
		if got := diaryEntries(t, g, g.players[0].id); len(got) != 0 {
			t.Fatalf("leader's diary = %v, want none", got)
		}
	})
	t.Run("partyless", func(t *testing.T) {
		g := bootGroup(t, []seat{{"Solo", 40}})
		g.makeNoble(t, 0)
		boss := g.spawnBesideLeader(t, raidBoss(raidBossID))
		if !boss.TakeDamage(5000, g.combatant(t, 0)) {
			t.Fatal("the hit did not kill the boss")
		}
		g.quiet(t)
		g.srv.FlushPersistence(t)
		if got := diaryEntries(t, g, g.players[0].id); len(got) != 1 || got[0] != [2]int{1, raidBossID} {
			t.Fatalf("noble killer's diary = %v, want one raid entry for %d", got, raidBossID)
		}
	})
}
