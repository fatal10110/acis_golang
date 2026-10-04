package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Door HP follows the reference door and creature status
// (model/actor/instance/Door.java, model/actor/status/CreatureStatus.java,
// model/actor/Creature.java):
//   - Door.reduceCurrentHp lowers HP only while the door's castle siege is in
//     progress (any attacker but a Swoop Cannon) or its siegable hall's siege
//     zone is active; a door with neither keeps its HP.
//   - CreatureStatus.setHp caps HP at the maximum, starts the regeneration
//     task below it and stops it there, and DoorStatus.broadcastStatusUpdate
//     sends DoorStatusUpdate (0x4d: object id, closed, damage stage, show HP,
//     door id, max HP, (int) HP) to every known player.
//   - Door.getDamage is max(0, min(6, 6 - ceil(hpRatio * 6))).
//   - Formulas.getRegeneratePeriod gives a door 100 x 3s = 5 minutes, at a
//     fixed rate from when HP first drops; each tick adds max(1, 1.5) (the
//     creature base hpRegen, which doors.xml never sets) and broadcasts.
//   - Death (CreatureStatus.reduceHp < 0.5 -> Creature.doDie -> Door.doDie):
//     the hit's setHp, doDie's setHp(0) and its broadcastStatusUpdate each
//     send DoorStatusUpdate; a closed door then leaves geodata but stays
//     closed, and changeState ignores a dead door.
//   - Door.doRevive takes the template's open state back (re-adding a closed
//     door's geodata), then Creature.doRevive sets HP to RespawnRestoreHP
//     (0.7) of the maximum, broadcasting it, and broadcasts Revive.
// Expected values below are worked from those formulas by hand.

const (
	hpDoorID    = unlockDoorID
	hpDoorMaxHP = 600
)

func hpDoorTemplate() *door.Template {
	tmpl := unlockDoorTemplate(false)
	tmpl.HP = hpDoorMaxHP
	tmpl.OpenKind = door.OpenNPC
	return tmpl
}

type doorStatusUpdate struct {
	objectID, closed, damage, showHP, doorID, maxHP, hp int32
}

// doorStatusUpdates returns every frame up to the client going quiet,
// decoding the DoorStatusUpdate ones and failing on anything else.
func doorStatusUpdates(t *testing.T, c *testsupport.ScriptedClient, objectID int32) []doorStatusUpdate {
	t.Helper()
	var out []doorStatusUpdate
	for {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return out
		}
		if frame[0] != serverpackets.OpcodeDoorStatusUpdate {
			t.Fatalf("frame opcode %#x, want only DoorStatusUpdate", frame[0])
		}
		r := wireReader(frame[1:])
		u := doorStatusUpdate{
			objectID: r.ReadInt32(), closed: r.ReadInt32(), damage: r.ReadInt32(), showHP: r.ReadInt32(),
			doorID: r.ReadInt32(), maxHP: r.ReadInt32(), hp: r.ReadInt32(),
		}
		if u.objectID != objectID {
			t.Fatalf("DoorStatusUpdate object id = %d, want %d", u.objectID, objectID)
		}
		out = append(out, u)
	}
}

func wantDoorStatus(objectID int32, damage, hp int32) doorStatusUpdate {
	return doorStatusUpdate{objectID: objectID, closed: 1, damage: damage, doorID: hpDoorID, maxHP: hpDoorMaxHP, hp: hp}
}

func assertDoorStatusUpdates(t *testing.T, got []doorStatusUpdate, want ...doorStatusUpdate) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("DoorStatusUpdates = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("DoorStatusUpdate[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// bootDoorHP boots one player who knows the closed door, and returns the
// door's live object.
func bootDoorHP(t *testing.T) (*gameservertest.Server, int32, *door.Object) {
	t.Helper()
	srv, objID := bootUnlock(t, unlockSkill(9000, "UNLOCK", unlockSkillLevel, 0), hpDoorTemplate())
	gate, ok := srv.WorldObjects.Door(hpDoorID)
	if !ok {
		t.Fatal("door not spawned")
	}
	return srv, objID, gate
}

func TestDoorHPDropsOnlyUnderSiege(t *testing.T) {
	t.Parallel()
	srv, objID, gate := bootDoorHP(t)
	c := srv.Client

	srv.HitDoor(t, hpDoorID, objID, 250)
	if got := doorStatusUpdates(t, c, gate.ObjectID()); len(got) != 0 {
		t.Fatalf("hit outside a siege sent %+v, want nothing", got)
	}
	if gate.CurrentHP() != hpDoorMaxHP || srv.DoorRegenerating(t, hpDoorID) {
		t.Fatalf("hit outside a siege left HP %v, regenerating %v; want full HP, no regeneration",
			gate.CurrentHP(), srv.DoorRegenerating(t, hpDoorID))
	}

	srv.BeginDoorSiege(t, hpDoorID)
	srv.HitDoor(t, hpDoorID, objID, 250)
	// 350/600 left: 6 - ceil(3.5) = stage 2.
	assertDoorStatusUpdates(t, doorStatusUpdates(t, c, gate.ObjectID()), wantDoorStatus(gate.ObjectID(), 2, 350))
	srv.HitDoor(t, hpDoorID, objID, 50)
	// 300/600 left: 6 - ceil(3) = stage 3.
	assertDoorStatusUpdates(t, doorStatusUpdates(t, c, gate.ObjectID()), wantDoorStatus(gate.ObjectID(), 3, 300))
	if gate.Dead() || !srv.DoorRegenerating(t, hpDoorID) {
		t.Fatalf("damaged door dead %v, regenerating %v; want alive and regenerating", gate.Dead(), srv.DoorRegenerating(t, hpDoorID))
	}
}

func TestDamagedDoorRegeneratesEveryFiveMinutes(t *testing.T) {
	t.Parallel()
	if task.DoorRegenPeriod != 5*time.Minute {
		t.Fatalf("DoorRegenPeriod = %v, want 5m", task.DoorRegenPeriod)
	}
	srv, objID, gate := bootDoorHP(t)
	c := srv.Client
	srv.BeginDoorSiege(t, hpDoorID)

	srv.HitDoor(t, hpDoorID, objID, 3)
	// 597/600: 6 - ceil(5.97) = stage 0.
	assertDoorStatusUpdates(t, doorStatusUpdates(t, c, gate.ObjectID()), wantDoorStatus(gate.ObjectID(), 0, 597))

	srv.AdvanceDoorRegen(t, 5*time.Minute-time.Second)
	if got := doorStatusUpdates(t, c, gate.ObjectID()); len(got) != 0 || gate.CurrentHP() != 597 {
		t.Fatalf("before the first period: sent %+v, HP %v; want nothing, 597", got, gate.CurrentHP())
	}
	// A later hit does not move the fixed-rate schedule.
	srv.HitDoor(t, hpDoorID, objID, 2)
	assertDoorStatusUpdates(t, doorStatusUpdates(t, c, gate.ObjectID()), wantDoorStatus(gate.ObjectID(), 0, 595))

	srv.AdvanceDoorRegen(t, time.Second)
	assertDoorStatusUpdates(t, doorStatusUpdates(t, c, gate.ObjectID()), wantDoorStatus(gate.ObjectID(), 0, 596))
	if gate.CurrentHP() != 596.5 {
		t.Fatalf("HP after one tick = %v, want 596.5", gate.CurrentHP())
	}

	srv.AdvanceDoorRegen(t, 5*time.Minute)
	assertDoorStatusUpdates(t, doorStatusUpdates(t, c, gate.ObjectID()), wantDoorStatus(gate.ObjectID(), 0, 598))
	srv.AdvanceDoorRegen(t, 5*time.Minute)
	assertDoorStatusUpdates(t, doorStatusUpdates(t, c, gate.ObjectID()), wantDoorStatus(gate.ObjectID(), 0, 599))
	// 599.5 + 1.5 caps at the maximum and stops the task.
	srv.AdvanceDoorRegen(t, 5*time.Minute)
	assertDoorStatusUpdates(t, doorStatusUpdates(t, c, gate.ObjectID()), wantDoorStatus(gate.ObjectID(), 0, hpDoorMaxHP))
	if gate.CurrentHP() != hpDoorMaxHP || srv.DoorRegenerating(t, hpDoorID) {
		t.Fatalf("full door HP %v, regenerating %v; want %d, stopped", gate.CurrentHP(), srv.DoorRegenerating(t, hpDoorID), hpDoorMaxHP)
	}
	srv.AdvanceDoorRegen(t, 5*time.Minute)
	if got := doorStatusUpdates(t, c, gate.ObjectID()); len(got) != 0 {
		t.Fatalf("full door kept regenerating: sent %+v", got)
	}
}

func TestBrokenDoorLeavesGeodataAndRevives(t *testing.T) {
	t.Parallel()
	srv, objID, gate := bootDoorHP(t)
	c := srv.Client
	geo := srv.DoorGeo(t)
	x, y, z := gate.Position()
	blocks := func() bool { return !geo.CanMove(x-48, y, z, x+48, y, z) }
	if !blocks() {
		t.Fatal("fixture: the closed door does not block geodata movement")
	}
	srv.BeginDoorSiege(t, hpDoorID)

	srv.HitDoor(t, hpDoorID, objID, 10_000)
	broken := wantDoorStatus(gate.ObjectID(), 6, 0)
	assertDoorStatusUpdates(t, doorStatusUpdates(t, c, gate.ObjectID()), broken, broken, broken)
	if !gate.Dead() || gate.Opened() || srv.DoorRegenerating(t, hpDoorID) {
		t.Fatalf("broken door dead %v, opened %v, regenerating %v; want dead, closed, not regenerating",
			gate.Dead(), gate.Opened(), srv.DoorRegenerating(t, hpDoorID))
	}
	if blocks() {
		t.Fatal("broken closed door still blocks geodata movement")
	}

	srv.HitDoor(t, hpDoorID, objID, 100)
	if srv.WorldObjects.SetDoorOpen(hpDoorID, true) {
		t.Fatal("broken door opened")
	}
	srv.AdvanceDoorRegen(t, 10*time.Minute)
	if got := doorStatusUpdates(t, c, gate.ObjectID()); len(got) != 0 {
		t.Fatalf("broken door answered a hit, an open or a regeneration tick: sent %+v", got)
	}

	if !srv.WorldObjects.ReviveDoor(hpDoorID, 0.7) {
		t.Fatal("ReviveDoor = false, want the broken door revived")
	}
	// 420/600: 6 - ceil(4.2) = stage 1, then Revive.
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeDoorStatusUpdate, "DoorStatusUpdate")
	r := wireReader(frame[1:])
	got := doorStatusUpdate{
		objectID: r.ReadInt32(), closed: r.ReadInt32(), damage: r.ReadInt32(), showHP: r.ReadInt32(),
		doorID: r.ReadInt32(), maxHP: r.ReadInt32(), hp: r.ReadInt32(),
	}
	if want := wantDoorStatus(gate.ObjectID(), 1, 420); got != want {
		t.Fatalf("revive DoorStatusUpdate = %+v, want %+v", got, want)
	}
	frame = c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeRevive, "Revive")
	if id := wireReader(frame[1:]).ReadInt32(); id != gate.ObjectID() {
		t.Fatalf("Revive object id = %d, want %d", id, gate.ObjectID())
	}
	if got := doorStatusUpdates(t, c, gate.ObjectID()); len(got) != 0 {
		t.Fatalf("revive sent extra %+v", got)
	}
	if gate.Dead() || gate.Opened() || !blocks() || !srv.DoorRegenerating(t, hpDoorID) {
		t.Fatalf("revived door dead %v, opened %v, blocks %v, regenerating %v; want alive, closed, blocking, regenerating",
			gate.Dead(), gate.Opened(), blocks(), srv.DoorRegenerating(t, hpDoorID))
	}
	if srv.WorldObjects.ReviveDoor(hpDoorID, 0.7) {
		t.Fatal("ReviveDoor revived a standing door")
	}
}
