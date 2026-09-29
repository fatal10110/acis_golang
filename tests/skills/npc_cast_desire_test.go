package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const npcDesiredSkill = modelskill.ID(9102)

var npcDesiredRef = modelskill.Ref{ID: npcDesiredSkill, Level: 1}

// bootNPCDesireCaster brings a player in next to a monster wired with the
// production AI-cast seam, whose only skill is a free two-second self buff
// with no reuse, and queues one CAST desire for it the way an AI script
// does. The monster's AI then casts it on its next think.
func bootNPCDesireCaster(t *testing.T) (*gameservertest.Server, *npc.Hostile) {
	t.Helper()
	return bootNPCDesireCasterWith(t, false)
}

// bootNPCDesireCasterWith is bootNPCDesireCaster whose self buff is a magic
// skill when magic is set.
func bootNPCDesireCasterWith(t *testing.T, magic bool) (*gameservertest.Server, *npc.Hostile) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	startInWorld(t, srv.Client)
	defs := modelskill.NewTable([]modelskill.Definition{{
		ID: npcDesiredSkill, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetSelf, SkillType: "BUFF", Magic: magic,
		HitTime: 2000, StaticHitTime: true, StaticReuse: true,
	}})
	hostile, _ := srv.SpawnCastingHostileNPC(t, &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}, defs)
	drainUntilQuiet(t, srv.Client)
	hostile.AI().Desires().AddOrUpdate(&ai.Desire{
		Kind: ai.IntentionCast, FinalTarget: hostile, Skill: npcDesiredRef,
		Weight: 1_000_000, QueuedAt: hostile.Now(),
	})
	return srv, hostile
}

// thinkOnNPCQueue runs one event-driven AI step on the monster's queue.
func thinkOnNPCQueue(t *testing.T, hostile *npc.Hostile) {
	t.Helper()
	onNPCQueue(t, hostile, func() {
		if err := hostile.Think(); err != nil {
			t.Errorf("Think() error: %v", err)
		}
	})
}

func hasCastDesire(hostile *npc.Hostile) bool {
	return hostile.AI().Desires().Has(&ai.Desire{Kind: ai.IntentionCast, FinalTarget: hostile, Skill: npcDesiredRef})
}

// assertNoRecast fails if a further think starts the desired skill again.
func assertNoRecast(t *testing.T, srv *gameservertest.Server, hostile *npc.Hostile) {
	t.Helper()
	thinkOnNPCQueue(t, hostile)
	for {
		frame := srv.Client.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return
		}
		if frame[0] == serverpackets.OpcodeMagicSkillUse {
			t.Fatal("the monster cast its skill again from a desire its last cast should have dropped")
		}
	}
}

// TestStunnedNPCCastDropsItsCastDesire pins NpcCast.notifyCastFinishToAI on
// an aborted cast: a monster stunned mid-cast drops the CAST desire that
// drove the cast, so once the stun ends it does not recast from it.
func TestStunnedNPCCastDropsItsCastDesire(t *testing.T) {
	t.Parallel()
	srv, hostile := bootNPCDesireCaster(t)
	c := srv.Client

	thinkOnNPCQueue(t, hostile)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillUse, "NPC MagicSkillUse")

	stun, err := effect.New(
		effect.Skill{ID: 101, Level: 1, Debuff: true},
		modelskill.EffectTemplate{Name: "Stun", Time: 30},
	)
	if err != nil {
		t.Fatalf("effect.New(Stun): %v", err)
	}
	stun.Effector, stun.Effected = hostile, hostile
	onNPCQueue(t, hostile, func() { hostile.EffectList().Add(stun) })
	assertFrameOpcode(t, readUntil(t, c, serverpackets.OpcodeMagicSkillCanceled), serverpackets.OpcodeMagicSkillCanceled, "NPC MagicSkillCanceled")

	if hasCastDesire(hostile) {
		t.Fatal("CAST desire still queued after the stun aborted its cast")
	}

	onNPCQueue(t, hostile, func() { hostile.EffectList().Remove(stun) })
	drainUntilQuiet(t, c)
	assertNoRecast(t, srv, hostile)
}

// TestCompletedNPCCastDropsItsCastDesire pins NpcCast.notifyCastFinishToAI
// on a completed cast: the CAST desire is dropped and the AI re-runs desire
// selection, leaving the monster idle instead of holding the finished cast
// as its intention.
func TestCompletedNPCCastDropsItsCastDesire(t *testing.T) {
	t.Parallel()
	srv, hostile := bootNPCDesireCaster(t)
	c := srv.Client

	thinkOnNPCQueue(t, hostile)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillUse, "NPC MagicSkillUse")
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionCast {
		t.Fatalf("CurrentIntention() mid-cast = %v, want %v", got, ai.IntentionCast)
	}

	srv.AdvanceUntil(t, "the monster's cast finishing", func() bool { return !hostile.AI().CastController().CastingNow() })

	if hasCastDesire(hostile) {
		t.Fatal("CAST desire still queued after its cast completed")
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("CurrentIntention() after the cast completed = %v, want %v", got, ai.IntentionIdle)
	}
	drainUntilQuiet(t, c)
	assertNoRecast(t, srv, hostile)
}

// readUntil reads frames until one has opcode want and returns it.
func readUntil(t *testing.T, c interface{ Read() []byte }, want byte) []byte {
	t.Helper()
	for i := 0; i < 50; i++ {
		if frame := c.Read(); frame[0] == want {
			return frame
		}
	}
	t.Fatalf("opcode %#x not read within 50 frames", want)
	return nil
}
