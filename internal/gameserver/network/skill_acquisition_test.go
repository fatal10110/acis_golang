package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

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
