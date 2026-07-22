package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
)

func TestGameClientLinkMagicSkillUseStartsKnownActiveSkill(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
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

func TestGameClientLinkMagicSkillUseAppliesBuffEffectToSelf(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 4, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: 500, ReuseDelay: 1200, StaticHitTime: true, StaticReuse: true,
			MPInitialConsume: 2, MPConsume: 3, SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60}},
		},
	}), store)
	var objID int32
	c, _, _, state := newLinkedGameClientWithSkillsSeed(t, skills, func(chars *fakeCharStore, _ *fakeItemStore) {
		objID = seedSelectableCharacter(t, chars, "player1", "Newbie", 5, 0)
		store.seedKnown(objID, 0, player.SkillLevels{4: 1})
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestMagicSkillUse(4, false, false))
	c.read() // MagicSkillUse
	c.read() // SystemMessage
	c.read() // SetupGauge
	c.read() // MagicSkillLaunched
	c.read() // StatusUpdate

	obj, ok := state.Player(objID)
	if !ok {
		t.Fatalf("player %d not found in world state after cast", objID)
	}
	character, ok := obj.(*livePlayer)
	if !ok {
		t.Fatalf("world state player %d is not a *livePlayer", objID)
	}
	effects := character.EffectList().All()
	if len(effects) != 1 || effects[0].Skill.ID != 4 {
		t.Fatalf("effects after self-cast BUFF = %+v, want one effect from skill 4", effects)
	}
}

func TestGameClientLinkMagicSkillUseSendsAttackFailedWhenContinuousSkillDoesNotLand(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 5, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "DEBUFF", EffectType: "DEBUFF", Debuff: true,
			BaseLandRate: 0, IgnoreResists: true,
			Effects: []modelskill.EffectTemplate{{Name: "Debuff", Time: 60}},
		},
	}), store)
	var objID int32
	c, _, _, state := newLinkedGameClientWithSkillsSeed(t, skills, func(chars *fakeCharStore, _ *fakeItemStore) {
		objID = seedSelectableCharacter(t, chars, "player1", "Newbie", 5, 0)
		store.seedKnown(objID, 0, player.SkillLevels{5: 1})
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestMagicSkillUse(5, false, false))
	c.read() // MagicSkillUse
	c.read() // SystemMessage UseS1
	c.read() // MagicSkillLaunched
	assertStaticSystemMessageFrame(t, c.read(), serverpackets.SystemMessageAttackFailed)

	obj, ok := state.Player(objID)
	if !ok {
		t.Fatalf("player %d not found in world state after cast", objID)
	}
	character, ok := obj.(*livePlayer)
	if !ok {
		t.Fatalf("world state player %d is not a *livePlayer", objID)
	}
	if effects := character.EffectList().All(); len(effects) != 0 {
		t.Fatalf("effects after failed DEBUFF = %+v, want none", effects)
	}
}

func TestGameClientLinkMagicSkillUseAppliesReferencedEffectSkillAtFallbackLevel(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 454, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", EffectID: 5123,
		},
		{
			ID: 5123, Level: 1, SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60}},
		},
	}), store)
	var objID int32
	c, _, _, state := newLinkedGameClientWithSkillsSeed(t, skills, func(chars *fakeCharStore, _ *fakeItemStore) {
		objID = seedSelectableCharacter(t, chars, "player1", "Newbie", 5, 0)
		store.seedKnown(objID, 0, player.SkillLevels{454: 1})
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestMagicSkillUse(454, false, false))
	c.read() // MagicSkillUse
	c.read() // SystemMessage
	c.read() // MagicSkillLaunched

	obj, ok := state.Player(objID)
	if !ok {
		t.Fatalf("player %d not found in world state after cast", objID)
	}
	character, ok := obj.(*livePlayer)
	if !ok {
		t.Fatalf("world state player %d is not a *livePlayer", objID)
	}
	effects := character.EffectList().All()
	if len(effects) != 1 || effects[0].Skill.ID != 5123 || effects[0].Skill.Level != 1 {
		t.Fatalf("effects after effectId self-cast BUFF = %+v, want one effect from skill 5123 level 1", effects)
	}
}

// TestGameClientLinkTogglesOnThenOff reproduces recasting a toggle skill
// twice: the first cast has no active instance yet, so it pays the MP cost
// and applies the buff; the second cast finds that instance still active,
// so it turns it off at no cost instead of refreshing it. Both casts send
// only the one MagicSkillUse packet (caster and target both the caster,
// hitTime/reuseDelay both 0) — no SystemMessage, SetupGauge, or
// MagicSkillLaunched, since a toggle's cast window is instantaneous and
// carries no cast bar.
func TestGameClientLinkTogglesOnThenOff(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 288, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf,
			MPConsume: 12, SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60}},
		},
	}), store)
	var objID int32
	c, _, _, state := newLinkedGameClientWithSkillsSeed(t, skills, func(chars *fakeCharStore, _ *fakeItemStore) {
		objID = seedSelectableCharacter(t, chars, "player1", "Newbie", 5, 0)
		store.seedKnown(objID, 0, player.SkillLevels{288: 1})
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	obj, ok := state.Player(objID)
	if !ok {
		t.Fatalf("player %d not found in world state", objID)
	}
	character, ok := obj.(*livePlayer)
	if !ok {
		t.Fatalf("world state player %d is not a *livePlayer", objID)
	}

	// First cast: no active instance yet, activates and pays MP.
	c.send(encodeRequestMagicSkillUse(288, false, false))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeMagicSkillUse {
		t.Fatalf("magic use opcode = %#x, want MagicSkillUse (%#x)", reply[0], serverpackets.OpcodeMagicSkillUse)
	}
	r := wire.NewReader(reply[1:])
	if caster, target, skillID, level := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster != objID || target != objID || skillID != 288 || level != 1 {
		t.Fatalf("MagicSkillUse ids = caster %d target %d skill %d level %d, want %d/%d/288/1", caster, target, skillID, level, objID, objID)
	}
	if hitTime, reuse := r.ReadInt32(), r.ReadInt32(); hitTime != 0 || reuse != 0 {
		t.Fatalf("MagicSkillUse timing = hit %d reuse %d, want 0/0", hitTime, reuse)
	}
	c.expectNoFrame()

	effects := character.EffectList().All()
	if len(effects) != 1 || effects[0].Skill.ID != 288 {
		t.Fatalf("effects after toggle activation = %+v, want one effect from skill 288", effects)
	}

	// Second cast: an instance is already active, so this turns it off
	// instead of reapplying it, and never touches MP.
	beforeMP := character.MP()
	c.send(encodeRequestMagicSkillUse(288, false, false))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeMagicSkillUse {
		t.Fatalf("magic use opcode = %#x, want MagicSkillUse (%#x)", reply[0], serverpackets.OpcodeMagicSkillUse)
	}
	c.expectNoFrame()

	if got := character.MP(); got != beforeMP {
		t.Fatalf("MP after toggle deactivation = %d, want unchanged %d", got, beforeMP)
	}
	effects = character.EffectList().All()
	if len(effects) != 0 {
		t.Fatalf("effects after toggle deactivation = %+v, want none", effects)
	}
}

func TestGameClientLinkMagicSkillUseRejectsInsufficientMP(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
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
