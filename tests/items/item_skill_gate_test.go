package items

import (
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
