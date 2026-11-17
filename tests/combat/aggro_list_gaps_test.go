package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const aggroListMonsterID = 31912

// aggroListWorld boots two players, a (the founder) and b standing 100
// east of it, and spawns through the server's NPC population a monster of
// aggroRange 50 east of a, with no hate against anyone. It returns the
// server, the monster and both players.
func aggroListWorld(t *testing.T, aggroRange int) (*gameservertest.Server, *npc.Hostile, attackable.Combatant, attackable.Combatant) {
	t.Helper()
	tmpl := &npc.Template{
		ID: aggroListMonsterID, TemplateID: aggroListMonsterID, Type: "Monster", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20, AggroRange: aggroRange,
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Alpha", 10, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{tmpl})),
		gameservertest.WithNpcSpawns(nil),
	)
	c, aID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	bID := srv.SeedCharacterFor(t, "second", "Beta", 10, 0).ID
	bc := srv.DialClient(t, "second", 1)
	startInWorld(t, bc)
	combatant := func(id int32) attackable.Combatant {
		obj, ok := srv.State.Player(id)
		if !ok {
			t.Fatalf("world.Player(%d) missing", id)
		}
		return obj.(attackable.Combatant)
	}
	x, y, z := srv.PlayerPosition(t, aID)
	obj, _ := srv.State.Player(bID)
	pc, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("%T is not an online character", obj)
	}
	pc.TeleportTo(x+100, y, z, 0)
	if err := srv.NpcSpawns.SpawnFixed(tmpl, x+50, y, z, 0, 0); err != nil {
		t.Fatalf("spawn the monster: %v", err)
	}
	var monster *npc.Hostile
	for _, obj := range srv.State.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && h.NpcID() == aggroListMonsterID {
			monster = h
		}
	}
	if monster == nil {
		t.Fatal("monster not spawned")
	}
	drainUntilQuiet(t, bc)
	drainUntilQuiet(t, c)
	return srv, monster, combatant(aID), combatant(bID)
}

// attackDesireWeights returns the queued attack desires' weights by target.
func attackDesireWeights(t *testing.T, monster *npc.Hostile) map[int32]float64 {
	t.Helper()
	out := map[int32]float64{}
	onQueue(t, monster.Queue(), func() {
		for _, d := range monster.AI().Desires().Snapshot() {
			if d.Kind == ai.IntentionAttack && d.FinalTarget != nil {
				out[d.FinalTarget.ObjectID()] = d.Weight
			}
		}
	})
	return out
}

// TestRandomizeHateRebuildsEveryAttackDesireFromTheHateList pins what attack
// randomization leaves queued (AggroList.randomizeAttack): the hate swap
// raises the chosen attacker over the most hated by 200, then every queued
// attack desire is dropped, whatever it aimed at, and one is queued per
// hate-list entry, a hate of 0 included, weighing the entry's hate after
// the swap. The chosen attacker's desire then outweighs the rest.
func TestRandomizeHateRebuildsEveryAttackDesireFromTheHateList(t *testing.T) {
	t.Parallel()
	srv, monster, hated, bystander := aggroListWorld(t, 0)
	// A third attacker the monster has lost all hate for: its entry stays.
	forgotten := srv.SpawnHostileNPCTemplateAt(t, gameservertest.MovingHostileTemplate("Monster"),
		location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	onQueue(t, monster.Queue(), func() {
		monster.AddDamageHate(hated, 0, 1000)
		monster.AddDamageHate(bystander, 0, 500)
		monster.AddDamageHate(forgotten, 5, 0)
		// Stale desires: one at a weight the hate no longer matches, one
		// aimed at nothing on the hate list.
		monster.AI().Desires().AddOrUpdate(&ai.Desire{Kind: ai.IntentionAttack, FinalTarget: hated, Weight: 7})
		monster.AI().Desires().AddOrUpdate(&ai.Desire{Kind: ai.IntentionAttack, FinalTarget: monster, Weight: 9999})
	})

	if got := randomizeHate(t, monster); got != bystander.ObjectID() {
		t.Fatalf("most hated after the randomization = %d, want the bystander %d", got, bystander.ObjectID())
	}
	want := map[int32]float64{hated.ObjectID(): 1000, bystander.ObjectID(): 1200, forgotten.ObjectID(): 0}
	got := attackDesireWeights(t, monster)
	if len(got) != len(want) {
		t.Fatalf("queued attack desires = %v, want one per hate entry %v", got, want)
	}
	for id, weight := range want {
		if w, ok := got[id]; !ok || w != weight {
			t.Fatalf("attack desire of %d = %v (queued %v), want weight %v; all %v", id, w, ok, weight, got)
		}
	}
	var top *ai.Desire
	onQueue(t, monster.Queue(), func() { top, _ = monster.AI().Desires().Peek() })
	if top == nil || top.FinalTarget.ObjectID() != bystander.ObjectID() {
		t.Fatalf("heaviest queued desire = %+v, want the attack on the bystander", top)
	}
}

// TestReconsiderTargetFallsBackToTheKnownList pins AggroList.reconsiderTarget
// when the hate list has nothing to give: an aggressive monster that is not a
// siege guard takes the first creature it knows inside its aggro range that
// it may auto-attack (and, for a positive range, inside that range too) and
// notes it with 1 hate and no damage, as if it had just walked in; a
// monster with no aggro range, or nothing in sight, takes none. With a hate
// list of two or more it swaps inside the list and does not look further.
func TestReconsiderTargetFallsBackToTheKnownList(t *testing.T) {
	t.Parallel()
	reconsider := func(t *testing.T, monster *npc.Hostile, rangeVal int) (attackable.Combatant, bool) {
		t.Helper()
		var target attackable.Combatant
		var ok bool
		onQueue(t, monster.Queue(), func() { target, ok = monster.ReconsiderTarget(rangeVal) })
		return target, ok
	}
	threat := func(t *testing.T, monster *npc.Hostile, who attackable.Combatant) (hate, damage float64, listed bool) {
		t.Helper()
		onQueue(t, monster.Queue(), func() {
			if e, found := monster.AI().Threats().Get(who); found {
				hate, damage, listed = e.Hate, e.Damage, true
			}
		})
		return hate, damage, listed
	}

	t.Run("empty hate list takes a known creature with 1 hate", func(t *testing.T) {
		t.Parallel()
		_, monster, a, b := aggroListWorld(t, 400)
		got, ok := reconsider(t, monster, 0)
		if !ok || (got.ObjectID() != a.ObjectID() && got.ObjectID() != b.ObjectID()) {
			t.Fatalf("ReconsiderTarget(0) = %v, %v, want one of the two players in sight", got, ok)
		}
		if hate, damage, listed := threat(t, monster, got); !listed || hate != 1 || damage != 0 {
			t.Fatalf("hate against the chosen = %v, damage %v, listed %v; want 1, 0, true", hate, damage, listed)
		}
		if n := len(attackDesireWeights(t, monster)); n != 0 {
			t.Fatalf("%d attack desires queued by the fallback, want none", n)
		}
	})
	t.Run("a positive range limits the known-list pick", func(t *testing.T) {
		t.Parallel()
		_, monster, a, b := aggroListWorld(t, 400)
		// a stands 50 from the monster, b 50 on the other side of a.
		got, ok := reconsider(t, monster, 60)
		if !ok || got.ObjectID() != a.ObjectID() {
			t.Fatalf("ReconsiderTarget(60) = %v, %v, want the player 50 away", got, ok)
		}
		if _, _, listed := threat(t, monster, b); listed {
			t.Fatal("the player outside the range was noted")
		}
		if got, ok := reconsider(t, monster, 10); ok {
			t.Fatalf("ReconsiderTarget(10) = %v, want none: no one within 10", got)
		}
	})
	t.Run("a monster with no aggro range takes none", func(t *testing.T) {
		t.Parallel()
		_, monster, a, b := aggroListWorld(t, 0)
		if got, ok := reconsider(t, monster, 0); ok {
			t.Fatalf("ReconsiderTarget(0) = %v, want none", got)
		}
		for _, who := range []attackable.Combatant{a, b} {
			if _, _, listed := threat(t, monster, who); listed {
				t.Fatalf("player %d was noted", who.ObjectID())
			}
		}
	})
	t.Run("a hate list swaps inside the list", func(t *testing.T) {
		t.Parallel()
		_, monster, a, b := aggroListWorld(t, 400)
		onQueue(t, monster.Queue(), func() {
			monster.AddDamageHate(a, 0, 1000)
			monster.AddDamageHate(b, 0, 500)
			monster.AI().Desires().AddOrUpdate(&ai.Desire{Kind: ai.IntentionAttack, FinalTarget: a, Weight: 1000})
		})
		got, ok := reconsider(t, monster, 0)
		if !ok || got.ObjectID() != b.ObjectID() {
			t.Fatalf("ReconsiderTarget(0) = %v, %v, want the less hated player", got, ok)
		}
		// The old most hated's hate is stopped before the chosen is given
		// it, so the chosen gains nothing.
		if hate, _, _ := threat(t, monster, a); hate != 0 {
			t.Fatalf("old most hated's hate = %v, want 0", hate)
		}
		if hate, _, _ := threat(t, monster, b); hate != 500 {
			t.Fatalf("chosen's hate = %v, want its own 500", hate)
		}
		if w := attackDesireWeights(t, monster); len(w) != 0 {
			t.Fatalf("attack desires left = %v, want the old most hated's dropped", w)
		}
	})
}
