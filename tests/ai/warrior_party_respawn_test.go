package ai

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/rs/zerolog"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// summonDen is an npcmaker spawning one summoning party leader, which
// respawns, and declares no privates of its own: the leader's template
// lists them.
const summonDen = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="den" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="summon_den" territory="den" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="27062" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`

// summonDenKey is the slot key of the den's leader.
const summonDenKey = "summon_den#0#0"

// TestRespawnedSummonerLeaderStaysAMaster runs a party leader with a
// SummonPrivateRate of 50 through a respawn. In its first life a hit,
// drawing 0, summons its private. It dies, letting the private go, and
// its spawn brings it back: the same NPC to the reference, which keeps its
// privates on the respawned object, so the new life is still a master and
// still lists the private. A hit on it that draws 99 summons nothing, yet
// reaches its party: the leader adds the party formula for itself,
// damage / (level + 7) * damage * its weight point 10 * 10, on top of the
// Warrior formula, and the old private, alive and masterless, its own
// party formula.
func TestRespawnedSummonerLeaderStaysAMaster(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "den.xml"), []byte(summonDen), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	var draw atomic.Int32
	w := bootPartyWith(t, table, []gameservertest.Option{gameservertest.WithScriptRand(func(int) int { return int(draw.Load()) })},
		leaderTemplate(summonerLeaderID, npc.AIParams{"SummonPrivateRate": "50"}, 1), warriorTemplate(partyPrivateID, nil))
	leaders := w.liveNPCs(summonerLeaderID)
	if len(leaders) != 1 {
		t.Fatalf("den leaders = %d, want 1", len(leaders))
	}
	first := leaders[0]

	w.hit(t, first, 10)
	w.srv.ReadQueued(t, w.srv.Client)
	private := privatesOf(t, first, 1)[0]
	w.kill(t, first)
	if private.Master() != nil {
		t.Fatal("the dead leader kept its private's master link")
	}
	w.onQueue(t, first, func() { first.Decay(w.srv.State, w.srv.NpcSpawns.RespawnHook(first.ObjectID())) })
	w.srv.NpcSpawns.Respawn(summonDenKey)
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)

	leaders = w.liveNPCs(summonerLeaderID)
	if len(leaders) != 1 || leaders[0] == first {
		t.Fatalf("den leaders after the respawn = %d, want one new life", len(leaders))
	}
	second := leaders[0]
	if !second.IsMaster() {
		t.Fatal("the respawned leader forgot it is a master")
	}
	if got := privatesOf(t, second, 1)[0]; got != private {
		t.Fatalf("respawned leader's private = %d, want the one it summoned, %d", got.ObjectID(), private.ObjectID())
	}

	draw.Store(99)
	privateBefore := private.AggroHate(w.player)
	w.hit(t, second, 10)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), second.ObjectID()); len(got) != 0 {
		t.Fatalf("leader lines on a hit drawing 99 = %q, want none: nothing is summoned", got)
	}
	privatesOf(t, second, 1)
	// Warrior: 10 / 8 * 100; party, leader for itself: int(10 / 8 * 10 * 10 * 10).
	if got := second.AggroHate(w.player); got != 125+1250 {
		t.Fatalf("respawned leader hate after a hit of 10 = %v, want 1375", got)
	}
	// Party, for the private: int(10 / 8 * 10 * 1 * 10).
	if got := private.AggroHate(w.player) - privateBefore; got != 125 {
		t.Fatalf("old private hate gained = %v, want 125", got)
	}
}
