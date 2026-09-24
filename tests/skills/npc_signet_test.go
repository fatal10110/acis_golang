package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestNPCSignetSpawnsPointAtCasterAndExpiresThroughTheEffectTicker has a
// monster cast a plain SIGNET through the production AI-cast seam: the
// signet casts for any live caster, so the effect point must spawn at the
// monster's position — even for a ground-targeted signet, whose picked
// point only a player owns — and it must leave the world once its only
// effect expires through the server's effect ticker.
func TestNPCSignetSpawnsPointAtCasterAndExpiresThroughTheEffectTicker(t *testing.T) {
	t.Parallel()
	for name, target := range map[string]modelskill.Target{"self": modelskill.TargetSelf, "ground": modelskill.TargetGround} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			castNPCSignet(t, target)
		})
	}
}

func castNPCSignet(t *testing.T, target modelskill.Target) {
	def := modelskill.Definition{
		ID: 454, Level: 1, Activation: modelskill.ActivationActive, Target: target, CastRange: 900,
		HitTime: 500, StaticHitTime: true, StaticReuse: true,
		SkillType: "SIGNET", EffectNpcID: 13018, Radius: 180,
		Effects: []modelskill.EffectTemplate{{Name: "Signet", Count: 1, Time: 1}},
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{{ID: 13018, Type: "EffectPoint", CollisionRadius: 8}})),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
	)
	startInWorld(t, srv.Client)
	hostile, aiCtl := srv.SpawnCastingHostileNPC(t, &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}, modelskill.NewTable([]modelskill.Definition{def}))
	drainUntilQuiet(t, srv.Client)

	if !hostile.Queue().Post(func() { aiCtl.Cast(hostile, modelskill.Ref{ID: 454, Level: 1}) }) {
		t.Fatal("post npc cast: queue closed")
	}

	var point *npc.EffectPoint
	srv.AdvanceUntil(t, "npc signet effect point", func() bool {
		for _, obj := range srv.State.Objects() {
			if ep, ok := obj.(*npc.EffectPoint); ok {
				point = ep
			}
		}
		return point != nil
	})
	if got := point.OwnerID(); got != hostile.ObjectID() {
		t.Fatalf("effect point owner = %d, want the casting monster %d", got, hostile.ObjectID())
	}
	px, py, pz := point.Position()
	hx, hy, hz := hostile.Position()
	if px != hx || py != hy || pz != hz {
		t.Fatalf("effect point at (%d,%d,%d), want the monster's position (%d,%d,%d)", px, py, pz, hx, hy, hz)
	}

	srv.Advance(t, 1100*time.Millisecond)
	srv.TickEffects()
	if _, ok := srv.State.Object(point.ObjectID()); ok {
		t.Fatal("npc signet point still in world after its only tick: its list never reached the effect ticker")
	}
}
