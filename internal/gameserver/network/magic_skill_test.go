package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func TestGameClientLinkMagicSkillUseStartsKnownActiveSkill(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := NewSkillPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: 500, ReuseDelay: 1200, StaticHitTime: true, StaticReuse: true,
			MPInitialConsume: 2, MPConsume: 3, SkillType: "DUMMY",
		},
	}), store)
	var objID int32
	c, _, _, _ := newLinkedGameClientWithSkillsSeed(t, skills, func(chars *fakeCharStore, _ *fakeItemStore) {
		objID = seedSelectableCharacter(t, chars, "player1", "Newbie", 5, 0)
		store.seedKnown(objID, 0, player.SkillLevels{3: 1})
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestMagicSkillUse(3, false, false))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeMagicSkillUse {
		t.Fatalf("magic use opcode = %#x, want MagicSkillUse (%#x)", reply[0], serverpackets.OpcodeMagicSkillUse)
	}
	r := wire.NewReader(reply[1:])
	if caster, target, skillID, level := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster != objID || target != objID || skillID != 3 || level != 1 {
		t.Fatalf("MagicSkillUse ids = caster %d target %d skill %d level %d, want %d/%d/3/1", caster, target, skillID, level, objID, objID)
	}
	if hitTime, reuse := r.ReadInt32(), r.ReadInt32(); hitTime != 500 || reuse != 1200 {
		t.Fatalf("MagicSkillUse timing = hit %d reuse %d, want 500/1200", hitTime, reuse)
	}

	reply = c.read()
	assertSystemMessageSkillFrame(t, reply, serverpackets.SystemMessageUseS1, 3, 1)

	reply = c.read()
	if reply[0] != serverpackets.OpcodeSetupGauge {
		t.Fatalf("setup gauge opcode = %#x, want SetupGauge (%#x)", reply[0], serverpackets.OpcodeSetupGauge)
	}
	r = wire.NewReader(reply[1:])
	if color, current, maxTime := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); color != int32(serverpackets.GaugeBlue) || current != 500 || maxTime != 500 {
		t.Fatalf("SetupGauge = color %d current %d max %d, want blue/500/500", color, current, maxTime)
	}

	reply = c.read()
	if reply[0] != serverpackets.OpcodeMagicSkillLaunched {
		t.Fatalf("magic launched opcode = %#x, want MagicSkillLaunched (%#x)", reply[0], serverpackets.OpcodeMagicSkillLaunched)
	}
	r = wire.NewReader(reply[1:])
	if caster, skillID, level, count, target := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster != objID || skillID != 3 || level != 1 || count != 1 || target != objID {
		t.Fatalf("MagicSkillLaunched = caster %d skill %d level %d count %d target %d, want %d/3/1/1/%d", caster, skillID, level, count, target, objID, objID)
	}

	reply = c.read()
	assertStatusAttrs(t, reply, objID, []serverpackets.StatusAttribute{{Type: serverpackets.StatusCurrentMP, Value: 25}})
}

func TestGameClientLinkMagicSkillUseRejectsInsufficientMP(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := NewSkillPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			MPConsume: 100, SkillType: "DUMMY",
		},
	}), store)
	var objID int32
	c, _, _, _ := newLinkedGameClientWithSkillsSeed(t, skills, func(chars *fakeCharStore, _ *fakeItemStore) {
		objID = seedSelectableCharacter(t, chars, "player1", "Newbie", 5, 0)
		store.seedKnown(objID, 0, player.SkillLevels{3: 1})
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestMagicSkillUse(3, false, false))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("not-enough-mp opcode = %#x, want SystemMessage (%#x)", reply[0], serverpackets.OpcodeSystemMessage)
	}
	r := wire.NewReader(reply[1:])
	if id := r.ReadInt32(); id != serverpackets.SystemMessageNotEnoughMP {
		t.Fatalf("SystemMessage id = %d, want not enough MP", id)
	}
	if params := r.ReadInt32(); params != 0 {
		t.Fatalf("SystemMessage params = %d, want 0", params)
	}

	reply = c.read()
	if reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("after not-enough-mp opcode = %#x, want ActionFailed (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}
}
