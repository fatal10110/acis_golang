package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	npcCostSkill = modelskill.ID(9101)
	npcHPCost    = 50
)

// bootNPCCostCaster brings a player in next to a monster wired with the
// production AI-cast seam, whose only skill costs npcHPCost HP.
func bootNPCCostCaster(t *testing.T) (*gameservertest.Server, *npc.Hostile, func()) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	startInWorld(t, srv.Client)
	defs := modelskill.NewTable([]modelskill.Definition{{
		ID: npcCostSkill, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetSelf, SkillType: "BUFF",
		HitTime: 500, StaticHitTime: true, StaticReuse: true, HPConsume: npcHPCost,
	}})
	hostile, aiCtl := srv.SpawnCastingHostileNPC(t, &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}, defs)
	drainUntilQuiet(t, srv.Client)
	cast := func() {
		t.Helper()
		if !hostile.Queue().Post(func() { aiCtl.Cast(hostile, modelskill.Ref{ID: npcCostSkill, Level: 1}) }) {
			t.Fatal("post npc cast: queue closed")
		}
	}
	return srv, hostile, cast
}

// onNPCQueue runs fn as a task on the monster's own queue and waits for it.
func onNPCQueue(t *testing.T, hostile *npc.Hostile, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !hostile.Queue().Post(func() { fn(); close(done) }) {
		t.Fatal("post to npc queue: queue closed")
	}
	<-done
}

// TestNPCHPCostDoesNotWakeOrAggroCaster pins that a monster paying its own
// skill's HP cost treats it as consumption, not a hit: a sleep on it
// survives the payment, and the monster does not enter its own threat
// table.
func TestNPCHPCostDoesNotWakeOrAggroCaster(t *testing.T) {
	t.Parallel()
	_, hostile, cast := bootNPCCostCaster(t)
	onNPCQueue(t, hostile, func() {
		hostile.EffectList().Add(&effect.Effect{
			Skill:    effect.Skill{ID: 1},
			Template: modelskill.EffectTemplate{Name: "sleep"},
			Type:     effect.TypeSleep,
		})
	})

	cast()
	full := float64(hostile.MaxHP())
	waitFor(t, "the monster paying its HP cost", func() bool { return hostile.HP() == full-npcHPCost })

	var asleep bool
	onNPCQueue(t, hostile, func() {
		for _, e := range hostile.EffectList().All() {
			asleep = asleep || e.Type == effect.TypeSleep
		}
	})
	if !asleep {
		t.Fatal("the monster's own HP cost woke it from sleep")
	}
	if threats := hostile.AI().Threats().Snapshot(); len(threats) != 0 {
		t.Fatalf("threat table after the monster's own HP cost = %d entries, want none", len(threats))
	}
}

// TestInvulNPCPaysLethalHPCostAndDies pins that invulnerability does not
// cover a monster's own skill cost: the cost is paid, it empties the HP,
// and the death sequence runs and reaches observers. A cast only starts
// with HP above its cost, so the monster loses the difference and turns
// invulnerable while the cast is in flight.
func TestInvulNPCPaysLethalHPCostAndDies(t *testing.T) {
	t.Parallel()
	srv, hostile, cast := bootNPCCostCaster(t)
	onNPCQueue(t, hostile, func() { hostile.SetHP(npcHPCost + 1) })
	drainUntilQuiet(t, srv.Client)

	cast()
	onNPCQueue(t, hostile, func() {
		hostile.SetHP(npcHPCost)
		hostile.Live.SetInvul(true)
	})
	waitFor(t, "the invulnerable monster dying from its own HP cost", hostile.Dead)
	if hp := hostile.HP(); hp != 0 {
		t.Fatalf("monster HP after an exactly-lethal cost = %v, want 0", hp)
	}
	if !readsOpcode(t, srv.Client, serverpackets.OpcodeDie) {
		t.Fatal("no Die broadcast after the monster's exactly-lethal HP cost")
	}
}

// TestNPCLethalHPCostReplacesPlayerOverhit pins the overhit record a lethal
// self-cost leaves behind. The monster is its own attacker when it pays the
// cost, so the overhit check stores the monster itself over the player who
// armed it, and that player no longer qualifies for the overhit bonus.
func TestNPCLethalHPCostReplacesPlayerOverhit(t *testing.T) {
	t.Parallel()
	srv, hostile, cast := bootNPCCostCaster(t)
	obj, ok := srv.State.Player(srv.SoleObjectID(t))
	if !ok {
		t.Fatal("player missing from world")
	}
	player, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("world player %T is not an online character", obj)
	}
	onNPCQueue(t, hostile, func() { hostile.SetHP(npcHPCost + 1) })

	// Mid-cast, the monster drops to exactly its cost and turns
	// invulnerable; an overhit strike that would kill then lands on it,
	// recording the player as the overhit attacker but dealing no damage.
	cast()
	onNPCQueue(t, hostile, func() {
		hostile.SetHP(npcHPCost)
		hostile.Live.SetInvul(true)
		hostile.EnableOverhit()
		hostile.ReduceHP(npcHPCost*2, player, modelskill.Definition{})
	})
	if !hostile.OverhitValid(player) {
		t.Fatal("setup: the player's overhit strike was not recorded")
	}

	waitFor(t, "the monster dying from its own HP cost", hostile.Dead)
	if hostile.OverhitValid(player) {
		t.Fatal("player still holds the overhit after the monster's own lethal cost replaced it")
	}
}
