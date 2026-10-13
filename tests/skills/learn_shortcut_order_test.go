package skills

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Passive maxHp bonuses of the two levels the general learn-order test
// raises: the stat refresh UserInfo carries the difference.
const (
	shortcutOrderHPLevel1 = 10
	shortcutOrderHPLevel2 = 77
)

// assertSkillShortcutLevel asserts the persisted shortcut at slot points at
// the given skill level.
func assertSkillShortcutLevel(t *testing.T, srv *gameservertest.Server, objID, slot, skillID, level int32) {
	t.Helper()
	srv.FlushPersistence(t)
	rows, err := srv.Shortcuts.ListByOwner(context.Background(), objID, 0)
	if err != nil {
		t.Fatalf("list shortcuts: %v", err)
	}
	for _, sc := range rows {
		if sc.Slot == slot && sc.Page == 0 {
			if sc.ID != skillID || sc.Level != level {
				t.Fatalf("shortcut slot %d = id %d level %d, want id %d level %d", slot, sc.ID, sc.Level, skillID, level)
			}
			return
		}
	}
	t.Fatalf("shortcut slot %d missing (rows %+v)", slot, rows)
}

// TestLearnNextLevelRefreshesShortcutBeforeLearnedMessage pins #3462 on the
// usual trainer: learning the next level of a skill already on the shortcut
// bar re-points that shortcut while the skill is added, so after the SP
// charge and the passive's stat refreshes the client sees ShortCutRegister,
// then LEARNED_SKILL_S1, the SkillList and the trainer list.
func TestLearnNextLevelRefreshesShortcutBeforeLearnedMessage(t *testing.T) {
	t.Parallel()
	const skillID = 3
	tmpl := gameservertest.ClassTemplate()
	tmpl.Skills = []player.SkillGrant{
		{SkillID: skillID, Level: 1, MinLevel: 5, Cost: 50},
		{SkillID: skillID, Level: 2, MinLevel: 5, Cost: 60},
	}
	passive := func(level int, hp float64) modelskill.Definition {
		return modelskill.Definition{
			ID: skillID, Level: level, Activation: modelskill.ActivationPassive,
			Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "maxHp", Value: hp}},
		}
	}
	srv, c, objID := bootLearner(t,
		gameservertest.WithCharacter("Newbie", 5, 100),
		gameservertest.WithWantChars(1),
		gameservertest.WithClassTemplate(tmpl),
		gameservertest.WithSkills(learnerTable(t, passive(1, shortcutOrderHPLevel1), passive(2, shortcutOrderHPLevel2))))
	seedKnownSkill(t, srv, objID, skillID, 1)
	bindSkillShortcut(t, srv, objID, 3, skillID, 1)
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

	c.Send(encodeRequestAcquireSkill(skillID, 2, 0))
	assertSPStatus(t, c.Read(), objID, 40)
	assertNumberSystemMessage(t, c.Read(), serverpackets.SystemMessageSPDecreasedS1, 60)
	// Replacing the level drops the old level's stat funcs, then attaches the
	// new level's; each change refreshes the stats with its own UserInfo.
	if got, want := learnUserInfoMaxHP(t, c.Read()), baseHP-shortcutOrderHPLevel1; got != want {
		t.Fatalf("stat removal UserInfo MaxHP = %d, want %d (level 1 bonus removed)", got, want)
	}
	if got, want := learnUserInfoMaxHP(t, c.Read()), baseHP-shortcutOrderHPLevel1+shortcutOrderHPLevel2; got != want {
		t.Fatalf("stat attach UserInfo MaxHP = %d, want %d (level 2 bonus attached)", got, want)
	}
	assertShortCutRegister(t, c, 3, skillID, 2)
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageLearnedSkill, skillID, 2)
	assertSkillList(t, c.Read(), skillListEntry{passive: 1, level: 2, id: skillID})
	assertEmptyListClose(t, c, serverpackets.SystemMessageNoMoreSkillsToLearn)
	drainUntilQuiet(t, c)
	assertKnownSkills(t, srv, objID, map[int]int{skillID: 2})
	assertSkillShortcutLevel(t, srv, objID, 3, skillID, 2)
}

// TestLearnNextFishingLevelRefreshesShortcutBeforeLearnedMessage pins #3462
// on the fishing trainer: after the paid item's disappeared message, the
// shortcut refresh precedes LEARNED_SKILL_S1,
// which precedes the storage-cap sync of the 1368-1372 skills, the
// SkillList and the fishing list.
func TestLearnNextFishingLevelRefreshesShortcutBeforeLearnedMessage(t *testing.T) {
	t.Parallel()
	const skillID = 1368
	srv, c, objID := bootLearner(t,
		gameservertest.WithCharacter("Newbie", 5, 50),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(learnerTable(t,
			modelskill.Definition{ID: skillID, Level: 1, Activation: modelskill.ActivationActive},
			modelskill.Definition{ID: skillID, Level: 2, Activation: modelskill.ActivationActive})),
		gameservertest.WithSkillTrees(&modelskill.Trees{Fishing: []modelskill.FishingSkill{
			{ID: skillID, Level: 1, MinLevel: 5, ItemID: 57, ItemCount: 2},
			{ID: skillID, Level: 2, MinLevel: 5, ItemID: 57, ItemCount: 2},
		}}))
	srv.GiveItem(t, objID, 57, 5)
	seedKnownSkill(t, srv, objID, skillID, 1)
	bindSkillShortcut(t, srv, objID, 3, skillID, 1)
	startInWorld(t, c)
	selectTrainer(t, srv, c, objID)

	c.Send(encodeRequestAcquireSkill(skillID, 2, 1))
	assertSystemMessageParams(t, c.Read(), serverpackets.SystemMessageS1DisappearedAdena,
		systemMessageParam{serverpackets.SystemMessageParamNumber, 2})
	assertShortCutRegister(t, c, 3, skillID, 2)
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageLearnedSkill, skillID, 2)
	reply := c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeExtended, "ExStorageMaxCount extended")
	if sub := wireReader(reply[1:]).ReadUint16(); sub != serverpackets.OpcodeExStorageMaxCount {
		t.Fatalf("extended opcode = %#x, want ExStorageMaxCount (%#x)", sub, serverpackets.OpcodeExStorageMaxCount)
	}
	assertSkillList(t, c.Read(), skillListEntry{passive: 0, level: 2, id: skillID})
	assertEmptyListClose(t, c, serverpackets.SystemMessageNoMoreSkillsToLearn)
	drainUntilQuiet(t, c)
	assertKnownSkills(t, srv, objID, map[int]int{skillID: 2})
	assertSkillShortcutLevel(t, srv, objID, 3, skillID, 2)
}
