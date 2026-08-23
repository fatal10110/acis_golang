package cast

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// castHostileGeo/castHostileMove/castHostileAttack are this package's own
// copies of the inert movement/attack fakes npc's hostile_test.go keeps
// unexported — a mover that always reports "already in range" and an
// attacker that never fires, so thinkCast's pre-cast movement gate falls
// straight through to the cast attempt.
type castHostileGeo struct{}

func (castHostileGeo) CanMove(_, _, _, _, _, _ int) bool { return true }
func (castHostileGeo) Height(_, _, _ int) int16          { return 0 }
func (castHostileGeo) FindPath(_, _ location.Location) ([]location.Location, bool) {
	return nil, false
}
func (castHostileGeo) Walkable(int, int, int) bool { return true }
func (castHostileGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

type castHostileMove struct{}

func (castHostileMove) MaybeStartOffensiveFollow(attackable.Combatant, int) (bool, error) {
	return false, nil
}
func (castHostileMove) MoveHome(location.Location) error { return nil }
func (castHostileMove) Stop() error                      { return nil }

type castHostileAttack struct{}

func (castHostileAttack) BowCoolingDown() bool                { return false }
func (castHostileAttack) AttackingNow() bool                  { return false }
func (castHostileAttack) CanAttack(attackable.Combatant) bool { return false }
func (castHostileAttack) DoAttack(attackable.Combatant) error { return nil }

// castCombatant is the minimal attackable.Combatant/skilltarget.Creature/Target
// a hostile's queued IntentionCast desire can be aimed at.
type castCombatant struct {
	world.Presence
	id     int32
	frames [][]byte
}

func (t *castCombatant) ObjectID() int32                                    { return t.id }
func (t *castCombatant) Dead() bool                                         { return false }
func (t *castCombatant) AlikeDead() bool                                    { return false }
func (t *castCombatant) SiegeGuard() bool                                   { return false }
func (t *castCombatant) Category() skilltarget.Category                     { return skilltarget.CategoryAttackable }
func (t *castCombatant) AttackableBy(skilltarget.Creature) bool             { return true }
func (t *castCombatant) AttackableWithoutForceBy(skilltarget.Creature) bool { return true }

// BroadcastFrame records every packet t receives, so the test can prove the
// hostile-cast Start/Launch broadcasts (MagicSkillUse/MagicSkillLaunched)
// actually reach a known observer, matching npc's own
// TestHostileBroadcastSkillUseSendsFrameToKnownReceivers pattern.
func (t *castCombatant) BroadcastFrame(frame wire.Frame) bool {
	defer frame.Release()
	raw := frame.Bytes()
	payload := make([]byte, len(raw)-2)
	copy(payload, raw[2:])
	t.frames = append(t.frames, payload)
	return true
}

// WorldPlayer marks castCombatant as a world.Player, so spawning it activates
// its Region (world.Region.Active requires a nearby player) — the same real
// production gate task.AI's periodic Tick relies on to skip idle regions,
// which Hostile.Think()/canRunAI() also honors. Without an active region,
// Think() would run enterInactiveRegion()'s SetBackToPeace() and silently
// wipe the queued cast desire before ever reaching thinkCast.
func (t *castCombatant) WorldPlayer() {}

// TestHostileActorWiresIntoAIControllerAndCastsSkill4515 closes issue #1612:
// a live npc.Hostile had no cast.Actor adapter and no cast.AIController
// wired to its AI loop, so a ported hostile-AI script could never queue
// DefaultNpc's 4515/1 IntentionCast (DefaultNpc.java:199-202) — the runtime
// seam #185 depends on didn't exist. This wires a Controller/AIController
// the exact way manager.newLiveHostile does at spawn, queues the desire
// through the real ai.Attackable (not a fake CastController), and proves:
// the AI controller is live (Think() promotes the desire into an actual
// cast), MP is untouched until the Hit phase (NpcCast extends CreatureCast:
// spend MP, then call the skill at hit — NpcCast.java:22-54), and the
// existing skill-handler/packet hooks fire once the cast clock reaches Hit.
func TestHostileActorWiresIntoAIControllerAndCastsSkill4515(t *testing.T) {
	state := world.New()

	live, err := creature.NewLive(location.Location{}, 100, castHostileGeo{}, nil)
	if err != nil {
		t.Fatalf("creature.NewLive: %v", err)
	}
	hostile, err := npc.NewHostile(&npc.Instance{
		ObjectID: 101,
		Template: &npc.Template{ID: 18004, Type: "Monster", BaseAttackRange: 40, HPMax: 1000, MPMax: 200},
		Kind:     "Monster",
	}, live, castHostileMove{}, castHostileAttack{})
	if err != nil {
		t.Fatalf("NewHostile: %v", err)
	}
	hostile.SetWorld(state)
	hostile.SetFrameBuilder(serverpackets.NpcFrameBuilder{})
	state.Spawn(hostile, 0, 0, 0, 0)

	target := &castCombatant{id: 202}
	state.Spawn(target, 100, 100, 0, 0)

	ref := modelskill.Ref{ID: 4515, Level: 1}
	def := modelskill.Definition{
		ID: 4515, Level: 1, Magic: true, SkillType: "DUMMYCAST",
		Target: modelskill.TargetOne, CastRange: 600,
		StaticHitTime: true, StaticReuse: true,
		HitTime: 1500, CoolTime: 600, ReuseDelay: 12000, MPConsume: 10,
	}

	rec := &recordingSkillHandler{}
	clock := &fakeCastClock{}

	ctrl := NewController(HostileActor{Hostile: hostile})
	ctrl.afterFunc = clock.AfterFunc
	aiController := &AIController{
		Controller:  ctrl,
		Definitions: fakeDefinitions{ref: def},
		Effects:     newEffectHandlers(effectsKnown{}, "DUMMYCAST", rec),
		Caster:      hostile,
	}
	hostile.AI().SetCastController(aiController)

	beforeMP := int(hostile.MPValue())
	if beforeMP < def.MPConsume {
		t.Fatalf("test fixture MP %d too low to observe an MPConsume=%d reduction", beforeMP, def.MPConsume)
	}

	hostile.AI().Desires().AddOrUpdate(&ai.Desire{Kind: ai.IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})
	if err := hostile.Think(); err != nil {
		t.Fatalf("Think(): %v", err)
	}

	if !ctrl.CastingNow() {
		t.Fatal("CastingNow() = false after Think() promoted the queued IntentionCast desire: the AI controller is not live")
	}
	if len(rec.calls) != 0 {
		t.Fatal("skill handler ran before the Hit phase")
	}
	if got := int(hostile.MPValue()); got != beforeMP {
		t.Fatalf("MP = %d right after cast start, want unchanged %d (MPConsume applies at Hit, not Start)", got, beforeMP)
	}
	if len(target.frames) != 1 || target.frames[0][0] != serverpackets.OpcodeMagicSkillUse {
		t.Fatalf("frames after cast start = %#v, want [MagicSkillUse]", target.frames)
	}

	// HitTime is static (no atkSpd/spiritshot scaling), so buildPlan's
	// LaunchDelay/HitDelay split is exact: LaunchDelay = HitTime-400ms,
	// HitDelay = 400ms fixed (Controller.buildPlan, controller.go:622-625).
	clock.fire(1100 * time.Millisecond)

	if len(target.frames) != 2 || target.frames[1][0] != serverpackets.OpcodeMagicSkillLaunched {
		t.Fatalf("frames after Launch phase = %#v, want [MagicSkillUse MagicSkillLaunched]", target.frames)
	}

	clock.fire(400 * time.Millisecond)

	if len(rec.calls) != 1 {
		t.Fatalf("skill handler calls after Hit phase = %d, want 1", len(rec.calls))
	}
	if got := int(hostile.MPValue()); got != beforeMP-def.MPConsume {
		t.Fatalf("MP after Hit = %d, want %d (MPConsume applied at Hit)", got, beforeMP-def.MPConsume)
	}
}
