package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// learnPassiveHP is the maxHp bonus of the passive learned in
// TestLearnPassiveChargesSPBeforeStatRefresh.
const learnPassiveHP = 77

// TestLearnPassiveChargesSPBeforeStatRefresh pins the usual-trainer reply
// for a passive whose stat funcs attach. The reference charges the SP before
// adding the skill (RequestAcquireSkill: removeExpAndSp, then addSkill), and
// addSkill's addStatFuncs sends the stat refresh UserInfo
// (broadcastModifiedStats -> updateAndBroadcastStatus(1)), so the client sees
// the SP StatusUpdate and SP_DECREASED_S1 first, then the UserInfo carrying
// the new stat, then LEARNED_SKILL_S1 and the SkillList.
func TestLearnPassiveChargesSPBeforeStatRefresh(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootLearner(t,
		gameservertest.WithCharacter("Newbie", 5, 50),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(learnerTable(t, modelskill.Definition{
			ID: 3, Level: 1, Activation: modelskill.ActivationPassive,
			Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "maxHp", Value: learnPassiveHP}},
		})))
	baseHP := int32(-1)
	for _, f := range startInWorld(t, c) {
		if f[0] == serverpackets.OpcodeUserInfo {
			baseHP = learnUserInfoMaxHP(t, f)
		}
	}
	if baseHP < 0 {
		t.Fatal("EnterWorld burst has no UserInfo")
	}
	selectTrainer(t, srv, c, objID, 0)

	c.Send(encodeRequestAcquireSkill(3, 1, 0))
	assertSPStatus(t, c.Read(), objID, 0)
	assertNumberSystemMessage(t, c.Read(), serverpackets.SystemMessageSPDecreasedS1, 50)
	if got, want := learnUserInfoMaxHP(t, c.Read()), baseHP+learnPassiveHP; got != want {
		t.Fatalf("stat refresh UserInfo MaxHP = %d, want %d (base %d + the passive)", got, want, baseHP)
	}
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageLearnedSkill, 3, 1)
	assertSkillList(t, c.Read(), skillListEntry{passive: 1, level: 1, id: 3})
	assertEmptyListClose(t, c, serverpackets.SystemMessageNoMoreSkillsToLearn)
	drainUntilQuiet(t, c)
	assertKnownSkills(t, srv, objID, map[int]int{3: 1})
}

// learnUserInfoMaxHP decodes MaxHP out of a UserInfo frame.
func learnUserInfoMaxHP(t *testing.T, frame []byte) int32 {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeUserInfo, "UserInfo")
	r := wire.NewReader(frame[1:])
	for range 5 { // x, y, z, heading, object id
		r.ReadInt32()
	}
	r.ReadString()
	for range 4 { // race, sex, class, level
		r.ReadInt32()
	}
	r.ReadInt64()
	for range 6 { // STR..MEN
		r.ReadInt32()
	}
	return r.ReadInt32()
}
