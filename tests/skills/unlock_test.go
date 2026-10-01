package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Unlock behavior follows the reference unlock skill handler
// (handler/skillhandlers/Unlock.java) and its formulas (Formulas.doorUnlock,
// Formulas.chestUnlock). Every case below picks inputs whose outcome needs
// no random roll: UNLOCK_SPECIAL opens when Rnd.get(100) < power, so power
// 100 always opens and power 0 never does; a regular UNLOCK above level 10
// always opens a chest of level 30 or less, and one below level 10 never
// opens a chest above level 60.
const (
	unlockDoorID     = 19210001
	unlockDoorX      = 40
	unlockSkillLevel = 1
	unlockHitTime    = 500
	unlockReuse      = 1000
)

func unlockDoorTemplate(opened bool) *door.Template {
	return &door.Template{
		ID:       unlockDoorID,
		Name:     "unlock_gate",
		Kind:     door.KindDoor,
		Level:    1,
		Position: location.Location{X: unlockDoorX, Y: 0, Z: 0},
		Coordinates: []location.Point{
			{X: unlockDoorX - 8, Y: -8},
			{X: unlockDoorX + 8, Y: -8},
			{X: unlockDoorX + 8, Y: 8},
			{X: unlockDoorX - 8, Y: 8},
		},
		HP: 100, PDef: 10, MDef: 10, Height: 32,
		Opened:   opened,
		OpenKind: door.OpenSkill,
	}
}

func unlockSkill(id int32, skillType string, level, power int) modelskill.Definition {
	return modelskill.Definition{
		ID: modelskill.ID(id), Level: level, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetUnlockable, SkillType: skillType, Power: float32(power),
		CastRange: 900, HitTime: unlockHitTime, ReuseDelay: unlockReuse, StaticHitTime: true, StaticReuse: true,
	}
}

// bootUnlock boots one player who knows def at def.Level, with doors spawned
// through the production world-object owner.
func bootUnlock(t *testing.T, def modelskill.Definition, doors ...*door.Template) (*gameservertest.Server, int32) {
	t.Helper()
	return bootUnlockOpts(t, nil, def, doors...)
}

// bootUnlockOpts is bootUnlock with extra boot options.
func bootUnlockOpts(t *testing.T, extra []gameservertest.Option, def modelskill.Definition, doors ...*door.Template) (*gameservertest.Server, int32) {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
		gameservertest.WithDoors(doors...),
	}, extra...)...)
	objID := srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, int(def.ID), def.Level)
	if len(doors) == 0 {
		startInWorld(t, srv.Client)
		return srv, objID
	}
	// A door known at spawn adds its own info frames to the EnterWorld
	// reply, so this entry drains the burst rather than matching it.
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "SSQInfo")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "CharSelected")
	c.Send(encodeEnterWorld())
	drainUntilQuiet(t, c)
	return srv, objID
}

// selectTarget clicks objectID and drains the selection answer.
func selectTarget(t *testing.T, c *testsupport.ScriptedClient, objectID int32) {
	t.Helper()
	c.Send(encodeAction(objectID, 0, 0, 0, false))
	for range 10 {
		frame := c.Read()
		if frame[0] == serverpackets.OpcodeMyTargetSelected {
			if got := wireReader(frame[1:]).ReadInt32(); got != objectID {
				t.Fatalf("MyTargetSelected object id = %d, want %d", got, objectID)
			}
			drainUntilQuiet(t, c)
			return
		}
	}
	t.Fatalf("object %d was never selected", objectID)
}

// castUnlock casts def on the selected target and returns every frame up to
// the client going quiet after the hit.
func castUnlock(t *testing.T, c *testsupport.ScriptedClient, objID int32, def modelskill.Definition, targetID int32) [][]byte {
	t.Helper()
	c.Send(encodeRequestMagicSkillUse(int32(def.ID), false, false))
	readCastStartFrames(t, c, objID, int32(def.ID), int32(def.Level), unlockHitTime, unlockReuse, targetID)
	var frames [][]byte
	for {
		frame := c.ReadWithTimeout(time.Second)
		if frame == nil {
			return frames
		}
		frames = append(frames, frame)
	}
}

func framesWith(frames [][]byte, opcode byte, objectID int32) [][]byte {
	var out [][]byte
	for _, frame := range frames {
		if frame[0] == opcode && wireReader(frame[1:]).ReadInt32() == objectID {
			out = append(out, frame)
		}
	}
	return out
}

func systemMessageIDs(frames [][]byte) []int32 {
	var ids []int32
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeSystemMessage {
			ids = append(ids, wireReader(frame[1:]).ReadInt32())
		}
	}
	return ids
}

func assertNoSystemMessage(t *testing.T, frames [][]byte, id int32) {
	t.Helper()
	for _, got := range systemMessageIDs(frames) {
		if got == id {
			t.Fatalf("system message %d sent, want none", id)
		}
	}
}

func assertOneSystemMessage(t *testing.T, frames [][]byte, id int32) {
	t.Helper()
	count := 0
	for _, got := range systemMessageIDs(frames) {
		if got == id {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("system message %d sent %d times, want once (all: %v)", id, count, systemMessageIDs(frames))
	}
}

func TestUnlockSkillOpensDoorThroughWorldOwner(t *testing.T) {
	t.Parallel()
	def := unlockSkill(2235, "UNLOCK_SPECIAL", unlockSkillLevel, 100)
	srv, objID := bootUnlock(t, def, unlockDoorTemplate(false))
	gate, ok := srv.WorldObjects.Door(unlockDoorID)
	if !ok {
		t.Fatal("door not spawned")
	}
	selectTarget(t, srv.Client, gate.ObjectID())

	frames := castUnlock(t, srv.Client, objID, def, gate.ObjectID())

	if !gate.Opened() {
		t.Fatal("door still closed after a guaranteed unlock")
	}
	updates := framesWith(frames, serverpackets.OpcodeDoorStatusUpdate, gate.ObjectID())
	if len(updates) != 1 {
		t.Fatalf("DoorStatusUpdate frames = %d, want 1", len(updates))
	}
	r := wireReader(updates[0][1:])
	_ = r.ReadInt32()
	if closed := r.ReadInt32(); closed != 0 {
		t.Fatalf("DoorStatusUpdate closed flag = %d, want 0 (open)", closed)
	}
	assertNoSystemMessage(t, frames, serverpackets.SystemMessageFailedToUnlockDoor)
}

func TestUnlockSkillFailedRollKeepsDoorClosed(t *testing.T) {
	t.Parallel()
	def := unlockSkill(2235, "UNLOCK_SPECIAL", unlockSkillLevel, 0)
	srv, objID := bootUnlock(t, def, unlockDoorTemplate(false))
	gate, _ := srv.WorldObjects.Door(unlockDoorID)
	selectTarget(t, srv.Client, gate.ObjectID())

	frames := castUnlock(t, srv.Client, objID, def, gate.ObjectID())

	if gate.Opened() {
		t.Fatal("door opened by a never-succeeding unlock")
	}
	if got := framesWith(frames, serverpackets.OpcodeDoorStatusUpdate, gate.ObjectID()); len(got) != 0 {
		t.Fatalf("DoorStatusUpdate frames = %d, want none", len(got))
	}
	assertOneSystemMessage(t, frames, serverpackets.SystemMessageFailedToUnlockDoor)
}

func TestUnlockSkillOnOpenDoorReportsFailure(t *testing.T) {
	t.Parallel()
	def := unlockSkill(2235, "UNLOCK_SPECIAL", unlockSkillLevel, 100)
	srv, objID := bootUnlock(t, def, unlockDoorTemplate(true))
	gate, _ := srv.WorldObjects.Door(unlockDoorID)
	selectTarget(t, srv.Client, gate.ObjectID())

	frames := castUnlock(t, srv.Client, objID, def, gate.ObjectID())

	if !gate.Opened() {
		t.Fatal("open door closed by an unlock")
	}
	if got := framesWith(frames, serverpackets.OpcodeDoorStatusUpdate, gate.ObjectID()); len(got) != 0 {
		t.Fatalf("DoorStatusUpdate frames = %d, want none", len(got))
	}
	assertOneSystemMessage(t, frames, serverpackets.SystemMessageFailedToUnlockDoor)
}

func chestTemplate(id, level int) *npc.Template {
	return &npc.Template{
		ID: id, TemplateID: id, Type: "Chest", Level: level,
		HPMax: 1000, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
		CollisionRadius: 8, CollisionHeight: 20,
	}
}

func TestUnlockSkillOpensBoxChest(t *testing.T) {
	t.Parallel()
	def := unlockSkill(27, "UNLOCK", 11, 0)
	srv, objID := bootUnlock(t, def)
	chest := srv.SpawnHostileNPCTemplateAt(t, chestTemplate(18265, 1), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, srv.Client)
	selectTarget(t, srv.Client, chest.ObjectID())

	frames := castUnlock(t, srv.Client, objID, def, chest.ObjectID())

	if !chest.Dead() {
		t.Fatal("box chest survived a guaranteed unlock")
	}
	if !chest.Interacted() {
		t.Fatal("opened chest not marked interacted")
	}
	if got := framesWith(frames, serverpackets.OpcodeDie, chest.ObjectID()); len(got) != 1 {
		t.Fatalf("chest Die frames = %d, want 1", len(got))
	}
	if _, ok := srv.State.Object(chest.ObjectID()); !ok {
		t.Fatal("opened chest left the world before its corpse decayed")
	}
}

func TestUnlockSkillFailureDeletesBoxChest(t *testing.T) {
	t.Parallel()
	def := unlockSkill(27, "UNLOCK", 5, 0)
	srv, objID := bootUnlock(t, def)
	chest := srv.SpawnHostileNPCTemplateAt(t, chestTemplate(18298, 70), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, srv.Client)
	selectTarget(t, srv.Client, chest.ObjectID())

	frames := castUnlock(t, srv.Client, objID, def, chest.ObjectID())

	if got := framesWith(frames, serverpackets.OpcodeDie, chest.ObjectID()); len(got) != 0 {
		t.Fatalf("chest Die frames = %d, want none", len(got))
	}
	if got := framesWith(frames, serverpackets.OpcodeDeleteObject, chest.ObjectID()); len(got) != 1 {
		t.Fatalf("chest DeleteObject frames = %d, want 1", len(got))
	}
	if _, ok := srv.State.Object(chest.ObjectID()); ok {
		t.Fatal("failed chest still in the world")
	}
}

func TestUnlockSkillOnMimicChestLeavesItStanding(t *testing.T) {
	t.Parallel()
	def := unlockSkill(27, "UNLOCK", 11, 0)
	srv, objID := bootUnlock(t, def)
	chest := srv.SpawnHostileNPCTemplateAt(t, chestTemplate(18264, 1), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, srv.Client)
	selectTarget(t, srv.Client, chest.ObjectID())

	frames := castUnlock(t, srv.Client, objID, def, chest.ObjectID())

	if chest.Dead() || chest.Interacted() {
		t.Fatalf("mimic chest dead=%v interacted=%v, want neither", chest.Dead(), chest.Interacted())
	}
	if got := framesWith(frames, serverpackets.OpcodeDeleteObject, chest.ObjectID()); len(got) != 0 {
		t.Fatalf("mimic chest DeleteObject frames = %d, want none", len(got))
	}
	if _, ok := srv.State.Object(chest.ObjectID()); !ok {
		t.Fatal("mimic chest left the world")
	}
}

// An unlock skill with a ONE target (the event chest key, skill 2322) is
// not held to the UNLOCKABLE target check, so it can land on a player,
// including the caster; the reference handler answers a target that is
// neither a door nor a chest with INVALID_TARGET.
func TestUnlockSkillOnPlayerReportsInvalidTarget(t *testing.T) {
	t.Parallel()
	def := unlockSkill(2322, "UNLOCK_SPECIAL", unlockSkillLevel, 100)
	def.Target = modelskill.TargetOne
	srv, objID := bootUnlock(t, def)
	selectTarget(t, srv.Client, objID)

	frames := castUnlock(t, srv.Client, objID, def, objID)

	assertOneSystemMessage(t, frames, serverpackets.SystemMessageInvalidTarget)
}

// A chest an earlier attempt already claimed answers a later unlock with
// nothing: no roll, no death, no removal, no hate.
func TestUnlockSkillOnClaimedChestDoesNothing(t *testing.T) {
	t.Parallel()
	def := unlockSkill(27, "UNLOCK", 11, 0)
	srv, objID := bootUnlock(t, def)
	chest := srv.SpawnHostileNPCTemplateAt(t, chestTemplate(18265, 1), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, srv.Client)
	if !chest.ClaimInteraction() {
		t.Fatal("first ClaimInteraction() = false")
	}
	selectTarget(t, srv.Client, chest.ObjectID())

	frames := castUnlock(t, srv.Client, objID, def, chest.ObjectID())

	if chest.Dead() {
		t.Fatal("claimed chest opened by a second unlock")
	}
	if got := framesWith(frames, serverpackets.OpcodeDie, chest.ObjectID()); len(got) != 0 {
		t.Fatalf("chest Die frames = %d, want none", len(got))
	}
	if got := framesWith(frames, serverpackets.OpcodeDeleteObject, chest.ObjectID()); len(got) != 0 {
		t.Fatalf("chest DeleteObject frames = %d, want none", len(got))
	}
	if _, ok := srv.State.Object(chest.ObjectID()); !ok {
		t.Fatal("claimed chest left the world")
	}
	if threats := chest.AI().Threats().Snapshot(); len(threats) != 0 {
		t.Fatalf("chest threats = %v, want none", threats)
	}
	if ids := systemMessageIDs(frames); len(ids) != 0 {
		t.Fatalf("system messages = %v, want none", ids)
	}
}
