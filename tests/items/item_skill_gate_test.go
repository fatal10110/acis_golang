package items

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// escapeScrollID carries skill 2013 (no hit time, 5s reuse) in consumableSkills.
const escapeScrollID int32 = 736

// sitStandDelay is how long Player.sitDown/standUp hold the character
// before the SAT_DOWN/STOOD_UP event (Player.java:1542-1590).
const sitStandDelay = 2500 * time.Millisecond

func encodeRequestChangeWaitType(stand bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestChangeWaitType)
	w.WriteInt32(wire.BoolInt32(stand))
	return w.Bytes()
}

// changePosture sends a sit or stand request and reads its ChangeWaitType.
func changePosture(t *testing.T, c *testsupport.ScriptedClient, stand bool) {
	t.Helper()
	c.Send(encodeRequestChangeWaitType(stand))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeChangeWaitType, "ChangeWaitType")
}

// sitAndSettle seats the player and lets the sit-down transition end.
func sitAndSettle(t *testing.T, srv *gameservertest.Server) {
	t.Helper()
	changePosture(t, srv.Client, false)
	srv.Advance(t, sitStandDelay)
	drainUntilQuiet(t, srv.Client)
}

// assertNoFrameFor fails on any frame the client receives within d.
func assertNoFrameFor(t *testing.T, c *testsupport.ScriptedClient, d time.Duration, what string) {
	t.Helper()
	if frame := c.ReadWithTimeout(d); frame != nil {
		t.Fatalf("%s: got opcode %#x, want none", what, frame[0])
	}
}

func assertItemCount(t *testing.T, srv *gameservertest.Server, ownerID, objectID int32, want int) {
	t.Helper()
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, ownerID, objectID); inst.Count != want {
		t.Fatalf("persisted item %d count = %d, want %d", objectID, inst.Count, want)
	}
}

// TestUseItemSkillWaitsOutStandUp pins PlayableAI.tryToCast's
// isStandingNow() term (PlayableAI.java:313-318) for an ItemSkills cast:
// used during the stand-up, the cast is answered with ActionFailed and
// stored as the next intention, and it runs when STOOD_UP fires
// (PlayerAI.onEvtStoodUp) — not before.
func TestUseItemSkillWaitsOutStandUp(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(consumableSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	scroll := srv.GiveItem(t, objID, escapeScrollID, 3)
	startInWorld(t, c)
	sitAndSettle(t, srv)

	changePosture(t, c, true)
	standAt := c.Now()
	c.Send(encodeUseItem(scroll, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "queued item cast")

	frame := c.Read()
	if elapsed := c.Now().Sub(standAt); elapsed < sitStandDelay {
		t.Fatalf("queued item cast ran %v after stand-up, want no earlier than %v", elapsed, sitStandDelay)
	}
	assertMagicSkillUseSelf(t, frame, objID, 2013, 1, 0, 5000)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillLaunched, "MagicSkillLaunched")
	srv.InventoryUpdates.Tick()
	readInventoryUpdateFor(t, c, scroll, 2)
	assertItemCount(t, srv, objID, scroll, 2)
}

// TestUseItemSkillStandUpNotEndedByStaleSitTimer pins that a stand-up
// started before the sit-down ended owns the transition: the sit-down's
// timer, still running when the stand-up replaces it, must not end the
// stand-up early. An item cast queued during that stand-up runs only once
// the stand-up's own 2.5s have passed, as the reference holds isStandingNow()
// until standUp's own task (Player.java:1566-1580).
func TestUseItemSkillStandUpNotEndedByStaleSitTimer(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(consumableSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	scroll := srv.GiveItem(t, objID, escapeScrollID, 3)
	startInWorld(t, c)

	changePosture(t, c, false)
	srv.Advance(t, time.Second)
	changePosture(t, c, true)
	standAt := c.Now()
	c.Send(encodeUseItem(scroll, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "queued item cast")

	frame := c.Read()
	if elapsed := c.Now().Sub(standAt); elapsed < sitStandDelay {
		t.Fatalf("queued item cast ran %v after stand-up, want no earlier than %v", elapsed, sitStandDelay)
	}
	assertMagicSkillUseSelf(t, frame, objID, 2013, 1, 0, 5000)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillLaunched, "MagicSkillLaunched")
	srv.InventoryUpdates.Tick()
	readInventoryUpdateFor(t, c, scroll, 2)
	assertItemCount(t, srv, objID, scroll, 2)
}

// longCastSkillID is a known self buff whose 4s hit time outlasts the
// stand-up.
const longCastSkillID = 1040

// shortCastSkillID is a known self buff with a 1s hit time, queued from the
// skill bar behind longCastSkillID.
const shortCastSkillID = 1086

// longCastSkills is the escape scroll's skill plus longCastSkillID and
// shortCastSkillID.
func longCastSkills(t *testing.T) *skillstate.Persistence {
	t.Helper()
	db := sqltest.SharedDB(t)
	return skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: 248, Level: 3},
		{ID: 294, Level: 1},
		{
			ID: 2013, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "TELEPORT", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 5000,
		},
		{
			ID: longCastSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", StaticHitTime: true, HitTime: 4000, StaticReuse: true,
		},
		{
			ID: shortCastSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", StaticHitTime: true, HitTime: 1000, StaticReuse: true,
		},
	}), gamesql.NewCharacterSkillStore(db))
}

// TestQueuedItemSkillOutlastsStandUpBehindCast pins that the end of a
// sit/stand transition leaves a queued item cast alone while a cast is still
// in flight: the item cast runs when that cast finishes, rather than being
// tried at the stand-up's end and refused as already casting. The cast in
// flight at the stand-up's end comes from a sit and stand made during that
// cast, which Go applies at once and the reference would queue behind the
// cast (#2674).
func TestQueuedItemSkillOutlastsStandUpBehindCast(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(longCastSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, longCastSkillID, 1); err != nil {
		t.Fatalf("seed known skill: %v", err)
	}
	scroll := srv.GiveItem(t, objID, escapeScrollID, 3)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(longCastSkillID))
	assertMagicSkillUseSelf(t, c.Read(), objID, longCastSkillID, 1, 4000, 0)
	castAt := c.Now()
	drainUntilQuiet(t, c)
	changePosture(t, c, false)
	changePosture(t, c, true)
	c.Send(encodeUseItem(scroll, false))
	for {
		frame := c.Read()
		if frame[0] == serverpackets.OpcodeActionFailed {
			break
		}
		if frame[0] == serverpackets.OpcodeMagicSkillUse {
			t.Fatal("item cast started while the long cast was in flight")
		}
	}

	launched := false
	for {
		frame := c.ReadWithTimeout(2 * sitStandDelay)
		if frame == nil {
			t.Fatal("queued item cast never ran after the long cast finished")
		}
		switch frame[0] {
		case serverpackets.OpcodeMagicSkillLaunched:
			launched = true
			continue
		case serverpackets.OpcodeMagicSkillUse:
		default:
			continue
		}
		if !launched {
			t.Fatal("queued item cast started before the long cast launched")
		}
		if elapsed := c.Now().Sub(castAt); elapsed < 4*time.Second {
			t.Fatalf("queued item cast ran %v into the 4s cast, want after it", elapsed)
		}
		assertMagicSkillUseSelf(t, frame, objID, 2013, 1, 0, 5000)
		break
	}
	drainUntilQuiet(t, c)
	assertItemCount(t, srv, objID, scroll, 2)
}

// TestQueuedItemSkillWaitsOutStandUpAfterCastEnds pins the other order: a
// queued item cast whose cast in flight ends inside the stand-up stays
// queued until the stand-up ends, as the reference only runs it from
// onEvtStoodUp (PlayerAI.java:109-123) while standing up.
func TestQueuedItemSkillWaitsOutStandUpAfterCastEnds(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(longCastSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, longCastSkillID, 1); err != nil {
		t.Fatalf("seed known skill: %v", err)
	}
	scroll := srv.GiveItem(t, objID, escapeScrollID, 3)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(longCastSkillID))
	assertMagicSkillUseSelf(t, c.Read(), objID, longCastSkillID, 1, 4000, 0)
	castAt := c.Now()
	srv.Advance(t, 2*time.Second)
	drainUntilQuiet(t, c)
	changePosture(t, c, false)
	changePosture(t, c, true)
	standAt := c.Now()
	if castEnd := castAt.Add(4 * time.Second); !castEnd.After(standAt) || castEnd.Sub(standAt) >= sitStandDelay {
		t.Fatalf("long cast ends %v after the stand-up began, want inside the stand-up", castEnd.Sub(standAt))
	}
	c.Send(encodeUseItem(scroll, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "queued item cast")

	for {
		frame := c.ReadWithTimeout(2 * sitStandDelay)
		if frame == nil {
			t.Fatal("queued item cast never ran after the stand-up")
		}
		if frame[0] != serverpackets.OpcodeMagicSkillUse {
			continue
		}
		if elapsed := c.Now().Sub(standAt); elapsed < sitStandDelay {
			t.Fatalf("queued item cast ran %v after stand-up, want no earlier than %v", elapsed, sitStandDelay)
		}
		assertMagicSkillUseSelf(t, frame, objID, 2013, 1, 0, 5000)
		break
	}
	drainUntilQuiet(t, c)
	assertItemCount(t, srv, objID, scroll, 2)
}

// TestQueuedSkillBarCastWaitsOutStandUpAfterCastEnds is the skill-bar
// counterpart: a skill request queued behind a cast that ends inside the
// stand-up runs when the stand-up ends, not when the cast does.
func TestQueuedSkillBarCastWaitsOutStandUpAfterCastEnds(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(longCastSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	for _, id := range []int{longCastSkillID, shortCastSkillID} {
		if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, id, 1); err != nil {
			t.Fatalf("seed known skill %d: %v", id, err)
		}
	}
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(longCastSkillID))
	assertMagicSkillUseSelf(t, c.Read(), objID, longCastSkillID, 1, 4000, 0)
	castAt := c.Now()
	srv.Advance(t, 2*time.Second)
	drainUntilQuiet(t, c)
	changePosture(t, c, false)
	changePosture(t, c, true)
	standAt := c.Now()
	if castEnd := castAt.Add(4 * time.Second); !castEnd.After(standAt) || castEnd.Sub(standAt) >= sitStandDelay {
		t.Fatalf("long cast ends %v after the stand-up began, want inside the stand-up", castEnd.Sub(standAt))
	}
	c.Send(encodeRequestMagicSkillUse(shortCastSkillID))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "queued skill-bar cast")

	for {
		frame := c.ReadWithTimeout(2 * sitStandDelay)
		if frame == nil {
			t.Fatal("queued skill-bar cast never ran after the stand-up")
		}
		if frame[0] != serverpackets.OpcodeMagicSkillUse {
			continue
		}
		if elapsed := c.Now().Sub(standAt); elapsed < sitStandDelay {
			t.Fatalf("queued skill-bar cast ran %v after stand-up, want no earlier than %v", elapsed, sitStandDelay)
		}
		assertMagicSkillUseSelf(t, frame, objID, shortCastSkillID, 1, 1000, 0)
		break
	}
	drainUntilQuiet(t, c)
}

// TestQueuedItemSkillHeldBehindSwingDuringStandUp pins the swing-end side:
// an item cast queued behind a swing that ends inside the stand-up stays the
// next intention until the stand-up ends. The attack it replaced does not
// swing again meanwhile, and the item runs at the stand-up's end. The swing
// in flight comes from a sit and stand made during it, which Go applies at
// once (#2674).
func TestQueuedItemSkillHeldBehindSwingDuringStandUp(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(consumableSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	scroll := srv.GiveItem(t, objID, escapeScrollID, 3)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: 40, Y: 20, Z: 30})
	drainUntilQuiet(t, c)

	c.Send(encodeAttackRequest(hostile.ObjectID(), 10, 20, 30, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeValidateLocation, "select ValidateLocation")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMyTargetSelected, "MyTargetSelected")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeStatusUpdate, "selection StatusUpdate")
	c.Send(encodeAttackRequest(hostile.ObjectID(), 10, 20, 30, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeAutoAttackStart, "AutoAttackStart")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeAttack, "Attack")
	changePosture(t, c, false)
	changePosture(t, c, true)
	standAt := c.Now()
	c.Send(encodeUseItem(scroll, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "queued item cast")

	for {
		frame := c.ReadWithTimeout(2 * sitStandDelay)
		if frame == nil {
			t.Fatal("queued item cast never ran after the stand-up")
		}
		if frame[0] == serverpackets.OpcodeAttack {
			t.Fatalf("attack swung again %v into the stand-up, want the queued item cast to replace it", c.Now().Sub(standAt))
		}
		if frame[0] != serverpackets.OpcodeMagicSkillUse {
			continue
		}
		if elapsed := c.Now().Sub(standAt); elapsed < sitStandDelay {
			t.Fatalf("queued item cast ran %v after stand-up, want no earlier than %v", elapsed, sitStandDelay)
		}
		assertMagicSkillUseSelf(t, frame, objID, 2013, 1, 0, 5000)
		break
	}
	drainUntilQuiet(t, c)
	assertItemCount(t, srv, objID, scroll, 2)
}

// TestUseItemSkillWhileSeatedIsRefused pins PlayerCast.canAttemptCast's
// sitting rule (PlayerCast.java:212-216) on the immediate ItemSkills path:
// CANT_MOVE_SITTING then ActionFailed, no cast and nothing consumed.
func TestUseItemSkillWhileSeatedIsRefused(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(consumableSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	scroll := srv.GiveItem(t, objID, escapeScrollID, 3)
	startInWorld(t, c)
	sitAndSettle(t, srv)

	c.Send(encodeUseItem(scroll, false))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCannotMoveWhileSitting)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "seated ActionFailed")
	assertNoFrameFor(t, c, 3*time.Second, "seated item use")
	assertItemCount(t, srv, objID, scroll, 3)
}

// TestUseItemSkillDuringSitDownIsRefusedOnceSeated pins the sit-down half:
// the caster is not sitting yet, so the pre-attempt gate lets the cast
// through and isSittingNow() queues it with ActionFailed; when SAT_DOWN runs
// it (PlayerAI.onEvtSatDown), thinkCast's canAttemptCast refuses it with
// CANT_MOVE_SITTING alone (PlayerAI.java:241-242). Nothing is consumed.
func TestUseItemSkillDuringSitDownIsRefusedOnceSeated(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(consumableSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	scroll := srv.GiveItem(t, objID, escapeScrollID, 3)
	startInWorld(t, c)

	changePosture(t, c, false)
	sitAt := c.Now()
	c.Send(encodeUseItem(scroll, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "queued item cast")

	frame := c.Read()
	if elapsed := c.Now().Sub(sitAt); elapsed < sitStandDelay {
		t.Fatalf("queued item cast answered %v after sit-down, want no earlier than %v", elapsed, sitStandDelay)
	}
	assertStaticSystemMessage(t, frame, serverpackets.SystemMessageCannotMoveWhileSitting)
	assertNoFrameFor(t, c, time.Second, "refused queued item cast")
	assertItemCount(t, srv, objID, scroll, 3)
}

// TestUseItemSkillGateRefusesBeforeQueueing pins that the pre-attempt gate
// runs before the busy branch (PlayableAI.java:305-318): a formal-wear
// caster's item used during the stand-up is refused with
// CANNOT_USE_ITEMS_SKILLS_WITH_FORMALWEAR and ActionFailed, and nothing is
// stored to run when the stand-up ends.
func TestUseItemSkillGateRefusesBeforeQueueing(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(consumableSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	scroll := srv.GiveItem(t, objID, escapeScrollID, 3)
	srv.GiveItem(t, objID, gameservertest.FormalWearID, 1)
	startInWorld(t, c)
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		inv := pc.Inventory()
		inst := inv.ItemByTemplateID(gameservertest.FormalWearID)
		tmpl, ok := inv.Templates().Get(gameservertest.FormalWearID)
		if inst == nil || !ok {
			t.Errorf("formal wear item %v / template %v missing", inst, ok)
			return
		}
		inv.EquipItem(inst, tmpl)
	})
	sitAndSettle(t, srv)

	changePosture(t, c, true)
	c.Send(encodeUseItem(scroll, false))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCannotUseSkillsWithFormalWear)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "gate ActionFailed")
	assertNoFrameFor(t, c, 2*sitStandDelay, "refused item cast after stand-up")
	assertItemCount(t, srv, objID, scroll, 3)
}

// TestSitRequestDropsQueuedItemSkill pins tryToSit taking the next-intention
// slot (PlayableAI.java:430-446): an item cast queued during the stand-up is
// replaced by a sit request, so it neither runs nor is answered later, and
// nothing is consumed.
func TestSitRequestDropsQueuedItemSkill(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(consumableSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	scroll := srv.GiveItem(t, objID, escapeScrollID, 3)
	startInWorld(t, c)
	sitAndSettle(t, srv)

	changePosture(t, c, true)
	c.Send(encodeUseItem(scroll, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "queued item cast")
	changePosture(t, c, false)
	assertNoFrameFor(t, c, 2*sitStandDelay, "replaced item cast")
	assertItemCount(t, srv, objID, scroll, 3)
}

// ctrlKeySkills makes the unlockable key's skill a non-offensive single-
// target skill, which a monster target refuses unless Ctrl forces it
// (TargetOne's cast check, the skill's isCtrlPressed).
func ctrlKeySkills(t *testing.T) *skillstate.Persistence {
	t.Helper()
	db := sqltest.SharedDB(t)
	return skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: 248, Level: 3},
		{ID: 294, Level: 1},
		{
			ID: 2236, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "BUFF", CastRange: 400, StaticHitTime: true, HitTime: 0, StaticReuse: true,
		},
	}), gamesql.NewCharacterSkillStore(db))
}

// TestUseItemCtrlForcesItemSkillTarget pins ItemSkills.java:80 passing
// UseItem's Ctrl modifier to tryToCast as isCtrlPressed: the same key on the
// same monster is refused without Ctrl and cast with it, both when it
// starts at once and when it was queued behind the stand-up.
func TestUseItemCtrlForcesItemSkillTarget(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(ctrlKeySkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	key := srv.GiveItem(t, objID, gameservertest.UnlockableKeyID, 3)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: 40, Y: 20, Z: 30})
	drainUntilQuiet(t, c)
	c.Send(encodeAction(hostile.ObjectID(), 40, 20, 30, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeValidateLocation, "select ValidateLocation")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMyTargetSelected, "select MyTargetSelected")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeStatusUpdate, "select StatusUpdate")

	c.Send(encodeUseItem(key, false))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageInvalidTarget)
	assertNoFrameFor(t, c, 300*time.Millisecond, "unforced key")

	assertKeyCastOnHostile := func(what string) {
		t.Helper()
		frame := c.Read()
		assertFrameOpcode(t, frame, serverpackets.OpcodeMagicSkillUse, what)
		caster, target, skillID, _, _, _ := decodeMagicSkillUse(frame)
		if caster != objID || target != hostile.ObjectID() || skillID != 2236 {
			t.Fatalf("%s MagicSkillUse = caster %d target %d skill %d, want %d/%d/2236", what, caster, target, skillID, objID, hostile.ObjectID())
		}
		drainUntilQuiet(t, c)
	}
	c.Send(encodeUseItem(key, true))
	assertKeyCastOnHostile("forced key")

	sitAndSettle(t, srv)
	changePosture(t, c, true)
	c.Send(encodeUseItem(key, true))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "queued forced key")
	assertKeyCastOnHostile("queued forced key")
	assertItemCount(t, srv, objID, key, 1)
}

// onPlayerQueue runs fn on the online player's actor queue and waits for it.
func onPlayerQueue(t *testing.T, srv *gameservertest.Server, objID int32, fn func(*player.Character)) {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	pc, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	done := make(chan struct{})
	if !pc.Queue().Post(func() { fn(pc); close(done) }) {
		t.Fatal("post to player queue: queue closed")
	}
	<-done
}

// TestUseItemSkillDuringFakeDeathGetUpIsRefused pins that a player getting
// up out of fake death still plays dead until the get-up ends
// (Player.stopFakeDeath keeps _isFakeDeath until its task,
// Player.java:7035-7056), so an item used in it is refused by the use
// gate's isAlikeDead() (UseItem.java:66): no cast and nothing consumed.
func TestUseItemSkillDuringFakeDeathGetUpIsRefused(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(consumableSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	scroll := srv.GiveItem(t, objID, escapeScrollID, 3)
	startInWorld(t, c)
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		fake, err := effect.New(effect.Skill{ID: 60}, modelskill.EffectTemplate{Name: "FakeDeath"})
		if err != nil {
			t.Errorf("new fake-death effect: %v", err)
			return
		}
		fake.Effector, fake.Effected = pc, pc
		pc.EffectList().Add(fake)
	})
	srv.Advance(t, 3*time.Second)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestChangeWaitType(true))
	if seen := opcodesUntilQuiet(t, c); seen[serverpackets.OpcodeChangeWaitType] != 1 {
		t.Fatalf("fake-death stop ChangeWaitType count = %d, want 1", seen[serverpackets.OpcodeChangeWaitType])
	}

	c.Send(encodeUseItem(scroll, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "item use while getting up")
	srv.Advance(t, sitStandDelay)
	if seen := opcodesUntilQuiet(t, c); seen[serverpackets.OpcodeMagicSkillUse] != 0 {
		t.Fatal("refused item cast started after the fake-death get-up")
	}
	assertItemCount(t, srv, objID, scroll, 3)
}
