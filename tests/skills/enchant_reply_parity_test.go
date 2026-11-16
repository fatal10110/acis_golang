package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// assertEnchantListReopened asserts the trainer's enchant list comes back
// with entries, closed by ActionFailed.
func assertEnchantListReopened(t *testing.T, frame []byte) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeExtended, "ExEnchantSkillList")
	if sub := wireReader(frame[1:]).ReadUint16(); sub != serverpackets.OpcodeExEnchantSkillList {
		t.Fatalf("extended opcode = %#x, want ExEnchantSkillList (%#x)", sub, serverpackets.OpcodeExEnchantSkillList)
	}
}

// TestEnchantSkillFailureAtMaxNormalLevelLeavesShortcuts pins #4871: a failed
// first enchant of a skill already at its max normal level puts the skill
// back at the level it has, which changes nothing, so no bound shortcut is
// re-registered. The failure message is the first frame; the skill list,
// user info and the reopened enchant list still follow.
func TestEnchantSkillFailureAtMaxNormalLevelLeavesShortcuts(t *testing.T) {
	t.Parallel()
	srv, c, objID := seedEnchanter(t, 1,
		gameservertest.WithSkillTrees(enchantTree(101, 0)),
		gameservertest.WithSkillEnchantRoll(func() int { return 50 }),
	)
	bindSkillShortcut(t, srv, objID, 0, 1, 1)
	bindSkillShortcut(t, srv, objID, 1, 1, 1)
	startInWorld(t, c)
	selectTrainer(t, srv, c, objID, duelistParentClass)

	c.Send(encodeRequestExEnchantSkill(1, 101))
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageFailedEnchantingSkillS1, 1, 101)
	assertSkillListContains(t, c.Read(), skillListEntry{passive: 1, level: 1, id: 1})
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeUserInfo, "post-enchant UserInfo")
	assertEnchantListReopened(t, c.Read())
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "enchant list ActionFailed")
	drainUntilQuiet(t, c)
	assertKnownSkills(t, srv, objID, map[int]int{1: 1})
	assertSkillShortcutLevel(t, srv, objID, 0, 1, 1)
	assertSkillShortcutLevel(t, srv, objID, 1, 1, 1)
}

// TestEnchantSkillBadPassiveStillSendsFullReply pins #4873: once the enchant
// is paid and rolled, a passive definition whose stat functions do not build
// cannot cut the reply short. The skill is at the enchanted level and the
// client gets the shortcut refresh, the result message, the skill list, user
// info and the reopened list.
func TestEnchantSkillBadPassiveStillSendsFullReply(t *testing.T) {
	t.Parallel()
	srv, c, objID := seedEnchanter(t, 1,
		gameservertest.WithSkillTrees(enchantTree(101, 100)),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{ID: 1, Level: 1},
			{
				ID: 1, Level: 101, Activation: modelskill.ActivationPassive,
				Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "noSuchStat", Value: 1}},
			},
		})),
	)
	bindSkillShortcut(t, srv, objID, 0, 1, 1)
	startInWorld(t, c)
	selectTrainer(t, srv, c, objID, duelistParentClass)

	c.Send(encodeRequestExEnchantSkill(1, 101))
	assertShortCutRegister(t, c, 0, 1, 101)
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageSucceededEnchantingSkillS1, 1, 101)
	assertSkillListContains(t, c.Read(), skillListEntry{passive: 1, level: 101, id: 1})
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeUserInfo, "post-enchant UserInfo")
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageThereIsNoSkillThatEnablesEnchant)
	assertEmptyListClose(t, c, serverpackets.SystemMessageNoMoreSkillsToLearn)
	drainUntilQuiet(t, c)
	assertKnownSkills(t, srv, objID, map[int]int{1: 101})
}

// TestAcquireSkillInfoAnswersNodeAboveLevelAndFree pins #4872: the usual
// trainer quotes any next-level node of the player's template, whatever its
// minimum level and even at no cost; neither gates the info request.
func TestAcquireSkillInfoAnswersNodeAboveLevelAndFree(t *testing.T) {
	t.Parallel()
	tmpl := gameservertest.ClassTemplate()
	tmpl.Skills = []player.SkillGrant{{SkillID: 3, Level: 1, MinLevel: 50, Cost: 0}}
	srv, c, objID := bootLearner(t, append(generalLearnOpts(t, 50), gameservertest.WithClassTemplate(tmpl))...)
	startInWorld(t, c)
	selectTrainer(t, srv, c, objID, 0)

	c.Send(encodeRequestAcquireSkillInfo(3, 1, 0))
	reply := c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeAcquireSkillInfo, "AcquireSkillInfo")
	r := wireReader(reply[1:])
	if id, level, cost, mode, reqs := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != 3 || level != 1 || cost != 0 || mode != 0 || reqs != 0 {
		t.Fatalf("AcquireSkillInfo = id %d level %d cost %d mode %d reqs %d, want 3/1/0/0/0", id, level, cost, mode, reqs)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read AcquireSkillInfo: %v", err)
	}
	drainUntilQuiet(t, c)
}
