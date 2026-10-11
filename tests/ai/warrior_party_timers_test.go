package ai

import (
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// positionOf returns where h stands.
func positionOf(h *npc.Hostile) location.Location {
	x, y, z := h.Position()
	return location.Location{X: x, Y: y, Z: z}
}

// TestPartyPrivateOfDeadLeaderStaysWhileAttacking pins the 1006 check's
// keep branch: a private whose leader is dead stays in the world at the
// 20 s check while it is attacking, and leaves at the next check once it
// has stopped.
func TestPartyPrivateOfDeadLeaderStaysWhileAttacking(t *testing.T) {
	t.Parallel()
	w := bootParty(t, nil, leaderTemplate(leaderID, nil, 1), warriorTemplate(partyPrivateID, nil))
	leader := w.spawn(t, leaderID, 400, 0)
	private := privatesOf(t, leader, 1)[0]

	w.kill(t, leader)
	if private.Master() != nil {
		t.Fatal("the private still has its dead leader as master")
	}
	w.hit(t, private, 10)
	w.ticks(t, 2)
	if got := private.AI().CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("hit masterless private's intention = %v, want attack", got)
	}
	w.srv.Advance(t, 20*time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	if !w.inWorld(private) {
		t.Fatal("the 20 s check hid a masterless private that was attacking")
	}

	w.onQueue(t, private, func() {
		script.NPCOf(private).RemoveAllAttackDesire()
		private.AI().Desires().Clear()
	})
	w.ticks(t, 2)
	if got := private.AI().CurrentIntention(); got == actorai.IntentionAttack || got == actorai.IntentionCast {
		t.Fatalf("masterless private's intention once it stopped = %v, want neither attack nor cast", got)
	}
	w.srv.Advance(t, 20*time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	if w.inWorld(private) {
		t.Fatal("the next 20 s check kept a masterless private that had stopped attacking")
	}
}

// TestPartyTimersBringLeaderAndPrivateBackIntoTerritory pins the two 120 s
// checks. The summoning leader creates its private on a hit 60 s after it
// spawns, so the private's 1005 check runs 60 s apart from the leader's
// 1007 check. With its leader moved out of its territory, a private that
// is not attacking is teleported to its live leader, its hate dropped; the
// leader's own check, finding it out of its territory and not attacking,
// teleports it back to its spawn location, its hate dropped. A check that
// finds the leader in its territory moves nothing.
func TestPartyTimersBringLeaderAndPrivateBackIntoTerritory(t *testing.T) {
	t.Parallel()
	// Neither walks: only the checks' teleports move them.
	leaderTmpl := leaderTemplate(summonerLeaderID, npc.AIParams{"SummonPrivateRate": "100"}, 1)
	privateTmpl := warriorTemplate(partyPrivateID, nil)
	leaderTmpl.CanMove, privateTmpl.CanMove = false, false
	w := bootParty(t, nil, leaderTmpl, privateTmpl)
	leader := w.spawn(t, summonerLeaderID, 400, 0)
	home := positionOf(leader)

	w.srv.Advance(t, 60*time.Second)
	w.hit(t, leader, 10)
	w.srv.ReadQueued(t, w.srv.Client)
	private := privatesOf(t, leader, 1)[0]
	// The hit set both on the character. Drop their attack desires but not
	// their hate, and let an AI turn pick something else: the checks below
	// drop the hate.
	for _, h := range []*npc.Hostile{leader, private} {
		w.onQueue(t, h, func() { h.AI().Desires().Clear() })
	}
	w.ticks(t, 2)
	w.srv.ReadQueued(t, w.srv.Client)
	for _, h := range []*npc.Hostile{leader, private} {
		if got := h.AI().CurrentIntention(); got == actorai.IntentionAttack {
			t.Fatalf("npc %d intention = attack once its attack desires were dropped", h.NpcID())
		}
		if h.AggroHate(w.player) == 0 {
			t.Fatalf("npc %d hates nobody after the hit", h.NpcID())
		}
	}
	placed := positionOf(private)

	// 120 s: the leader's check finds it at home.
	w.srv.Advance(t, 60*time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	if got := positionOf(leader); got != home {
		t.Fatalf("leader in its territory moved to %v by its 120 s check, want %v", got, home)
	}

	w.teleport(t, leader, 1000, 0)
	away := positionOf(leader)

	// 180 s: the private's check, its leader out of its territory.
	w.srv.Advance(t, 60*time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	if got := positionOf(private); got == placed || got.Distance2D(away) > 1 {
		t.Fatalf("private at %v after its 120 s check, want teleported to its leader at %v", got, away)
	}
	if got := private.AggroHate(w.player); got != 0 {
		t.Fatalf("teleported private's hate = %v, want its attack desires dropped", got)
	}
	if got := positionOf(leader); got != away || leader.AggroHate(w.player) == 0 {
		t.Fatalf("leader at %v hating %v before its own check, want still at %v with its hate", got, leader.AggroHate(w.player), away)
	}

	// 240 s: the leader's check, out of its territory.
	w.srv.Advance(t, 60*time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	if got := positionOf(leader); got.Distance2D(home) > 1 {
		t.Fatalf("leader at %v after its 120 s check out of its territory, want back at its spawn location %v", got, home)
	}
	if got := leader.AggroHate(w.player); got != 0 {
		t.Fatalf("leader teleported home hate = %v, want its attack desires dropped", got)
	}
}
