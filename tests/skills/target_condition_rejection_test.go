package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// assertNoActionFailedUntilQuiet drains the frames that trail a target
// rejection and fails on ActionFailed: a failed target condition answers
// with its system message only, never with ActionFailed.
func assertNoActionFailedUntilQuiet(t *testing.T, c *testsupport.ScriptedClient, what string) {
	t.Helper()
	for range 20 {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return
		}
		if frame[0] == serverpackets.OpcodeActionFailed {
			t.Fatalf("%s sent ActionFailed after the rejection message", what)
		}
	}
	t.Fatalf("%s kept sending frames", what)
}

func bootTargetConditionCaster(t *testing.T, def modelskill.Definition) *gameservertest.Server {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
	)
	seedKnownSkill(t, srv, srv.SoleObjectID(t), int(def.ID), int(def.Level))
	return srv
}

func conditionNPCTemplate(kind string, race npc.Race) *npc.Template {
	return &npc.Template{
		ID: 100, TemplateID: 100, Type: kind, Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
		CollisionRadius: 8, CollisionHeight: 20,
		CorpseTime: 8, Race: race,
	}
}

func sweeperSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: 42, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetCorpseMob, SkillType: "SWEEP",
		CastRange: 600, HitTime: 500, ReuseDelay: 500,
		StaticHitTime: true, StaticReuse: true, Power: 100,
	}
}

// TestCorpseMobCastRejections drives RequestMagicSkillUse against dead NPCs
// whose corpse fails a CORPSE_MOB condition. Each failure answers with its
// own system message and no ActionFailed, like every other target-condition
// rejection.
func TestCorpseMobCastRejections(t *testing.T) {
	t.Parallel()
	harvest := sweeperSkill()
	harvest.ID, harvest.SkillType = 432, "HARVEST"
	fresh := func() time.Time { return time.Now().Add(time.Minute) }
	old := func() time.Time { return time.Now().Add(time.Second) }
	for _, tt := range []struct {
		name     string
		def      modelskill.Definition
		kind     string
		dead     bool
		deadline func() time.Time
		message  int
	}{
		{"living target has no corpse", sweeperSkill(), "Monster", false, nil, serverpackets.SystemMessageInvalidTarget},
		{"harvest on a guard corpse", harvest, "Guard", true, fresh, serverpackets.SystemMessageHarvestFailedSeedNotSown},
		{"sweep on a guard corpse", sweeperSkill(), "Guard", true, fresh, serverpackets.SystemMessageSweeperFailedTargetNotSpoiled},
		{"sweep on a too-old monster corpse", sweeperSkill(), "Monster", true, old, serverpackets.SystemMessageCorpseTooOldSkillNotUsed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := bootTargetConditionCaster(t, tt.def)
			c := srv.Client
			startInWorld(t, c)
			mob := srv.SpawnHostileNPCTemplateAt(t, conditionNPCTemplate(tt.kind, npc.RaceHumanoid), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
			drainUntilQuiet(t, c)
			targetHostile(t, c, mob.ObjectID())
			if tt.dead {
				mob.MarkDead()
				mob.SetCorpseDeadline(tt.deadline())
			}
			drainUntilQuiet(t, c)

			c.Send(encodeRequestMagicSkillUse(int32(tt.def.ID), false, false))
			assertStaticSystemMessage(t, c.Read(), tt.message)
			assertNoActionFailedUntilQuiet(t, c, tt.name)
		})
	}
}

// TestWalkingCorpseMobRejectionStopsMovement casts Sweeper on a too-old
// corpse mid-walk. A resolved but ineligible corpse is rejected only after
// the long-hit-time cast has stopped the caster, so StopMove lands before
// the corpse message.
func TestWalkingCorpseMobRejectionStopsMovement(t *testing.T) {
	t.Parallel()
	def := sweeperSkill()
	srv := bootTargetConditionCaster(t, def)
	c := srv.Client
	startInWorld(t, c)
	mob := srv.SpawnHostileNPCTemplateAt(t, conditionNPCTemplate("Monster", npc.RaceHumanoid), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	targetHostile(t, c, mob.ObjectID())
	mob.MarkDead()
	mob.SetCorpseDeadline(time.Now().Add(time.Second))
	drainUntilQuiet(t, c)

	c.Send(encodeMoveBackwardToLocation(200, 70, 30))
	expectGroundClickAck(t, c)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMoveToLocation, "walk")

	c.Send(encodeRequestMagicSkillUse(int32(def.ID), false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeStopMove, "cast stop")
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCorpseTooOldSkillNotUsed)
	assertNoActionFailedUntilQuiet(t, c, "walking too-old corpse rejection")
}

// TestUndeadTargetCastRejections drives an UNDEAD-target skill: a living
// monster that is not undead refuses the skill by name, a dead undead
// monster is an invalid target, and a living undead monster starts the cast.
func TestUndeadTargetCastRejections(t *testing.T) {
	t.Parallel()
	const skillID int32 = 1400
	def := modelskill.Definition{
		ID: modelskill.ID(skillID), Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetUndead, Offensive: true, SkillType: "DEBUFF",
		CastRange: 600, HitTime: 500, ReuseDelay: 60_000,
		StaticHitTime: true, StaticReuse: true,
	}
	for _, tt := range []struct {
		name  string
		race  npc.Race
		dead  bool
		check func(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, mobID int32)
	}{
		{"living non-undead monster", npc.RaceHumanoid, false, func(t *testing.T, _ *gameservertest.Server, c *testsupport.ScriptedClient, _ int32) {
			assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageS1CannotBeUsed, skillID, 1)
			assertNoActionFailedUntilQuiet(t, c, "living non-undead")
		}},
		{"dead undead monster", npc.RaceUndead, true, func(t *testing.T, _ *gameservertest.Server, c *testsupport.ScriptedClient, _ int32) {
			assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageInvalidTarget)
			assertNoActionFailedUntilQuiet(t, c, "dead undead")
		}},
		{"living undead monster", npc.RaceUndead, false, func(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, mobID int32) {
			readCastStartFrames(t, c, srv.SoleObjectID(t), skillID, 1, 500, 60_000, mobID)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := bootTargetConditionCaster(t, def)
			c := srv.Client
			startInWorld(t, c)
			mob := srv.SpawnHostileNPCTemplateAt(t, conditionNPCTemplate("Monster", tt.race), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
			drainUntilQuiet(t, c)
			targetHostile(t, c, mob.ObjectID())
			if tt.dead {
				mob.MarkDead()
			}
			drainUntilQuiet(t, c)

			c.Send(encodeRequestMagicSkillUse(skillID, false, false))
			tt.check(t, srv, c, mob.ObjectID())
		})
	}
}
