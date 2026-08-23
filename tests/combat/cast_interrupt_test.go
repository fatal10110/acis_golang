package combat

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestTargetCancelAbortsCastInsideInterruptWindow pins Esc-during-cast:
// RequestTargetCancel with unselect=0 while the cast bar still runs aborts
// the cast — MagicSkillCanceled followed by ActionFailed — and no launch
// ever goes out, instead of the not-casting clear-target reply.
func TestTargetCancelAbortsCastInsideInterruptWindow(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(combatPersistence(t,
			[]modelskill.Definition{{
				ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				HitTime: 5000, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
				MPInitialConsume: 2, MPConsume: 3, SkillType: "DUMMY",
			}},
		)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 3, 1)
	startInWorld(t, c)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(3, false, false))
	assertFrameOpcode(t, mustRead(t, c, "MagicSkillUse"), serverpackets.OpcodeMagicSkillUse, "MagicSkillUse")
	assertFrameOpcode(t, mustRead(t, c, "cast message"), serverpackets.OpcodeSystemMessage, "cast message")
	assertFrameOpcode(t, mustRead(t, c, "SetupGauge"), serverpackets.OpcodeSetupGauge, "SetupGauge")

	c.Send(encodeRequestTargetCancel(0))
	assertFrameOpcode(t, mustRead(t, c, "MagicSkillCanceled"), serverpackets.OpcodeMagicSkillCanceled, "MagicSkillCanceled")
	assertFrameOpcode(t, mustRead(t, c, "cancel ActionFailed"), serverpackets.OpcodeActionFailed, "cancel ActionFailed")

	if reply := c.ReadWithTimeout(readQuietWindow); reply != nil {
		t.Fatalf("post-cancel frame opcode %#x, want silence (aborted cast never launches)", reply[0])
	}
}
