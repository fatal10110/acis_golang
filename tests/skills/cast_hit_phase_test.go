package skills

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// bootHitPhaseCaster boots one caster knowing def at level 1.
func bootHitPhaseCaster(t *testing.T, def modelskill.Definition) (*gameservertest.Server, *testsupport.ScriptedClient, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, int(def.ID), def.Level)
	startInWorld(t, c)
	return srv, c, objID
}

// TestCastHitReusesLaunchTargetAfterItDies kills the target of an offensive
// single-target skill between its launch and its hit. The reference resolves
// the affected list once at launch (CreatureCast.onMagicLaunch,
// CreatureCast.java:232) and hands that same list to the skill handler at
// the hit (:291) without re-checking the target, so the handler still runs:
// the debuff itself is refused on the corpse, but the skill's self effect
// lands on the caster (Continuous.java hasSelfEffects).
func TestCastHitReusesLaunchTargetAfterItDies(t *testing.T) {
	t.Parallel()
	const skillID = 2452
	srv, c, objID := bootHitPhaseCaster(t, modelskill.Definition{
		ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		CastRange: 900, HitTime: 500, StaticHitTime: true,
		SkillType: "DEBUFF", EffectType: "DEBUFF", Offensive: true, Debuff: true,
		BaseLandRate: 100, IgnoreResists: true,
		Effects:     []modelskill.EffectTemplate{{Name: "Debuff", Time: 60}},
		SelfEffects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, Self: true}},
	})
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	readCastStartFrames(t, c, objID, skillID, 1, 500, 0, hostile.ObjectID())
	// The launch has gone out; the hit is still 400ms away on the driven
	// clock.
	onNPCQueue(t, hostile, func() {
		if !hostile.Die(nil, nil) {
			t.Error("target death rejected")
		}
	})
	srv.Advance(t, time.Second)
	drainUntilQuiet(t, c)

	if !slices.Contains(liveHeldSkillIDs(t, srv, objID), skillID) {
		t.Fatalf("caster effects = %v, want the skill's self effect %d: the hit must run the handler on the launch-time target", liveHeldSkillIDs(t, srv, objID), skillID)
	}
	if got := len(hostile.EffectList().All()); got != 0 {
		t.Fatalf("dead target effects = %d, want 0", got)
	}
}

// TestCtrlForcedBuffOnMonsterLandsAtHit forces a non-offensive
// single-target buff onto a monster. The start-time check accepts it only
// because ctrl is held (TargetOne.meetCastConditions); nothing re-checks the
// target at the hit, so the buff lands on the monster.
func TestCtrlForcedBuffOnMonsterLandsAtHit(t *testing.T) {
	t.Parallel()
	const skillID = 2453
	srv, c, objID := bootHitPhaseCaster(t, modelskill.Definition{
		ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		CastRange: 900, HitTime: 500, StaticHitTime: true, SkillType: "BUFF",
		Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
	})
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(skillID, true, false))
	readCastStartFrames(t, c, objID, skillID, 1, 500, 0, hostile.ObjectID())
	srv.Advance(t, time.Second)
	drainUntilQuiet(t, c)

	var held []int
	onNPCQueue(t, hostile, func() {
		for _, e := range hostile.EffectList().All() {
			held = append(held, int(e.Skill.ID))
		}
	})
	if !slices.Contains(held, skillID) {
		t.Fatalf("monster effects = %v, want the ctrl-forced buff %d", held, skillID)
	}
}

// TestCastFinalCostsSendOneStatusPerPayment pins the hit-time cost packets
// (CreatureCast.onMagicHitTimer, CreatureCast.java:248-271): the MP payment
// sends the caster's status at once, then the HP payment sends its own; when
// the caster lost the HP for the second cost after the cast started, the MP
// status still goes out, ahead of NOT_ENOUGH_HP and the abort.
func TestCastFinalCostsSendOneStatusPerPayment(t *testing.T) {
	t.Parallel()
	const skillID = 1859
	def := modelskill.Definition{
		ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, StaticHitTime: true, MPConsume: 3, HPConsume: 10, SkillType: "DUMMY",
	}
	for _, tt := range []struct {
		name     string
		dropHPTo int
	}{
		{name: "both costs paid"},
		{name: "HP lost before the hit", dropHPTo: 5},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, c, objID := bootHitPhaseCaster(t, def)
			hp, mp := srv.PlayerCurrentHP(t, objID), srv.PlayerCurrentMP(t, objID)

			c.Send(encodeRequestMagicSkillUse(skillID, false, false))
			readCastStartFrames(t, c, objID, skillID, 1, 500, 0, objID)
			if tt.dropHPTo > 0 {
				onPlayerQueue(t, srv, objID, func(pc *player.Character) {
					pc.ReduceCurrentHP(hp - tt.dropHPTo)
				})
				hp = tt.dropHPTo
			}

			assertCasterStatus(t, srv, c.Read(), objID, hp, mp-3)
			if tt.dropHPTo > 0 {
				assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageNotEnoughHP)
				assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillCanceled, "hit-time abort")
				assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "hit-time abort ack")
				drainUntilQuiet(t, c)
				return
			}
			assertCasterStatus(t, srv, c.Read(), objID, hp-10, mp-3)
			drainUntilQuiet(t, c)
		})
	}
}

// TestCastFinalMPShortageSendsNoStatus is the other hit-time failure: with no
// MP payment made there is no status, only NOT_ENOUGH_MP and the abort.
func TestCastFinalMPShortageSendsNoStatus(t *testing.T) {
	t.Parallel()
	const skillID = 1860
	srv, c, objID := bootHitPhaseCaster(t, modelskill.Definition{
		ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 500, StaticHitTime: true, MPConsume: 3, HPConsume: 10, SkillType: "DUMMY",
	})

	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	readCastStartFrames(t, c, objID, skillID, 1, 500, 0, objID)
	srv.DrainPlayerMP(t, objID, srv.PlayerCurrentMP(t, objID)-1)

	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageNotEnoughMP)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillCanceled, "hit-time abort")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "hit-time abort ack")
	drainUntilQuiet(t, c)
}

// TestFusionCastLandsForceBeforeMagicSkillUse pins PlayerCast.doFusionCast's
// start order (PlayerCast.java:50-89): the force effect lands on the target
// (FusionSkill's constructor) before the MagicSkillUse broadcast, USE_S1
// follows it, and the blue gauge follows USE_S1 whatever the hit time.
func TestFusionCastLandsForceBeforeMagicSkillUse(t *testing.T) {
	t.Parallel()
	const (
		fusionID = 2651
		forceID  = 2652
	)
	for _, hitTime := range []int{15_000, 400} {
		t.Run("hit time "+strconv.Itoa(hitTime), func(t *testing.T) {
			t.Parallel()
			defs := []modelskill.Definition{
				{
					ID: fusionID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
					CastRange: 900, HitTime: hitTime, SkillType: "FUSION", TriggeredID: forceID, TriggeredLevel: 1,
				},
				{
					ID: forceID, Level: 1, Activation: modelskill.ActivationPassive, Target: modelskill.TargetSelf,
					SkillType: "BUFF", Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
				},
			}
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Caster", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(skillPersistence(t, defs)),
			)
			caster, casterID := srv.Client, srv.SoleObjectID(t)
			patientID := srv.SeedCharacterFor(t, "player2", "Patient", 5, 0).ID
			patient := srv.DialClient(t, "player2", 1)
			seedKnownSkill(t, srv, casterID, fusionID, 1)
			startInWorld(t, caster)
			startInWorldAmongPlayers(t, patient)
			drainUntilQuiet(t, caster)
			drainUntilQuiet(t, patient)

			px, py, pz := srv.PlayerPosition(t, patientID)
			caster.Send(encodeAction(patientID, int32(px), int32(py), int32(pz), false))
			drainUntilQuiet(t, caster)
			drainUntilQuiet(t, patient)

			caster.Send(encodeRequestMagicSkillUse(fusionID, false, false))
			use := caster.Read()
			assertFrameOpcode(t, use, serverpackets.OpcodeMagicSkillUse, "caster MagicSkillUse")
			assertSystemMessageSkillFrame(t, caster.Read(), serverpackets.SystemMessageUseS1, fusionID, 1)
			gauge := caster.Read()
			assertFrameOpcode(t, gauge, serverpackets.OpcodeSetupGauge, "SetupGauge")
			r := wireReader(gauge[1:])
			if color, current, maxTime := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); color != int32(serverpackets.GaugeBlue) || current != int32(hitTime) || maxTime != int32(hitTime) {
				t.Fatalf("SetupGauge = color %d current %d max %d, want blue/%d/%d", color, current, maxTime, hitTime, hitTime)
			}

			// The target's client sees its new force icon before the
			// caster's MagicSkillUse.
			var sawIcon bool
			for {
				frame := patient.Read()
				if frame[0] == serverpackets.OpcodeAbnormalStatusUpdate {
					for _, e := range readAbnormalStatusUpdateEntriesFromFrame(t, frame) {
						sawIcon = sawIcon || e.SkillID == forceID
					}
					continue
				}
				if frame[0] == serverpackets.OpcodeMagicSkillUse {
					break
				}
			}
			if !sawIcon {
				t.Fatalf("target saw the caster's MagicSkillUse before the force effect %d icon", forceID)
			}
		})
	}
}
