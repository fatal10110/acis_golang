package skills

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// castCollectingEffectFeedback casts skillID on the caster and returns, in
// arrival order, the skill-name system messages ("sm:<msg>:<skill>") and
// icon refreshes ("icons:[...]") that follow the launch.
func castCollectingEffectFeedback(t *testing.T, c *testsupport.ScriptedClient, skillID int32) []string {
	t.Helper()
	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillUse, "MagicSkillUse")
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageUseS1, skillID, 1)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillLaunched, "MagicSkillLaunched")

	var got []string
	for i := 0; i < 100; i++ {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return got
		}
		switch frame[0] {
		case serverpackets.OpcodeSystemMessage:
			r := wire.NewReader(frame[1:])
			id, params := r.ReadInt32(), r.ReadInt32()
			if params == 1 && r.ReadInt32() == serverpackets.SystemMessageParamSkillName {
				got = append(got, fmt.Sprintf("sm:%d:%d", id, r.ReadInt32()))
			}
		case serverpackets.OpcodeAbnormalStatusUpdate:
			got = append(got, fmt.Sprintf("icons:%v", buffSlotIDs(readAbnormalStatusUpdateEntriesFromFrame(t, frame))))
		}
	}
	t.Fatal("client kept receiving frames after 100 drains")
	return got
}

// TestStackChangeAnnouncesDisappearedThenFelt pins the add path's stack
// messages: a buff that takes over an empty stack group is felt; a
// higher-order buff over it announces the displaced buff, then the new one,
// then the icon refresh; an identical recast replaces the head the same way.
func TestStackChangeAnnouncesDisappearedThenFelt(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			stackedBuffDef(201, 1, 30), stackedBuffDef(202, 2, 30),
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 201, 1)
	seedKnownSkill(t, srv, objID, 202, 1)
	startInWorld(t, c)

	felt := serverpackets.SystemMessageYouFeelS1Effect
	gone := serverpackets.SystemMessageEffectS1Disappeared
	for _, step := range []struct {
		skill int32
		want  []string
	}{
		{skill: 201, want: []string{fmt.Sprintf("sm:%d:201", felt), "icons:[201]"}},
		{skill: 202, want: []string{fmt.Sprintf("sm:%d:201", gone), fmt.Sprintf("sm:%d:202", felt), "icons:[202]"}},
		{skill: 202, want: []string{fmt.Sprintf("sm:%d:202", gone), fmt.Sprintf("sm:%d:202", felt), "icons:[202]"}},
	} {
		if got := castCollectingEffectFeedback(t, c, step.skill); !slices.Equal(got, step.want) {
			t.Fatalf("feedback after casting %d = %v, want %v", step.skill, got, step.want)
		}
	}
	if ids := liveHeldSkillIDs(t, srv, objID); !slices.Equal(ids, []int32{202}) {
		t.Fatalf("held effects = %v, want [202]", ids)
	}
}

// TestLowerStackedBuffIsNotAnnounced pins that a lower-order buff joining a
// stack group it cannot lead sends no stack message.
func TestLowerStackedBuffIsNotAnnounced(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			stackedBuffDef(201, 1, 30), stackedBuffDef(202, 2, 30),
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 201, 1)
	seedKnownSkill(t, srv, objID, 202, 1)
	startInWorld(t, c)

	castCollectingEffectFeedback(t, c, 202)
	if got := castCollectingEffectFeedback(t, c, 201); !slices.Equal(got, []string{"icons:[202]"}) {
		t.Fatalf("feedback after lower buff = %v, want only the icon refresh", got)
	}
}
