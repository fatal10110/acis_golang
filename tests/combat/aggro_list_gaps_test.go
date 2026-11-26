package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const aggroListMonsterID = 31912

// aggroListWorld boots two players, a (the founder) and b standing 150
// east of it (arrived through a completed teleport), and spawns through the
// server's NPC population a monster of the given aggro range 50 east of a,
// with no hate against anyone: a is 50 from the monster, b 100. Both are
// known to the monster. It returns the server, the monster and both players.
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
	pc.TeleportTo(x+150, y, z, 0)
	bc.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	drainUntilQuiet(t, bc)
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
	a, b := combatant(aID), combatant(bID)
	onQueue(t, monster.Queue(), func() {
		if !monster.Knows(a) || !monster.Knows(b) {
			t.Errorf("monster knows a=%v b=%v, want both", monster.Knows(a), monster.Knows(b))
		}
	})
	return srv, monster, a, b
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
