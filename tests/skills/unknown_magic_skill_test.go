package skills

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestMagicSkillUseUnknownSkillAnswersActionFailed asks to cast a skill id
// the table does not hold and one the table holds but the player never
// learned: each request is answered with ActionFailed alone, and a learned
// skill cast right after still starts.
func TestMagicSkillUseUnknownSkillAnswersActionFailed(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t,
			[]modelskill.Definition{
				{
					ID: 5, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
					HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
					MPInitialConsume: 2, MPConsume: 3, SkillType: "BUFF",
					Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
				},
				{
					ID: 6, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetGround,
					HitTime: 500, CastRange: 500, MPConsume: 3, SkillType: "SIGNET",
				},
			},
		)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 5, 1)
	startInWorld(t, c)

	for _, id := range []int32{9999, 6} {
		c.Send(encodeRequestMagicSkillUse(id, false, false))
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "unknown skill request")
		if queued := c.ReadQueued(); len(queued) != 0 {
			t.Fatalf("skill %d request sent opcode %#x after ActionFailed, want nothing", id, queued[0][0])
		}
	}

	c.Send(encodeRequestMagicSkillUse(5, false, false))
	for range 8 {
		if c.Read()[0] == serverpackets.OpcodeMagicSkillUse {
			return
		}
	}
	t.Fatal("learned skill request after the unknown ones sent no MagicSkillUse")
}
