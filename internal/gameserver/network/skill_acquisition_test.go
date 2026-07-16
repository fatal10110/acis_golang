package network

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// testBookPolicy maps the test template's skill 3 to Adena (item id 57), so
// learning it requires and consumes one Adena. The wire contract for the
// requirement entry (type 99, item id, count 1, unk 50) is what the oracle
// sends; the item id itself is data-driven.
func testBookPolicy(t *testing.T) modelskill.BookPolicy {
	t.Helper()
	tbl, err := modelskill.NewSpellbookTable([]modelskill.Spellbook{{SkillID: 3, ItemID: 57}})
	if err != nil {
		t.Fatalf("build spellbook table: %v", err)
	}
	return modelskill.BookPolicy{Table: tbl, SPBookNeeded: true, DivineBookNeeded: true}
}

// newAcquireSkillClient wires a linked client with a spellbook policy and one
// selectable character at level 5 with sp sp.
func newAcquireSkillClient(t *testing.T, skills *SkillPersistence, policy modelskill.BookPolicy, trees *modelskill.Trees, sp int, seedItems func(*fakeItemStore, int32)) *fakeGameClient {
	t.Helper()
	var objID int32
	c, _, _, _, _ := newLinkedGameClientWithSkillsShortcutsCrestsSeed(t, skills, nil, nil, policy, trees, func(chars *fakeCharStore, items *fakeItemStore) {
		objID = seedSelectableCharacter(t, chars, "player1", "Newbie", 5, sp)
		if seedItems != nil {
			seedItems(items, objID)
		}
	}, 1)
	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)
	return c
}

// TestAcquireSkillInfoIncludesSpellbookRequirement verifies that a skill
// level with a spellbook sends an AcquireSkillInfo whose single requirement
// matches the oracle tuple (type 99, book item id, count 1, unk 50).
func TestAcquireSkillInfoIncludesSpellbookRequirement(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := NewSkillPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{ID: 3, Level: 1, Activation: modelskill.ActivationActive},
	}), store)
	c := newAcquireSkillClient(t, skills, testBookPolicy(t), nil, 50, nil)

	c.send(encodeRequestAcquireSkillInfo(3, 1, 0))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeAcquireSkillInfo {
		t.Fatalf("opcode = %#x, want AcquireSkillInfo (%#x)", reply[0], serverpackets.OpcodeAcquireSkillInfo)
	}
	r := wire.NewReader(reply[1:])
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
}

// TestAcquireSkillLearnBlockedByMissingSpellbook verifies that learning a
// spellbook-requiring skill without the book in inventory sends the
// item-missing system message and the skill list, without learning the skill.
func TestAcquireSkillLearnBlockedByMissingSpellbook(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := NewSkillPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{ID: 3, Level: 1, Activation: modelskill.ActivationActive},
	}), store)
	c := newAcquireSkillClient(t, skills, testBookPolicy(t), nil, 50, nil)

	c.send(encodeRequestAcquireSkill(3, 1, 0))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcode = %#x, want SystemMessage (%#x)", reply[0], serverpackets.OpcodeSystemMessage)
	}
	if id := wire.NewReader(reply[1:]).ReadInt32(); id != serverpackets.SystemMessageItemMissingToLearnSkill {
		t.Fatalf("SystemMessage id = %d, want item-missing (%d)", id, serverpackets.SystemMessageItemMissingToLearnSkill)
	}

	reply = c.read()
	if reply[0] != serverpackets.OpcodeAcquireSkillList {
		t.Fatalf("opcode = %#x, want AcquireSkillList (%#x)", reply[0], serverpackets.OpcodeAcquireSkillList)
	}
	r := wire.NewReader(reply[1:])
	if skillType, count := r.ReadInt32(), r.ReadInt32(); skillType != int32(serverpackets.AcquireSkillTypeUsual) || count != 1 {
		t.Fatalf("AcquireSkillList = type %d count %d, want usual with 1 entry", skillType, count)
	}
}

func TestGameClientLinkSendsSkillCoolTimeInGame(t *testing.T) {
	c, _, _, _ := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestSkillCoolTime())
	reply := c.read()
	if reply[0] != serverpackets.OpcodeSkillCoolTime {
		t.Fatalf("opcode = %#x, want SkillCoolTime (%#x)", reply[0], serverpackets.OpcodeSkillCoolTime)
	}
	if count := wire.NewReader(reply[1:]).ReadInt32(); count != 0 {
		t.Fatalf("SkillCoolTime count = %d, want 0", count)
	}

	c.send(encodeRequestManorList())
	reply = c.read()
	if reply[0] != serverpackets.OpcodeExtended {
		t.Fatalf("opcode = %#x, want extended packet (%#x)", reply[0], serverpackets.OpcodeExtended)
	}
	if second := wire.NewReader(reply[1:]).ReadUint16(); second != serverpackets.OpcodeExSendManorList {
		t.Fatalf("extended opcode = %#x, want ExSendManorList (%#x)", second, serverpackets.OpcodeExSendManorList)
	}
}

func TestGameClientLinkAcquireSkillInfoAndLearnGeneralSkill(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := NewSkillPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{ID: 3, Level: 1, Activation: modelskill.ActivationActive},
	}), store)
	var objID int32
	c, _, _, _ := newLinkedGameClientWithSkillsSeed(t, skills, func(chars *fakeCharStore, _ *fakeItemStore) {
		objID = seedSelectableCharacter(t, chars, "player1", "Newbie", 5, 50)
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestAcquireSkillInfo(3, 1, 0))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeAcquireSkillInfo {
		t.Fatalf("skill info opcode = %#x, want AcquireSkillInfo (%#x)", reply[0], serverpackets.OpcodeAcquireSkillInfo)
	}
	r := wire.NewReader(reply[1:])
	if id, level, cost, mode := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != 3 || level != 1 || cost != 50 || mode != 0 {
		t.Fatalf("AcquireSkillInfo header = id %d level %d cost %d mode %d, want 3/1/50/0", id, level, cost, mode)
	}
	if reqs := r.ReadInt32(); reqs != 0 {
		t.Fatalf("AcquireSkillInfo requirements = %d, want 0", reqs)
	}

	c.send(encodeRequestAcquireSkill(3, 1, 0))
	reply = c.read()
	assertSystemMessageSkillFrame(t, reply, serverpackets.SystemMessageLearnedSkill, 3, 1)

	reply = c.read()
	assertSPStatus(t, reply, objID, 0)

	reply = c.read()
	if reply[0] != serverpackets.OpcodeSkillList {
		t.Fatalf("learn SkillList opcode = %#x, want SkillList (%#x)", reply[0], serverpackets.OpcodeSkillList)
	}
	r = wire.NewReader(reply[1:])
	if count := r.ReadInt32(); count != 1 {
		t.Fatalf("SkillList count = %d, want 1", count)
	}
	if passive, level, id := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); passive != 0 || level != 1 || id != 3 {
		t.Fatalf("SkillList entry = passive %d level %d id %d, want 0/1/3", passive, level, id)
	}
	r.ReadUint8()
	if err := r.Err(); err != nil {
		t.Fatalf("read SkillList: %v", err)
	}

	reply = c.read()
	if reply[0] != serverpackets.OpcodeAcquireSkillList {
		t.Fatalf("learn AcquireSkillList opcode = %#x, want AcquireSkillList (%#x)", reply[0], serverpackets.OpcodeAcquireSkillList)
	}
	r = wire.NewReader(reply[1:])
	if skillType, count := r.ReadInt32(), r.ReadInt32(); skillType != int32(serverpackets.AcquireSkillTypeUsual) || count != 0 {
		t.Fatalf("AcquireSkillList = type %d count %d, want usual empty", skillType, count)
	}

	known := store.knownFor(objID, 0)
	if known[3] != 1 {
		t.Fatalf("persisted skill 3 = %d, want 1", known[3])
	}
}

func TestGameClientLinkAcquireSkillNeedsSP(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := NewSkillPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{ID: 3, Level: 1, Activation: modelskill.ActivationActive},
	}), store)
	var objID int32
	c, _, _, _ := newLinkedGameClientWithSkillsSeed(t, skills, func(chars *fakeCharStore, _ *fakeItemStore) {
		objID = seedSelectableCharacter(t, chars, "player1", "Newbie", 5, 49)
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestAcquireSkill(3, 1, 0))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("needs-sp opcode = %#x, want SystemMessage (%#x)", reply[0], serverpackets.OpcodeSystemMessage)
	}
	r := wire.NewReader(reply[1:])
	if id := r.ReadInt32(); id != serverpackets.SystemMessageNotEnoughSPToLearnSkill {
		t.Fatalf("SystemMessage id = %d, want not enough SP", id)
	}

	reply = c.read()
	if reply[0] != serverpackets.OpcodeAcquireSkillList {
		t.Fatalf("needs-sp list opcode = %#x, want AcquireSkillList (%#x)", reply[0], serverpackets.OpcodeAcquireSkillList)
	}
	r = wire.NewReader(reply[1:])
	if skillType, count := r.ReadInt32(), r.ReadInt32(); skillType != int32(serverpackets.AcquireSkillTypeUsual) || count != 1 {
		t.Fatalf("AcquireSkillList = type %d count %d, want usual with 1 entry", skillType, count)
	}
	if id, level, shownLevel, cost, unknown := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != 3 || level != 1 || shownLevel != 1 || cost != 50 || unknown != 0 {
		t.Fatalf("AcquireSkillList entry = %d/%d/%d/%d/%d, want 3/1/1/50/0", id, level, shownLevel, cost, unknown)
	}
	if known := store.knownFor(objID, 0); len(known) != 0 {
		t.Fatalf("persisted skills = %+v, want none", known)
	}
}

// fishingTrees returns a skill tree with one learnable fishing skill: id 1368
// (an Expand Inventory level), level 1, min player level 5, costing 2 of item
// 57 (Adena in the test item table). 1368 falls in the storage-sync range so
// learning it prompts an ExStorageMaxCount packet.
func fishingTrees() *modelskill.Trees {
	return &modelskill.Trees{Fishing: []modelskill.FishingSkill{
		{ID: 1368, Level: 1, MinLevel: 5, ItemID: 57, ItemCount: 2},
	}}
}

func fishingSkills(t *testing.T) *SkillPersistence {
	t.Helper()
	store := newMemorySkillSaveStore()
	return NewSkillPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{ID: 1368, Level: 1, Activation: modelskill.ActivationActive},
	}), store)
}

func enterFishingClient(t *testing.T, trees *modelskill.Trees, skills *SkillPersistence, seedItems func(*fakeItemStore, int32)) (*fakeGameClient, *memorySkillSaveStore, int32) {
	t.Helper()
	store := newMemorySkillSaveStore()
	pers := NewSkillPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{ID: 1368, Level: 1, Activation: modelskill.ActivationActive},
	}), store)
	var objID int32
	c, _, _, _, _ := newLinkedGameClientWithSkillsShortcutsCrestsSeed(t, pers, nil, nil, modelskill.BookPolicy{}, trees, func(chars *fakeCharStore, items *fakeItemStore) {
		objID = seedSelectableCharacter(t, chars, "player1", "Newbie", 5, 50)
		if seedItems != nil {
			seedItems(items, objID)
		}
	}, 1)
	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)
	return c, store, objID
}

// TestAcquireFishingSkillInfo verifies the fishing AcquireSkillInfo carries
// the oracle's tuple (mode 1, sp 0, requirement type 4 / item id / count / 0).
func TestAcquireFishingSkillInfo(t *testing.T) {
	c, _, _ := enterFishingClient(t, fishingTrees(), fishingSkills(t), nil)

	c.send(encodeRequestAcquireSkillInfo(1368, 1, 1))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeAcquireSkillInfo {
		t.Fatalf("opcode = %#x, want AcquireSkillInfo (%#x)", reply[0], serverpackets.OpcodeAcquireSkillInfo)
	}
	r := wire.NewReader(reply[1:])
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
}

// TestLearnFishingSkill verifies consuming the fishing item lets the skill
// land, sends ExStorageMaxCount for the storage-sync range, and persists.
func TestLearnFishingSkill(t *testing.T) {
	c, store, objID := enterFishingClient(t, fishingTrees(), fishingSkills(t), func(items *fakeItemStore, owner int32) {
		if err := items.Create(context.Background(), owner, item.Instance{
			ObjectID: 700, TemplateID: 57, OwnerID: owner, Count: 5, Location: item.LocationInventory,
		}); err != nil {
			t.Fatalf("seed item: %v", err)
		}
	})

	c.send(encodeRequestAcquireSkill(1368, 1, 1))

	reply := c.read()
	assertSystemMessageSkillFrame(t, reply, serverpackets.SystemMessageLearnedSkill, 1368, 1)

	reply = c.read()
	if reply[0] != serverpackets.OpcodeExtended {
		t.Fatalf("opcode = %#x, want extended (%#x)", reply[0], serverpackets.OpcodeExtended)
	}
	if second := wire.NewReader(reply[1:]).ReadUint16(); second != serverpackets.OpcodeExStorageMaxCount {
		t.Fatalf("extended opcode = %#x, want ExStorageMaxCount (%#x)", second, serverpackets.OpcodeExStorageMaxCount)
	}

	reply = c.read()
	if reply[0] != serverpackets.OpcodeSkillList {
		t.Fatalf("opcode = %#x, want SkillList (%#x)", reply[0], serverpackets.OpcodeSkillList)
	}
	r := wire.NewReader(reply[1:])
	if count := r.ReadInt32(); count != 1 {
		t.Fatalf("SkillList count = %d, want 1", count)
	}
	if passive, level, id := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); passive != 0 || level != 1 || id != 1368 {
		t.Fatalf("SkillList entry = passive %d level %d id %d, want 0/1/1368", passive, level, id)
	}

	reply = c.read()
	if reply[0] != serverpackets.OpcodeAcquireSkillList {
		t.Fatalf("opcode = %#x, want AcquireSkillList (%#x)", reply[0], serverpackets.OpcodeAcquireSkillList)
	}
	r = wire.NewReader(reply[1:])
	if skillType, count := r.ReadInt32(), r.ReadInt32(); skillType != int32(serverpackets.AcquireSkillTypeFishing) || count != 0 {
		t.Fatalf("AcquireSkillList = type %d count %d, want fishing empty", skillType, count)
	}

	if known := store.knownFor(objID, 0); known[1368] != 1 {
		t.Fatalf("persisted skill 1368 = %d, want 1", known[1368])
	}
}

// TestLearnFishingSkillBlockedByMissingItem verifies a missing fishing item
// sends the item-missing message and a fishing list, without learning.
func TestLearnFishingSkillBlockedByMissingItem(t *testing.T) {
	c, store, objID := enterFishingClient(t, fishingTrees(), fishingSkills(t), nil)

	c.send(encodeRequestAcquireSkill(1368, 1, 1))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcode = %#x, want SystemMessage (%#x)", reply[0], serverpackets.OpcodeSystemMessage)
	}
	if id := wire.NewReader(reply[1:]).ReadInt32(); id != serverpackets.SystemMessageItemMissingToLearnSkill {
		t.Fatalf("SystemMessage id = %d, want item-missing (%d)", id, serverpackets.SystemMessageItemMissingToLearnSkill)
	}

	reply = c.read()
	if reply[0] != serverpackets.OpcodeAcquireSkillList {
		t.Fatalf("opcode = %#x, want AcquireSkillList (%#x)", reply[0], serverpackets.OpcodeAcquireSkillList)
	}
	r := wire.NewReader(reply[1:])
	if skillType, count := r.ReadInt32(), r.ReadInt32(); skillType != int32(serverpackets.AcquireSkillTypeFishing) || count != 1 {
		t.Fatalf("AcquireSkillList = type %d count %d, want fishing with 1 entry", skillType, count)
	}
	if id, level, shownLevel, cost, marker := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != 1368 || level != 1 || shownLevel != 1 || cost != 0 || marker != 1 {
		t.Fatalf("AcquireSkillList fishing entry = %d/%d/%d/%d/%d, want 1368/1/1/0/1", id, level, shownLevel, cost, marker)
	}
	if known := store.knownFor(objID, 0); len(known) != 0 {
		t.Fatalf("persisted skills = %+v, want none", known)
	}
}
