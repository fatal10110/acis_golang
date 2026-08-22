package skills

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// bootLearner boots a linked server whose client plays a level-5 character
// with sp 50 against the given options.
func bootLearner(t *testing.T, opts ...gameservertest.Option) (*gameservertest.Server, *testsupport.ScriptedClient, int32) {
	t.Helper()
	srv := gameservertest.Boot(t, opts...)
	return srv, srv.Client, srv.SoleObjectID(t)
}

// learnerTable returns the boot-default definition table plus extra defs;
// trainer offers and lists answer only for skill levels the loaded
// definition table knows.
func learnerTable(t testing.TB, extra ...modelskill.Definition) *skillstate.Persistence {
	t.Helper()
	defs := append([]modelskill.Definition{
		{ID: 248, Level: 3},
		{ID: 294, Level: 1},
	}, extra...)
	return skillPersistence(t, defs)
}

// generalLearnOpts are the boot options for the usual-trainer learn tests:
// skill 3's definition must be loaded for the trainer to offer it.
func generalLearnOpts(t *testing.T, sp int) []gameservertest.Option {
	t.Helper()
	return []gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 5, sp),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(learnerTable(t, modelskill.Definition{ID: 3, Level: 1, Activation: modelskill.ActivationActive})),
	}
}

// bindSkillShortcut persists a F1-slot shortcut bound to skillID before the
// client selects the character, mirroring a client that keeps the skill on
// its bar across levels.
func bindSkillShortcut(t *testing.T, srv *gameservertest.Server, objID, slot, skillID, level int32) {
	t.Helper()
	if err := srv.Shortcuts.Save(context.Background(), objID, shortcut.Shortcut{
		Slot: slot, Page: 0, Type: shortcut.Skill, ID: skillID, Level: level, CharacterType: 1,
	}); err != nil {
		t.Fatalf("seed shortcut: %v", err)
	}
}

// TestAcquireSkillInfoIncludesSpellbookRequirement verifies that a skill
// level with a spellbook sends an AcquireSkillInfo whose single requirement
// matches the oracle tuple (type 99, book item id, count 1, unk 50), and
// that learning without the book is rejected with the item-missing message
// while the trainer list keeps offering the skill.
func TestAcquireSkillInfoIncludesSpellbookRequirement(t *testing.T) {
	srv, c, objID := bootLearner(t, append(generalLearnOpts(t, 50),
		gameservertest.WithSpellbooks(bookPolicy(t, 3)))...)
	startInWorld(t, c)

	c.Send(encodeRequestAcquireSkillInfo(3, 1, 0))
	reply := c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeAcquireSkillInfo, "AcquireSkillInfo")
	r := wireReader(reply[1:])
	if id, level, cost, mode := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != 3 || level != 1 || cost != 50 || mode != 0 {
		t.Fatalf("AcquireSkillInfo header = id %d level %d cost %d mode %d, want 3/1/50/0", id, level, cost, mode)
	}
	if reqCount := r.ReadInt32(); reqCount != 1 {
		t.Fatalf("requirement count = %d, want 1", reqCount)
	}
	if rtype, itemID, count, unk := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); rtype != 99 || itemID != 57 || count != 1 || unk != 50 {
		t.Fatalf("requirement = type %d item %d count %d unk %d, want 99/57/1/50", rtype, itemID, count, unk)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read AcquireSkillInfo: %v", err)
	}

	c.Send(encodeRequestAcquireSkill(3, 1, 0))
	reply = c.Read()
	assertStaticSystemMessage(t, reply, serverpackets.SystemMessageItemMissingToLearnSkill)

	reply = c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeAcquireSkillList, "AcquireSkillList")
	r = wireReader(reply[1:])
	if skillType, count := r.ReadInt32(), r.ReadInt32(); skillType != int32(serverpackets.AcquireSkillTypeUsual) || count != 1 {
		t.Fatalf("AcquireSkillList = type %d count %d, want usual with 1 entry", skillType, count)
	}
	drainUntilQuiet(t, c)
	assertKnownSkills(t, srv, objID, map[int]int{})
}

// TestLearnGeneralSkillPersistsAndRefreshesShortcut walks the full learn
// flow: the SP charge (StatusUpdate + message), the LearnedSkill message,
// the refreshed skill list, the emptied trainer list, the persisted
// character_skills row, and the shortcut bound to the skill re-pointed at
// the newly learned level.
func TestLearnGeneralSkillPersistsAndRefreshesShortcut(t *testing.T) {
	srv, c, objID := bootLearner(t, generalLearnOpts(t, 50)...)
	bindSkillShortcut(t, srv, objID, 3, 3, -1)
	startInWorld(t, c)

	c.Send(encodeRequestAcquireSkill(3, 1, 0))
	assertSPStatus(t, c.Read(), objID, 0)

	reply := c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeSystemMessage, "SP-decreased SystemMessage")
	r := wireReader(reply[1:])
	if id, params, typ, cost := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != serverpackets.SystemMessageSPDecreasedS1 || params != 1 || typ != serverpackets.SystemMessageParamNumber || cost != 50 {
		t.Fatalf("SP-decreased message = %d/%d/%d/%d, want %d/1/number/50", id, params, typ, cost, serverpackets.SystemMessageSPDecreasedS1)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SP-decreased message: %v", err)
	}

	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageLearnedSkill, 3, 1)
	assertSkillList(t, c.Read(), skillListEntry{passive: 0, level: 1, id: 3})
	assertShortCutRegister(t, c, 3, 3, 1)
	assertAcquireSkillListEmpty(t, c.Read(), serverpackets.AcquireSkillTypeUsual)
	drainUntilQuiet(t, c)
	assertKnownSkills(t, srv, objID, map[int]int{3: 1})
}

// TestLearnSkillRejectsInsufficientSP verifies a character short on SP gets
// the not-enough-SP message and an unchanged trainer list, and nothing is
// persisted.
func TestLearnSkillRejectsInsufficientSP(t *testing.T) {
	srv, c, objID := bootLearner(t, generalLearnOpts(t, 49)...)
	startInWorld(t, c)

	c.Send(encodeRequestAcquireSkill(3, 1, 0))
	reply := c.Read()
	assertStaticSystemMessage(t, reply, serverpackets.SystemMessageNotEnoughSPToLearnSkill)

	reply = c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeAcquireSkillList, "AcquireSkillList")
	r := wireReader(reply[1:])
	if skillType, count := r.ReadInt32(), r.ReadInt32(); skillType != int32(serverpackets.AcquireSkillTypeUsual) || count != 1 {
		t.Fatalf("AcquireSkillList = type %d count %d, want usual with 1 entry", skillType, count)
	}
	if id, level, shownLevel, cost, unknown := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != 3 || level != 1 || shownLevel != 1 || cost != 50 || unknown != 0 {
		t.Fatalf("AcquireSkillList entry = %d/%d/%d/%d/%d, want 3/1/1/50/0", id, level, shownLevel, cost, unknown)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read AcquireSkillList: %v", err)
	}
	drainUntilQuiet(t, c)
	assertKnownSkills(t, srv, objID, map[int]int{})
}

// TestFishingSkillTreeGating covers the fishing branch of the trainer: its
// AcquireSkillInfo carries the oracle's item requirement tuple (mode 1,
// requirement type 4), learning consumes the required items, syncs the
// storage cap, and persists the skill.
func TestFishingSkillTreeGating(t *testing.T) {
	srv, c, objID := bootLearner(t,
		gameservertest.WithCharacter("Newbie", 5, 50),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(learnerTable(t, modelskill.Definition{ID: 1368, Level: 1, Activation: modelskill.ActivationActive})),
		gameservertest.WithSkillTrees(fishingTrees()))
	adena := srv.GiveItem(t, objID, 57, 5)
	startInWorld(t, c)

	c.Send(encodeRequestAcquireSkillInfo(1368, 1, 1))
	reply := c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeAcquireSkillInfo, "AcquireSkillInfo")
	r := wireReader(reply[1:])
	if id, level, cost, mode := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != 1368 || level != 1 || cost != 0 || mode != 1 {
		t.Fatalf("AcquireSkillInfo header = id %d level %d cost %d mode %d, want 1368/1/0/1", id, level, cost, mode)
	}
	if reqCount := r.ReadInt32(); reqCount != 1 {
		t.Fatalf("requirement count = %d, want 1", reqCount)
	}
	if rtype, itemID, count, unk := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); rtype != 4 || itemID != 57 || count != 2 || unk != 0 {
		t.Fatalf("requirement = type %d item %d count %d unk %d, want 4/57/2/0", rtype, itemID, count, unk)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read AcquireSkillInfo: %v", err)
	}

	c.Send(encodeRequestAcquireSkill(1368, 1, 1))
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageLearnedSkill, 1368, 1)

	reply = c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeExtended, "ExStorageMaxCount extended")
	if sub := wireReader(reply[1:]).ReadUint16(); sub != serverpackets.OpcodeExStorageMaxCount {
		t.Fatalf("extended opcode = %#x, want ExStorageMaxCount (%#x)", sub, serverpackets.OpcodeExStorageMaxCount)
	}
	assertSkillList(t, c.Read(), skillListEntry{passive: 0, level: 1, id: 1368})
	assertAcquireSkillListEmpty(t, c.Read(), serverpackets.AcquireSkillTypeFishing)
	drainUntilQuiet(t, c)
	assertKnownSkills(t, srv, objID, map[int]int{1368: 1})

	srv.InventoryUpdates.Tick()
	srv.FlushItems(t)
	var left int
	if err := srv.DB.QueryRowContext(context.Background(),
		"SELECT count FROM items WHERE object_id = ?", adena).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 3 {
		t.Fatalf("adena count after learning = %d, want 3 (two books consumed)", left)
	}
}

// TestFishingSkillTreeRejectsMissingItem verifies a character without the
// fishing item gets the item-missing message and a fishing trainer list,
// and nothing is learned or persisted.
func TestFishingSkillTreeRejectsMissingItem(t *testing.T) {
	srv, c, objID := bootLearner(t,
		gameservertest.WithCharacter("Newbie", 5, 50),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(learnerTable(t, modelskill.Definition{ID: 1368, Level: 1, Activation: modelskill.ActivationActive})),
		gameservertest.WithSkillTrees(fishingTrees()))
	startInWorld(t, c)

	c.Send(encodeRequestAcquireSkill(1368, 1, 1))
	reply := c.Read()
	assertStaticSystemMessage(t, reply, serverpackets.SystemMessageItemMissingToLearnSkill)

	reply = c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeAcquireSkillList, "AcquireSkillList")
	r := wireReader(reply[1:])
	if skillType, count := r.ReadInt32(), r.ReadInt32(); skillType != int32(serverpackets.AcquireSkillTypeFishing) || count != 1 {
		t.Fatalf("AcquireSkillList = type %d count %d, want fishing with 1 entry", skillType, count)
	}
	if id, level, shownLevel, cost, marker := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != 1368 || level != 1 || shownLevel != 1 || cost != 0 || marker != 1 {
		t.Fatalf("AcquireSkillList fishing entry = %d/%d/%d/%d/%d, want 1368/1/1/0/1", id, level, shownLevel, cost, marker)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read AcquireSkillList: %v", err)
	}
	drainUntilQuiet(t, c)
	assertKnownSkills(t, srv, objID, map[int]int{})
}
