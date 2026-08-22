package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// consumableSkills builds the skill table the consumable and scroll flows
// resolve their item-carried skills against, plus the boot defaults.
func consumableSkills(t *testing.T) *skillstate.Persistence {
	t.Helper()
	db := sqltest.SharedDB(t)
	store := gamesql.NewSkillSaveStore(db)
	known := gamesql.NewCharacterSkillStore(db)
	return skillstate.NewPersistence(store, modelskill.NewTable([]modelskill.Definition{
		{ID: 248, Level: 3},
		{ID: 294, Level: 1},
		{
			ID: 2031, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "HOT", Potion: true, HitTime: 0,
			Effects: []modelskill.EffectTemplate{{Name: "HealOverTime", Count: 7, Time: 2, Value: 16, Icon: true}},
		},
		{
			ID: 2013, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "TELEPORT", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 5000,
		},
	}), known)
}

// readExUseSharedGroupItem asserts the next frame is the extended
// ExUseSharedGroupItem packet carrying templateID/group/remain/total.
func readExUseSharedGroupItem(t *testing.T, c *testsupport.ScriptedClient, templateID, group, remain, total int32) {
	t.Helper()
	reply := c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeExtended, "extended")
	r := wire.NewReader(reply[1:])
	if sub := r.ReadUint16(); sub != serverpackets.OpcodeExUseSharedGroupItem {
		t.Fatalf("extended sub-opcode = %#x, want ExUseSharedGroupItem (%#x)", sub, serverpackets.OpcodeExUseSharedGroupItem)
	}
	if gotItem, gotGroup, gotRemain, gotTotal := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); gotItem != templateID || gotGroup != group || gotRemain != remain || gotTotal != total {
		t.Fatalf("ExUseSharedGroupItem = item %d group %d remain %d total %d, want %d/%d/%d/%d", gotItem, gotGroup, gotRemain, gotTotal, templateID, group, remain, total)
	}
}

// readExRegenMax asserts the next frame is ExRegenMax describing the
// heal-over-time HUD ticker.
func readExRegenMax(t *testing.T, c *testsupport.ScriptedClient, count, period int32, hpRegen float64) {
	t.Helper()
	reply := c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeExtended, "extended")
	r := wire.NewReader(reply[1:])
	if sub := r.ReadUint16(); sub != serverpackets.OpcodeExRegenMax {
		t.Fatalf("extended sub-opcode = %#x, want ExRegenMax (%#x)", sub, serverpackets.OpcodeExRegenMax)
	}
	if kind, gotCount, gotPeriod, gotRegen := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadFloat64(); kind != 1 || gotCount != count || gotPeriod != period || gotRegen != hpRegen {
		t.Fatalf("ExRegenMax = %d/%d/%d/%g, want 1/%d/%d/%g", kind, gotCount, gotPeriod, gotRegen, count, period, hpRegen)
	}
}

// readAbnormalStatusUpdate asserts the next frame is AbnormalStatusUpdate
// with exactly the given single entry.
func readAbnormalStatusUpdate(t *testing.T, c *testsupport.ScriptedClient, skillID, level, durationSeconds int32) {
	t.Helper()
	reply := c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeAbnormalStatusUpdate, "AbnormalStatusUpdate")
	r := wire.NewReader(reply[1:])
	if n := r.ReadUint16(); n != 1 {
		t.Fatalf("AbnormalStatusUpdate count = %d, want 1", n)
	}
	if sid, lvl := r.ReadInt32(), r.ReadUint16(); sid != skillID || int32(lvl) != level {
		t.Fatalf("AbnormalStatusUpdate entry = %d/%d, want %d/%d", sid, lvl, skillID, level)
	}
	if dur := r.ReadInt32(); dur != durationSeconds {
		t.Fatalf("AbnormalStatusUpdate duration = %d, want %d", dur, durationSeconds)
	}
}

// readShortBuffStatusUpdate asserts the next frame is ShortBuffStatusUpdate
// carrying skillID/level/durationSeconds.
func readShortBuffStatusUpdate(t *testing.T, c *testsupport.ScriptedClient, skillID, level, durationSeconds int32) {
	t.Helper()
	reply := c.Read()
	assertFrameOpcode(t, reply, serverpackets.OpcodeShortBuffStatusUpdate, "ShortBuffStatusUpdate")
	r := wire.NewReader(reply[1:])
	if sid, lvl, dur := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); sid != skillID || lvl != level || dur != durationSeconds {
		t.Fatalf("ShortBuffStatusUpdate = skill %d level %d duration %d, want %d/%d/%d", sid, lvl, dur, skillID, level, durationSeconds)
	}
}

// assertSystemMessageSkill asserts a SystemMessage whose single param is the
// given skill name.
func assertSystemMessageSkill(t *testing.T, frame []byte, messageID, skillID, level int32) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != messageID {
		t.Fatalf("system message id = %d, want %d", got, messageID)
	}
	if params := r.ReadInt32(); params != 1 {
		t.Fatalf("param count = %d, want 1", params)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamSkillName {
		t.Fatalf("param type = %d, want skill", typ)
	}
	if sid, lvl := r.ReadInt32(), r.ReadInt32(); sid != skillID || lvl != level {
		t.Fatalf("skill param = %d/%d, want %d/%d", sid, lvl, skillID, level)
	}
}

// TestUseHealingPotionAppliesAndConsumes drives the healing potion flow:
// the shared-reuse HUD packet, the instant-cast announcement, the effect
// packets, the tick-driven stack decrement in both the packet stream and
// the items row, and the reuse-window rejection for the second use.
func TestUseHealingPotionAppliesAndConsumes(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(consumableSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	potion := srv.GiveItem(t, objID, 1060, 5)
	startInWorld(t, c)

	c.Send(encodeUseItem(potion, false))
	readExUseSharedGroupItem(t, c, 1060, 8, 10, 10)
	assertMagicSkillUseSelf(t, c.Read(), objID, 2031, 1, 0, 0)
	assertSystemMessageSkill(t, c.Read(), serverpackets.SystemMessageUseS1, 2031, 1)
	readExRegenMax(t, c, 14, 2, 16*0.66)
	readAbnormalStatusUpdate(t, c, 2031, 1, 14)
	readShortBuffStatusUpdate(t, c, 2031, 1, 14)

	srv.InventoryUpdates.Tick()
	readInventoryUpdateFor(t, c, potion, 4)
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, potion); inst.Count != 4 {
		t.Fatalf("persisted potion count = %d, want 4", inst.Count)
	}

	c.Send(encodeUseItem(potion, false))
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "reuse SystemMessage")
	if id := systemMessageID(t, frame); id != serverpackets.SystemMessageS1PreparedForReuse {
		t.Fatalf("reuse message id = %d, want S1PreparedForReuse (%d)", id, serverpackets.SystemMessageS1PreparedForReuse)
	}
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "reuse ActionFailed")
	if inst := mustFindItem(t, srv, objID, potion); inst.Count != 4 {
		t.Fatalf("potion count after rejected reuse = %d, want 4", inst.Count)
	}
}

// TestUseEscapeScrollRunsAICastAndConsumes drives the non-potion
// item-carried skill path: the scroll's skill is announced against the
// player's own object with its real reuse delay, and one unit of the scroll
// is consumed once the cast starts.
func TestUseEscapeScrollRunsAICastAndConsumes(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(consumableSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	scroll := srv.GiveItem(t, objID, 736, 3)
	startInWorld(t, c)

	c.Send(encodeUseItem(scroll, false))
	assertMagicSkillUseSelf(t, c.Read(), objID, 2013, 1, 0, 5000)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillLaunched, "MagicSkillLaunched")

	srv.InventoryUpdates.Tick()
	readInventoryUpdateFor(t, c, scroll, 2)
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, scroll); inst.Count != 2 {
		t.Fatalf("persisted scroll count = %d, want 2", inst.Count)
	}
}

// TestUseItemGates walks the use-gate rejection table that is reachable
// from the wire: unknown object, quest item, store-operation state, and the
// dead gate. Each rejection answers instead of silently dropping the click,
// and the connection stays responsive.
func TestUseItemGates(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(consumableSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	potion := srv.GiveItem(t, objID, 1060, 1)
	token := srv.GiveItem(t, objID, 9001, 1)
	startInWorld(t, c)

	t.Run("unknown object", func(t *testing.T) {
		c.Send(encodeUseItem(99999, false))
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "unknown item")
		barrier(t, c)
	})

	t.Run("quest item", func(t *testing.T) {
		c.Send(encodeUseItem(token, false))
		assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCannotUseQuestItems)
		barrier(t, c)
	})

	t.Run("operating a store", func(t *testing.T) {
		srv.SetPlayerOperating(t, objID, true)
		c.Send(encodeUseItem(potion, false))
		assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageItemsUnavailableForStore)
		srv.SetPlayerOperating(t, objID, false)
		barrier(t, c)
	})

	t.Run("dead", func(t *testing.T) {
		srv.MarkPlayerDead(t, objID)
		c.Send(encodeUseItem(potion, false))
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "dead gate")
		barrier(t, c)
		if inst := mustFindItem(t, srv, objID, potion); inst.Count != 1 {
			t.Fatalf("potion count after gated uses = %d, want 1", inst.Count)
		}
	})
}

func barrier(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	testsupport.SyncBarrier(t, c, func() { c.Send(encodeRequestItemList()) }, serverpackets.OpcodeItemList)
}
