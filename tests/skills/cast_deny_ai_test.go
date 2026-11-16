package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// denyGroundSkillID is a ground-targeted skill the caster may aim at its
// own feet.
const denyGroundSkillID int32 = 5

// bootCastDenyAI boots an in-world caster who knows the gate skills plus a
// ground skill, over passable geodata.
func bootCastDenyAI(t *testing.T) (*gameservertest.Server, int32) {
	t.Helper()
	defs := append(gateSkills(), modelskill.Definition{
		ID: modelskill.ID(denyGroundSkillID), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetGround,
		CastRange: 40, HitTime: 500, StaticHitTime: true, MPConsume: 5, SkillType: "BUFF",
		Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
	})
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithGeo(&gameservertest.GateGeo{}),
		gameservertest.WithSkills(skillPersistence(t, defs)),
	)
	objID := srv.SoleObjectID(t)
	for _, def := range defs {
		seedKnownSkill(t, srv, objID, int(def.ID), 1)
	}
	startInWorld(t, srv.Client)
	return srv, objID
}

func stageConfused(t *testing.T) func(*player.Character) {
	return func(pc *player.Character) {
		e, err := effect.New(effect.Skill{ID: 2, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: "Confusion", Time: 30})
		if err != nil {
			t.Errorf("effect.New(Confusion): %v", err)
			return
		}
		e.Effector, e.Effected = pc, pc
		pc.EffectList().Add(e)
	}
}

func stageTeleporting(*testing.T) func(*player.Character) {
	return func(pc *player.Character) { pc.SetTeleporting(true) }
}

func stageStoreOpen(*testing.T) func(*player.Character) {
	return func(pc *player.Character) { pc.SetOperateType(privatestore.OperateSell) }
}

// sendGroundAtFeet sends RequestExMagicSkillUseGround aimed at the caster's
// own position.
func sendGroundAtFeet(t *testing.T, srv *gameservertest.Server, objID int32) {
	t.Helper()
	x, y, z := srv.PlayerPosition(t, objID)
	srv.Client.Send(encodeRequestExMagicSkillUseGround(int32(x), int32(y), int32(z), denyGroundSkillID, false, false))
}

// TestCastRequestRefusedWhileCasterCannotAct pins the request gates of both
// skill opcodes: RequestMagicSkillUse refuses an out-of-control caster
// (confused or teleporting) before the skill is looked up, and both requests
// refuse a caster that cannot act on an intention (teleporting, or running a
// private store) before any cast condition. Each answers ActionFailed alone:
// no sitting refusal, no cast packets, no MP spent, no cast in flight.
func TestCastRequestRefusedWhileCasterCannotAct(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		stage  func(*testing.T) func(*player.Character)
		ground bool
	}{
		{name: "confused", stage: stageConfused},
		{name: "teleporting", stage: stageTeleporting},
		{name: "store open", stage: stageStoreOpen},
		{name: "teleporting ground", stage: stageTeleporting, ground: true},
		{name: "store open ground", stage: stageStoreOpen, ground: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, objID := bootCastDenyAI(t)
			c := srv.Client
			onPlayerQueue(t, srv, objID, tt.stage(t))
			drainUntilQuiet(t, c)
			mpBefore := srv.PlayerCurrentMP(t, objID)

			if tt.ground {
				sendGroundAtFeet(t, srv, objID)
			} else {
				c.Send(encodeRequestMagicSkillUse(gateActiveSkillID, false, false))
			}
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "refused cast")
			if queued := c.ReadQueued(); len(queued) != 0 {
				t.Fatalf("refused cast extra opcodes = %x, want none", frameOpcodes(queued))
			}
			if got := srv.PlayerCurrentMP(t, objID); got != mpBefore {
				t.Fatalf("MP after refused cast = %d, want unchanged %d", got, mpBefore)
			}
			if srv.PlayerCastingNow(t, objID) {
				t.Fatal("refused cast is in flight")
			}
		})
	}
}

// TestConfusedGroundCastProceeds pins that confusion, which only the plain
// skill request refuses, leaves the ground request open: a confused caster's
// ground skill aimed at its feet starts its cast.
func TestConfusedGroundCastProceeds(t *testing.T) {
	t.Parallel()
	srv, objID := bootCastDenyAI(t)
	onPlayerQueue(t, srv, objID, stageConfused(t))
	drainUntilQuiet(t, srv.Client)

	sendGroundAtFeet(t, srv, objID)
	readUntil(t, srv.Client, serverpackets.OpcodeMagicSkillUse)
	drainUntilQuiet(t, srv.Client)
}
