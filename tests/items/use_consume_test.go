package items

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
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
			ID: 2279, Level: 2, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "HOT", Potion: true, HitTime: 0,
			Effects: []modelskill.EffectTemplate{{Name: "ManaHeal", Value: 20}},
		},
		{
			ID: 2165, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "HOT", Potion: true, HitTime: 0,
			Effects: []modelskill.EffectTemplate{{Name: "IncreaseCharges", Value: 1, Count: 2}},
		},
		{
			ID: 2031, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "HOT", Potion: true, HitTime: 0,
			Effects: []modelskill.EffectTemplate{{Name: "HealOverTime", Count: 7, Time: 2, Value: 16, Icon: true}},
		},
		{
			ID: 2013, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "TELEPORT", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 5000,
		},
		{
			ID: 2014, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", StaticHitTime: true, HitTime: 0, StaticReuse: true,
		},
		{
			ID: 2015, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", StaticHitTime: true, HitTime: 0, StaticReuse: true,
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

// TestUseTwoSkillItemQueuesLaterAICast pins ItemSkills.java:52-80 plus
// PlayableAI.tryToCast: a two-skill non-instant template starts the first
// eligible skill immediately and stores the later one as the next CAST
// intention, which runs when the first cast finishes. Each routed cast
// consumes one stack unit.
func TestUseTwoSkillItemQueuesLaterAICast(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(consumableSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	scroll := srv.GiveItem(t, objID, gameservertest.TwoSkillScrollID, 2)
	startInWorld(t, c)

	c.Send(encodeUseItem(scroll, false))
	assertMagicSkillUseSelf(t, c.Read(), objID, 2014, 1, 0, 0)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "queued later item skill")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillLaunched, "first MagicSkillLaunched")
	assertMagicSkillUseSelf(t, c.Read(), objID, 2015, 1, 0, 0)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillLaunched, "second MagicSkillLaunched")

	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)
	srv.FlushItems(t)
	for _, inst := range persistedItems(t, srv, objID) {
		if inst.ObjectID == scroll {
			t.Fatalf("persisted two-skill scroll count = %d, want consumed", inst.Count)
		}
	}
}

// TestUseTwoSkillItemLaterSkillReuseStillLaunchesFirst pins that a later
// attached skill still on reuse answers S1_PREPARED_FOR_REUSE without
// cancelling the first skill's already-started cast timeline.
func TestUseTwoSkillItemLaterSkillReuseStillLaunchesFirst(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(consumableSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	scroll := srv.GiveItem(t, objID, gameservertest.TwoSkillScrollID, 2)
	startInWorld(t, c)
	disablePlayerSkill(t, srv, objID, modelskill.Definition{ID: 2015, Level: 1}, time.Minute)

	c.Send(encodeUseItem(scroll, false))
	assertMagicSkillUseSelf(t, c.Read(), objID, 2014, 1, 0, 0)
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "later-skill reuse")
	if id := systemMessageID(t, frame); id != serverpackets.SystemMessageS1PreparedForReuse {
		t.Fatalf("reuse message id = %d, want S1PreparedForReuse (%d)", id, serverpackets.SystemMessageS1PreparedForReuse)
	}
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillLaunched, "first MagicSkillLaunched")
	drainUntilQuiet(t, c)
	if playerCastingNow(t, srv, objID) {
		t.Fatal("CastingNow() = true after first skill finished, want first cast still scheduled to completion")
	}
}

// TestUseTwoSkillItemWhileAttackingCastsOnlyLast pins next-intention
// overwrite while a swing is active: both attached skills queue, the later
// one replaces the earlier, and only that last skill runs when the swing
// finishes — one stack unit consumed.
func TestUseTwoSkillItemWhileAttackingCastsOnlyLast(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(consumableSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	scroll := srv.GiveItem(t, objID, gameservertest.TwoSkillScrollID, 2)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: 40, Y: 20, Z: 30})
	drainUntilQuiet(t, c)

	originX, originY, originZ := int32(10), int32(20), int32(30)
	c.Send(encodeAttackRequest(hostile.ObjectID(), originX, originY, originZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeValidateLocation, "select ValidateLocation")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMyTargetSelected, "MyTargetSelected")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeStatusUpdate, "selection StatusUpdate")
	c.Send(encodeAttackRequest(hostile.ObjectID(), originX, originY, originZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeAutoAttackStart, "AutoAttackStart")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeAttack, "Attack")

	c.Send(encodeUseItem(scroll, false))
	ids := collectMagicSkillUseIDs(t, c, 3*time.Second)
	if len(ids) != 1 || ids[0] != 2015 {
		t.Fatalf("MagicSkillUse skill ids = %v, want only 2015", ids)
	}

	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, scroll); inst.Count != 1 {
		t.Fatalf("persisted two-skill scroll count = %d, want 1", inst.Count)
	}
}

func disablePlayerSkill(t *testing.T, srv *gameservertest.Server, objID int32, def modelskill.Definition, delay time.Duration) {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	disabler, ok := obj.(interface {
		DisableSkill(int32, time.Duration)
	})
	if !ok {
		t.Fatalf("world.Player(%d) = %T does not expose DisableSkill", objID, obj)
	}
	disabler.DisableSkill(actorcast.ReuseKey(def), delay)
}

func playerCastingNow(t *testing.T, srv *gameservertest.Server, objID int32) bool {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	caster, ok := obj.(interface{ CastingNow() bool })
	if !ok {
		t.Fatalf("world.Player(%d) = %T does not expose CastingNow", objID, obj)
	}
	return caster.CastingNow()
}

func collectMagicSkillUseIDs(t *testing.T, c *testsupport.ScriptedClient, window time.Duration) []int32 {
	t.Helper()
	deadline := time.Now().Add(window)
	quiet := 500 * time.Millisecond
	var ids []int32
	for time.Now().Before(deadline) {
		timeout := 200 * time.Millisecond
		if len(ids) > 0 {
			timeout = quiet
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		if timeout > remaining {
			timeout = remaining
		}
		frame := c.ReadWithTimeout(timeout)
		if frame == nil {
			if len(ids) > 0 {
				return ids
			}
			continue
		}
		if frame[0] != serverpackets.OpcodeMagicSkillUse {
			continue
		}
		ids = append(ids, magicSkillUseSkillID(frame))
	}
	return ids
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

// TestUseFlyingConditionRejected pins the datapack flying gate: with the
// player in a flying transport mode, the potion's use condition answers
// S1_CANNOT_BE_USED naming the item, and the stack is untouched.
func TestUseFlyingConditionRejected(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(consumableSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	potion := srv.GiveItem(t, objID, 1060, 5)
	startInWorld(t, c)

	srv.SetPlayerFlying(t, objID, true)
	c.Send(encodeUseItem(potion, false))
	assertSystemMessageItem(t, c.Read(), serverpackets.SystemMessageS1CannotBeUsed, 1060)
	barrier(t, c)
	if inst := mustFindItem(t, srv, objID, potion); inst.Count != 5 {
		t.Fatalf("potion count after flying rejection = %d, want 5", inst.Count)
	}
}

// TestDisabledItemUseIsSilent pins the per-item disable gate: a use click
// for an item under an active disable produces no reply at all and consumes
// nothing.
func TestDisabledItemUseIsSilent(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(consumableSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	potion := srv.GiveItem(t, objID, 1060, 5)
	startInWorld(t, c)

	srv.DisablePlayerItem(t, objID, potion, time.Minute)
	c.Send(encodeUseItem(potion, false))
	if reply := c.ReadWithTimeout(300 * time.Millisecond); reply != nil {
		t.Fatalf("disabled item use replied %x, want no reply at all", reply)
	}
	if inst := mustFindItem(t, srv, objID, potion); inst.Count != 5 {
		t.Fatalf("potion count after disabled use = %d, want 5", inst.Count)
	}
}

// TestUseManaPotionRestoresMPAndConsumes drives the mana potion: the
// restore skill is announced, a StatusUpdate carries the refreshed MP once
// the batching task drains, and one unit leaves the stack and the row.
func TestUseManaPotionRestoresMPAndConsumes(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(consumableSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	potion := srv.GiveItem(t, objID, 728, 3)
	startInWorld(t, c)
	srv.DrainPlayerMP(t, objID, 20)
	before := srv.PlayerCurrentMP(t, objID)

	c.Send(encodeUseItem(potion, false))
	assertMagicSkillUseSelf(t, c.Read(), objID, 2279, 2, 0, 0)
	drainUntilQuiet(t, c)

	srv.InventoryUpdates.Tick()
	readInventoryUpdateFor(t, c, potion, 2)
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, potion); inst.Count != 2 {
		t.Fatalf("persisted mana potion count = %d, want 2", inst.Count)
	}
	if got := srv.PlayerCurrentMP(t, objID); got <= before {
		t.Fatalf("MP after mana potion = %d, want > %d", got, before)
	}
}

// TestUseEnergyStoneCapsForceCharges drives the force-charge item: each use
// announces ForceIncreasedToS1 with the charge HUD packet until the cap,
// where it reports ForceMaxLevelReached instead, consuming one stone per
// use either way.
func TestUseEnergyStoneCapsForceCharges(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(consumableSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	stone := srv.GiveItem(t, objID, 5589, 2)
	startInWorld(t, c)

	assertChargeFrame := func(frame []byte, wantID int, charges int32) {
		t.Helper()
		r := wire.NewReader(frame[1:])
		if id := r.ReadInt32(); id != int32(wantID) {
			t.Fatalf("charge message id = %d, want %d", id, wantID)
		}
		if wantID == serverpackets.SystemMessageForceIncreasedToS1 {
			if params := r.ReadInt32(); params != 1 || r.ReadInt32() != serverpackets.SystemMessageParamNumber || r.ReadInt32() != charges {
				t.Fatalf("force-increased params wrong, want one number %d", charges)
			}
		} else if params := r.ReadInt32(); params != 0 {
			t.Fatalf("force-max params = %d, want 0", params)
		}
	}

	c.Send(encodeUseItem(stone, false))
	first := collectUntilQuiet(t, c)
	var sawCast, sawCharge, sawHUD bool
	for _, f := range first {
		switch {
		case f[0] == serverpackets.OpcodeMagicSkillUse:
			sawCast = true
		case f[0] == serverpackets.OpcodeSystemMessage && systemMessageID(t, f) == serverpackets.SystemMessageForceIncreasedToS1:
			assertChargeFrame(f, serverpackets.SystemMessageForceIncreasedToS1, 1)
			sawCharge = true
		case f[0] == serverpackets.OpcodeEtcStatusUpdate:
			sawHUD = true
		}
	}
	if !sawCast || !sawCharge || !sawHUD {
		t.Fatalf("first stone use frames missing cast=%t charge=%t hud=%t across %d frames", sawCast, sawCharge, sawHUD, len(first))
	}

	c.Send(encodeUseItem(stone, false))
	second := collectUntilQuiet(t, c)
	sawMax := false
	for _, f := range second {
		if f[0] == serverpackets.OpcodeSystemMessage && systemMessageID(t, f) == serverpackets.SystemMessageForceMaxLevelReached {
			assertChargeFrame(f, serverpackets.SystemMessageForceMaxLevelReached, 0)
			sawMax = true
		}
	}
	if !sawMax {
		t.Fatalf("second stone use produced no ForceMaxLevelReached across %d frames", len(second))
	}

	waitFor(t, "both stones consumed", func() bool {
		srv.InventoryUpdates.Tick()
		srv.FlushItems(t)
		for _, inst := range persistedItems(t, srv, objID) {
			if inst.ObjectID == stone {
				return false
			}
		}
		return true
	})
}

// TestEscapeScrollReuseIsRejected pins the carried-skill reuse gate on the
// AI-cast path: a second scroll use inside the reuse window answers
// S1_PREPARED_FOR_REUSE only and consumes nothing further.
func TestEscapeScrollReuseIsRejected(t *testing.T) {
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
	drainUntilQuiet(t, c)

	c.Send(encodeUseItem(scroll, false))
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "reuse SystemMessage")
	if id := systemMessageID(t, frame); id != serverpackets.SystemMessageS1PreparedForReuse {
		t.Fatalf("reuse message id = %d, want S1PreparedForReuse (%d)", id, serverpackets.SystemMessageS1PreparedForReuse)
	}
	if reply := c.ReadWithTimeout(300 * time.Millisecond); reply != nil {
		t.Fatalf("rejected reuse follow-up frame %x, want none", reply[0])
	}
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, scroll); inst.Count != 2 {
		t.Fatalf("scroll count after rejected reuse = %d, want 2 (unchanged)", inst.Count)
	}
}
