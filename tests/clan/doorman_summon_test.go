package clan

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/wizard"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The gatekeeper (DoormanOfDrain) and the harmless spell it casts at a
// summon.
const (
	drainDoormanID = 22136
	drainProbeID   = modelskill.ID(9395)
)

// drainSummonWorld is the siege summon gate world with a gatekeeper beside
// the founder, bound to DoormanOfDrain, and the founder's Siege Golem.
func drainSummonWorld(t *testing.T) (*clanWorld, *npc.Hostile, attackable.Combatant, *summon.Actor) {
	t.Helper()
	form, err := zone.NewCuboid(gateWorldAt.X-1000, gateWorldAt.X+1000, gateWorldAt.Y-1000, gateWorldAt.Y+1000,
		gateWorldAt.Z-1000, gateWorldAt.Z+1000)
	if err != nil {
		t.Fatal(err)
	}
	field := zone.NewSiege(90001, form)
	field.SetActive(true)
	zones := zone.NewIndex()
	zones.Add(field)
	const path = "script.ai.individual.Monster.WizardBase.Wizard.DoormanOfDrain"
	w := gateWorld(t,
		gameservertest.WithZones(zones),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{siegeSummonTemplate(gateGolemNPCID, "Siege Golem"), {
			ID: drainDoormanID, TemplateID: drainDoormanID, Type: "Monster", Name: "Gatekeeper Zombie", Level: 40, HPMax: 1000,
			AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: true, CollisionRadius: 8, CollisionHeight: 20,
			AggroRange: 1000,
		}})),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithAITask(),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{drainDoormanID: script.KindHostile},
			[]script.Listing{{Path: path}}, script.Catalog{path: wizard.DoormanOfDrain}),
	)
	golem := callSiegeSummon(t, w, w.leaderID, gateGolemNPCID)
	n, err := w.srv.NpcSpawns.AddSpawn(drainDoormanID, gateWorldAt.X+100, gateWorldAt.Y, gateWorldAt.Z, 0, false, 0)
	if err != nil {
		t.Fatalf("spawn the gatekeeper: %v", err)
	}
	w.srv.Settle(t)
	h := n.(*npc.Hostile)
	obj, ok := w.srv.State.Object(w.leaderID)
	if !ok {
		t.Fatal("the founder is not in the world")
	}
	return w, h, obj.(attackable.Combatant), golem
}

// untilTicking runs AI task cycles, a second apart, until done holds.
func untilTicking(t *testing.T, w *clanWorld, what string, done func() bool) {
	t.Helper()
	for i := 0; !done(); i++ {
		if i == 30 {
			t.Fatalf("%s not observed within 30 AI cycles", what)
		}
		if err := w.srv.AI.Tick(); err != nil {
			t.Fatalf("AI.Tick() error: %v", err)
		}
		w.srv.Advance(t, time.Second)
		w.srv.Settle(t)
	}
}

// onHostileQueue runs fn on h's queue and waits for it.
func onHostileQueue(t *testing.T, w *clanWorld, h *npc.Hostile, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !h.Queue().Post(func() { defer close(done); fn() }) {
		t.Fatal("the gatekeeper's queue is closed")
	}
	<-done
	w.srv.Settle(t)
}

// wantsOwner fails unless h, its spell having outlived the founder's
// summon, holds the
// founder on its hate list by 200 and, listing no DD magic, gave up its
// spells (int slot 0) and wants to attack the founder, weight 1000.
func wantsOwner(t *testing.T, h *npc.Hostile, founder attackable.Combatant) {
	t.Helper()
	if got := h.AI().Hates().Hate(founder); got != 200 {
		t.Fatalf("gatekeeper hate list entry of the summon's owner = %v, want 200", got)
	}
	if got := h.Scratch().Int(npc.IntAI0); got != 1 {
		t.Fatalf("gatekeeper int slot 0 = %d, want 1", got)
	}
	for _, d := range h.AI().Desires().Snapshot() {
		if d.FinalTarget != nil && d.FinalTarget.ObjectID() == founder.ObjectID() && d.Weight == 1000 {
			return
		}
	}
	t.Fatalf("gatekeeper desires = %+v, want an attack on the summon's owner with weight 1000", h.AI().Desires().Snapshot())
}

// TestDoormanOfDrainTurnsOnTheOwnerOfASummonItsSpellOutlives runs
// DoormanOfDrain's skill-finished hook: a cast at the founder's Siege Golem
// ending on the golem dead turns it on the founder. Its attack-finished
// branch shares the answer; a killing swing raises no attack-finished hook
// yet (#4114).
func TestDoormanOfDrainTurnsOnTheOwnerOfASummonItsSpellOutlives(t *testing.T) {
	t.Parallel()
	w, h, founder, golem := drainSummonWorld(t)
	w.srv.InstallNPCCastSeam(t, h, modelskill.NewTable([]modelskill.Definition{{
		ID: drainProbeID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		Offensive: true, Magic: true, SkillType: "DEBUFF", CastRange: 600, HitTime: 2000, StaticHitTime: true, StaticReuse: true,
	}}))
	onHostileQueue(t, w, h, func() {
		h.AI().Desires().Clear()
		h.AI().AddCastDesire(golem, modelskill.Ref{ID: drainProbeID, Level: 1}, 1e9, false, true)
	})
	untilTicking(t, w, "the gatekeeper's cast", h.CastingNow)
	golem.Queue().Post(func() { golem.ReduceHP(1e9, nil, modelskill.Definition{}) })
	untilTicking(t, w, "the end of the cast on the dead golem", func() bool { return golem.Dead() && !h.CastingNow() })
	wantsOwner(t, h, founder)
}
