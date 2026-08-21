//go:build integration

package network

import (
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
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
	c, _, _, _, _, _ := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		objID = seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
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

func TestGameClientLinkMagicSkillUseRecordsCtrlShiftModifiers(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: 500, ReuseDelay: 1200, StaticHitTime: true, StaticReuse: true,
			MPInitialConsume: 2, MPConsume: 3, SkillType: "DUMMY",
		},
	}), store)
	var objID int32
	c, _, _, _, _, state := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		objID = seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
		store.seedKnown(objID, 0, player.SkillLevels{3: 1})
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestMagicSkillUse(3, true, true))
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
	if ctrl, shift := character.CastModifiers(); !ctrl || !shift {
		t.Fatalf("CastModifiers() = (%v,%v), want (true,true)", ctrl, shift)
	}
}

func TestGameClientLinkMagicSkillUseRecordsCtrlShiftModifiersOnRejection(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			MPConsume: 100, SkillType: "DUMMY",
		},
	}), store)
	var objID int32
	c, _, _, _, _, state := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		objID = seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
		store.seedKnown(objID, 0, player.SkillLevels{3: 1})
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestMagicSkillUse(3, true, true))
	c.read() // SystemMessage (not enough MP)
	c.read() // ActionFailed

	obj, ok := state.Player(objID)
	if !ok {
		t.Fatalf("player %d not found in world state after rejected cast", objID)
	}
	character, ok := obj.(*livePlayer)
	if !ok {
		t.Fatalf("world state player %d is not a *livePlayer", objID)
	}
	if ctrl, shift := character.CastModifiers(); !ctrl || !shift {
		t.Fatalf("CastModifiers() after rejected cast = (%v,%v), want (true,true)", ctrl, shift)
	}
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
	c, _, _, _, _, state := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		objID = seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
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

func TestGameClientLinkEffectStanceBroadcasts(t *testing.T) {
	var objID int32
	c, _, _, _, _, state := newLinkedSQLGameClient(t, nil, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		objID = seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
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
	live, ok := obj.(*livePlayer)
	if !ok {
		t.Fatalf("world state player %d is not a *livePlayer", objID)
	}
	start := func(name string, want serverpackets.WaitType) *effect.Effect {
		t.Helper()
		e, err := effect.New(effect.Skill{}, modelskill.EffectTemplate{Name: name})
		if err != nil {
			t.Fatalf("New(%s): %v", name, err)
		}
		e.Effected = live.Character
		if !e.OnStart(e) {
			t.Fatalf("%s start rejected live character", name)
		}
		frame := c.read()
		if frame[0] != serverpackets.OpcodeChangeWaitType {
			t.Fatalf("%s opcode = %#x, want ChangeWaitType", name, frame[0])
		}
		r := wire.NewReader(frame[1:])
		if got := r.ReadInt32(); got != objID {
			t.Fatalf("%s object id = %d, want %d", name, got, objID)
		}
		if got := r.ReadInt32(); got != int32(want) {
			t.Fatalf("%s wait type = %d, want %d", name, got, want)
		}
		return e
	}

	start("Relax", serverpackets.WaitSitting)
	start("ChameleonRest", serverpackets.WaitSitting)
	fakeDeath := start("FakeDeath", serverpackets.WaitFakeDeathStart)
	if frame := c.read(); frame[0] != serverpackets.OpcodeAbnormalStatusUpdate {
		t.Fatalf("fake death start follow-up opcode = %#x, want AbnormalStatusUpdate", frame[0])
	}
	fakeDeath.OnExit(fakeDeath)
	frame := c.read()
	if frame[0] != serverpackets.OpcodeChangeWaitType {
		t.Fatalf("fake death exit opcode = %#x, want ChangeWaitType", frame[0])
	}
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != objID {
		t.Fatalf("fake death exit object id = %d, want %d", got, objID)
	}
	if got := r.ReadInt32(); got != int32(serverpackets.WaitFakeDeathStop) {
		t.Fatalf("fake death exit wait type = %d, want %d", got, serverpackets.WaitFakeDeathStop)
	}
	if frame := c.read(); frame[0] != serverpackets.OpcodeRevive {
		t.Fatalf("fake death exit opcode = %#x, want Revive", frame[0])
	}
}

func TestGameClientLinkMagicSkillUseGroundRecordsGroundTargetAndAppliesEffect(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 5, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetGround,
			CastRange: 3000, HitTime: 500, ReuseDelay: 1200, StaticHitTime: true, StaticReuse: true,
			MPInitialConsume: 2, MPConsume: 3, SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60}},
		},
	}), store)
	var objID int32
	c, _, _, _, _, state := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		objID = seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
		store.seedKnown(objID, 0, player.SkillLevels{5: 1})
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestExMagicSkillUseGround(1000, 2000, 300, 5, false, false))
	c.read() // ValidateLocation
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
	if x, y, z := character.GroundTarget(); x != 1000 || y != 2000 || z != 300 {
		t.Fatalf("GroundTarget() = (%d,%d,%d), want (1000,2000,300)", x, y, z)
	}
	effects := character.EffectList().All()
	if len(effects) != 1 || effects[0].Skill.ID != 5 {
		t.Fatalf("effects after ground-target cast = %+v, want one effect from skill 5", effects)
	}
}

func TestGameClientLinkMagicSkillUseGroundWalksIntoCastRange(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{{
		ID: 5, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetGround,
		CastRange: 900, HitTime: 500, StaticHitTime: true, SkillType: "SIGNET",
	}}), store)
	var id int32
	c, _, _, _, _, state := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		id = seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
		store.seedKnown(id, 0, player.SkillLevels{5: 1})
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read()
	c.read()
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)
	obj, ok := state.Player(id)
	if !ok {
		t.Fatalf("player %d not found in world state", id)
	}
	obj.(*livePlayer).Move().SetSpeed(100000)

	c.send(encodeRequestExMagicSkillUseGround(5000, 0, 0, 5, false, false))
	if reply := c.read(); reply[0] != serverpackets.OpcodeMoveToLocation {
		t.Fatalf("out-of-range ground cast opcode = %#x, want MoveToLocation (%#x)", reply[0], serverpackets.OpcodeMoveToLocation)
	}
	if reply := c.read(); reply[0] != serverpackets.OpcodeValidateLocation {
		t.Fatalf("post-walk ground cast opcode = %#x, want ValidateLocation (%#x)", reply[0], serverpackets.OpcodeValidateLocation)
	}
	if reply := c.read(); reply[0] != serverpackets.OpcodeMagicSkillUse {
		t.Fatalf("post-walk ground cast opcode = %#x, want MagicSkillUse (%#x)", reply[0], serverpackets.OpcodeMagicSkillUse)
	}
}

func TestGameClientLinkMagicSkillUseGroundShiftRejectsOutOfRangeTarget(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{{
		ID: 5, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetGround,
		CastRange: 900, HitTime: 500, StaticHitTime: true, SkillType: "SIGNET",
	}}), store)
	c, _, _, _, _, _ := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		id := seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
		store.seedKnown(id, 0, player.SkillLevels{5: 1})
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read()
	c.read()
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestExMagicSkillUseGround(5000, 0, 0, 5, false, true))
	assertStaticSystemMessageFrame(t, c.read(), serverpackets.SystemMessageTargetTooFar)
}

func TestGameClientLinkMagicSkillUseGroundSilentlyIgnoresNonGroundSkill(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 5, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: 500, ReuseDelay: 1200, StaticHitTime: true, StaticReuse: true,
			MPInitialConsume: 2, MPConsume: 3, SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60}},
		},
	}), store)
	var objID int32
	c, _, _, _, _, state := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		objID = seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
		store.seedKnown(objID, 0, player.SkillLevels{5: 1})
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestExMagicSkillUseGround(1000, 2000, 300, 5, false, false))
	if reply := c.readWithTimeout(300 * time.Millisecond); reply != nil {
		t.Fatalf("non-ground ground-cast reply = opcode %#x, want no reply", reply[0])
	}

	obj, ok := state.Player(objID)
	if !ok {
		t.Fatalf("player %d not found in world state after request", objID)
	}
	character, ok := obj.(*livePlayer)
	if !ok {
		t.Fatalf("world state player %d is not a *livePlayer", objID)
	}
	if x, y, z := character.GroundTarget(); x != 0 || y != 0 || z != 0 {
		t.Fatalf("GroundTarget() = (%d,%d,%d), want unchanged (0,0,0)", x, y, z)
	}

	c.send(encodeRequestExMagicSkillUseGround(1000, 2000, 300, 9999, false, false))
	if reply := c.readWithTimeout(300 * time.Millisecond); reply != nil {
		t.Fatalf("unlearned ground-cast reply = opcode %#x, want no reply", reply[0])
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
	c, _, _, _, _, state := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		objID = seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
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
	c, _, _, _, _, state := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		objID = seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
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
	assertEventuallyEffect(t, character, 5123, 1)
}

// TestGameClientLinkTogglesOnThenOff reproduces recasting a toggle skill
// twice: the first cast has no active instance yet, so it pays the MP cost
// and applies the buff; the second cast finds that instance still active,
// so it turns it off at no cost instead of refreshing it. Both casts send
// only the one MagicSkillUse packet (caster and target both the caster,
// hitTime/reuseDelay both 0) — no SystemMessage, SetupGauge, or
// MagicSkillLaunched, since a toggle's cast window is instantaneous and
// carries no cast bar. On both directions the MagicSkillUse ack lands
// before any AbnormalStatusUpdate the effect add/remove triggers, matching
// PlayerCast.doToggleCast's broadcast at PlayerCast.java:127 preceding
// either callSkill's effect application or effect.exit() at
// PlayerCast.java:135-137.
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
	c, _, _, _, _, state := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		objID = seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
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

	// First cast: no active instance yet, activates and pays MP. The
	// reference broadcasts MagicSkillUse before applying the skill's
	// effects, so the ack packet lands before any AbnormalStatusUpdate the
	// activation's effect triggers.
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
	if entries := readAbnormalStatusUpdateFrame(t, c); len(entries) != 0 {
		t.Fatalf("AbnormalStatusUpdate entries after activation = %+v, want none (test skill has no icon)", entries)
	}
	c.expectNoFrame()

	effects := character.EffectList().All()
	if len(effects) != 1 || effects[0].Skill.ID != 288 {
		t.Fatalf("effects after toggle activation = %+v, want one effect from skill 288", effects)
	}

	// Second cast: an instance is already active, so this turns it off
	// instead of reapplying it, and never touches MP. The reference
	// broadcasts MagicSkillUse before effect.exit() on the deactivation
	// path too (PlayerCast.java:127 vs 135-137), so the ack packet must
	// land before the effect-exit AbnormalStatusUpdate here as well.
	beforeMP := character.MP()
	c.send(encodeRequestMagicSkillUse(288, false, false))
	reply = c.read()
	if reply[0] != serverpackets.OpcodeMagicSkillUse {
		t.Fatalf("magic use opcode = %#x, want MagicSkillUse (%#x)", reply[0], serverpackets.OpcodeMagicSkillUse)
	}
	if entries := readAbnormalStatusUpdateFrame(t, c); len(entries) != 0 {
		t.Fatalf("AbnormalStatusUpdate entries after deactivation = %+v, want none (test skill has no icon)", entries)
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

func TestGameClientLinkToggleCostFailuresBroadcastCastAbort(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{ID: 289, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf, MPConsume: 100000},
		{ID: 290, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf, HPConsume: 100000},
	}), store)
	var objID int32
	c, _, _, _, _, _ := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		objID = seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
		store.seedKnown(objID, 0, player.SkillLevels{289: 1, 290: 1})
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	for _, test := range []struct {
		skillID int32
		message int32
	}{
		{289, serverpackets.SystemMessageNotEnoughMP},
		{290, serverpackets.SystemMessageNotEnoughHP},
	} {
		t.Run(fmt.Sprintf("skill-%d", test.skillID), func(t *testing.T) {
			c.send(encodeRequestMagicSkillUse(test.skillID, false, false))
			if frame := c.read(); frame[0] != serverpackets.OpcodeMagicSkillUse {
				t.Fatalf("first opcode = %#x, want MagicSkillUse (%#x)", frame[0], serverpackets.OpcodeMagicSkillUse)
			}
			frame := c.read()
			if frame[0] != serverpackets.OpcodeSystemMessage {
				t.Fatalf("second opcode = %#x, want SystemMessage (%#x)", frame[0], serverpackets.OpcodeSystemMessage)
			}
			if got := wire.NewReader(frame[1:]).ReadInt32(); got != test.message {
				t.Fatalf("SystemMessage id = %d, want %d", got, test.message)
			}
			if frame := c.read(); frame[0] != serverpackets.OpcodeMagicSkillCanceled {
				t.Fatalf("third opcode = %#x, want MagicSkillCanceled (%#x)", frame[0], serverpackets.OpcodeMagicSkillCanceled)
			}
			if frame := c.read(); frame[0] != serverpackets.OpcodeActionFailed {
				t.Fatalf("fourth opcode = %#x, want ActionFailed (%#x)", frame[0], serverpackets.OpcodeActionFailed)
			}
			c.expectNoFrame()
		})
	}
}

// TestGameClientLinkTogglesRejectAllSkillsDisabled covers the blanket-lock
// gate on toggle activation (Controller.CanCastToggle, toggle.go). Java
// routes toggle activation through the same RequestMagicSkillUse ->
// tryToCast path as any other skill (RequestMagicSkillUse.java:24-68 has no
// toggle branch), so a stunned caster is rejected by
// PlayableAI.tryToCast's denyAiAction() check (PlayableAI.java:299-303)
// before the toggle's own logic runs; toggles get no exemption from the
// blanket lock in the reference.
func TestGameClientLinkTogglesRejectAllSkillsDisabled(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 288, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf,
			MPConsume: 12, SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60}},
		},
	}), store)
	var objID int32
	c, _, _, _, _, state := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		objID = seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
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

	e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: "Stun"})
	if err != nil {
		t.Fatal(err)
	}
	character.Character.EffectList().Add(e)
	c.read() // AbnormalStatusUpdate from the Stun effect landing

	beforeMP := character.MP()
	c.send(encodeRequestMagicSkillUse(288, false, false))
	if reply := c.read(); reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("reply opcode = %#x, want ActionFailed only (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}
	c.expectNoFrame()

	if got := character.MP(); got != beforeMP {
		t.Fatalf("MP after rejected toggle = %d, want unchanged %d", got, beforeMP)
	}
	if effects := character.EffectList().All(); len(effects) != 1 {
		t.Fatalf("effects after rejected toggle = %+v, want only the Stun effect", effects)
	}
}

func assertEventuallyEffect(t *testing.T, character *livePlayer, skillID modelskill.ID, level int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		effects := character.EffectList().All()
		if len(effects) == 1 && effects[0].Skill.ID == skillID && effects[0].Skill.Level == level {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("effects after effectId self-cast BUFF = %+v, want one effect from skill %d level %d", effects, skillID, level)
		}
		time.Sleep(10 * time.Millisecond)
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
	c, _, _, _, _, _ := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		objID = seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
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

func TestGameClientLinkMagicSkillUseCubicCastBroadcastsCharacterInfo(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 10, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON", IsCubic: true, NpcID: 1,
			CubicActivationTime: 8, CubicActivationChance: 30, SummonTotalLifeTime: 1200000,
			StaticHitTime: true, StaticReuse: true,
		},
	}), store)
	var objID int32
	c, _, _, _, _, state := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		objID = seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
		store.seedKnown(objID, 0, player.SkillLevels{10: 1})
	}, 1)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestMagicSkillUse(10, false, false))
	c.read() // MagicSkillUse
	c.read() // SystemMessage UseS1
	c.read() // MagicSkillLaunched

	reply := c.read()
	if reply[0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("opcode = %#x, want UserInfo (%#x): a new cubic must refresh the caster's character info", reply[0], serverpackets.OpcodeUserInfo)
	}

	obj, ok := state.Player(objID)
	if !ok {
		t.Fatalf("player %d not found in world state after cast", objID)
	}
	character, ok := obj.(*livePlayer)
	if !ok {
		t.Fatalf("world state player %d is not a *livePlayer", objID)
	}
	if ids := character.Character.CubicIDs(); len(ids) != 1 || ids[0] != 1 {
		t.Fatalf("CubicIDs() after cast = %v, want [1]", ids)
	}
}

func TestGameClientLinkMagicSkillUseRejectsCubicCastWhenListFull(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 11, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON", IsCubic: true, NpcID: 2,
			StaticHitTime: true, StaticReuse: true,
		},
	}), store)
	var objID int32
	c, _, _, _, _, state := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		objID = seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
		store.seedKnown(objID, 0, player.SkillLevels{11: 1})
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
	// Already at the default cap (no Cubic Mastery): one active cubic.
	character.Character.AddOrRefreshCubic(1, false)

	c.send(encodeRequestMagicSkillUse(11, false, false))
	assertStaticSystemMessageFrame(t, c.read(), serverpackets.SystemMessageCubicSummoningFailed)
	if reply := c.read(); reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("follow-up opcode = %#x, want ActionFailed (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}
	if ids := character.Character.CubicIDs(); len(ids) != 1 || ids[0] != 1 {
		t.Fatalf("CubicIDs() after rejected cast = %v, want unchanged [1]", ids)
	}
}

// TestGameClientLinkMagicSkillUseRejectsAllSkillsDisabled covers the
// blanket-lock cast-start gate (Controller.CanCast, controller.go:226-228):
// a stunned caster gets ActionFailed only, no reason message. Java's
// PlayableAI.tryToCast (PlayableAI.java:299-303) rejects a CC'd caster via
// denyAiAction() before canAttemptCast/isSkillDisabled ever run, so the
// S1_PREPARED_FOR_REUSE branch (CreatureCast.java:324-327) never fires for
// this case; PlayerAI.clientActionFailed() (PlayerAI.java:556-560) sends
// only ActionFailed.
func TestGameClientLinkMagicSkillUseRejectsAllSkillsDisabled(t *testing.T) {
	store := newMemorySkillSaveStore()
	skills := skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{
			ID: 12, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "DUMMY", StaticHitTime: true, StaticReuse: true,
		},
	}), store)
	var objID int32
	c, _, _, _, _, state := newLinkedSQLGameClient(t, skills, func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		objID = seedSelectableSQLCharacter(t, chars, "player1", "Newbie", 5, 0).ID
		store.seedKnown(objID, 0, player.SkillLevels{12: 1})
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
	e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: "Stun"})
	if err != nil {
		t.Fatal(err)
	}
	character.Character.EffectList().Add(e)
	c.read() // AbnormalStatusUpdate from the Stun effect landing

	c.send(encodeRequestMagicSkillUse(12, false, false))
	if reply := c.read(); reply[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("reply opcode = %#x, want ActionFailed only (%#x)", reply[0], serverpackets.OpcodeActionFailed)
	}
}
