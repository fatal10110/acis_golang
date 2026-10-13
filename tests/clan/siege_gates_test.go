package clan

import (
	"math"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: a castle's doors are the doors.xml doors its castles.xml
// <gates> names (Residence.setDoors). The siege runs them:
//   - startSiege closes them (Residence.closeDoors -> Door.closeMe).
//   - endSiege and midVictory stand them again (Castle.spawnDoors): a dead
//     door revives (Door.doRevive: template open state, then
//     Creature.doRevive: HP to RespawnRestoreHP of the maximum, broadcast,
//     then Revive), each closes, then setHp to the maximum, or to
//     getMaxHp() / 2 (integer) at mid-victory; every setHp broadcasts
//     DoorStatusUpdate.
//   - Door.isAttackableBy: during the castle's siege in progress only a
//     playable of an ATTACKER clan, and a wall only through a siege summon
//     other than the Swoop Cannon. DoorInfo's show-HP field is
//     isAttackableBy(viewer).
//   - Door.reduceCurrentHp: HP drops only during the castle's siege in
//     progress, from any attacker but the Swoop Cannon.
//   - Door.doDie during the siege in progress tells the DEFENDER side (the
//     owner and the defenders) CASTLE_WALL_DAMAGED (1686) for a wall,
//     CASTLE_GATE_BROKEN_DOWN (288) for a gate.
//   - DoorStatusUpdate's show-HP field is set while the castle's siege is
//     in progress.

// Gludio Castle's outer gate and first wall, under their doors.xml ids and
// names, standing by the siege world's players with 600 HP.
const (
	gludioGateID = 19210001
	gludioWallID = 19210003
	gateDoorHP   = 600
)

// gateWorldAt is where the gate world's players stand, the doors beside
// them.
var gateWorldAt = location.Location{X: 20000, Y: 20000, Z: -1000}

func gludioDoor(id int, name string, kind door.Kind, dx int) *door.Template {
	x, y := gateWorldAt.X+dx, gateWorldAt.Y
	return &door.Template{
		ID: id, Name: name, Kind: kind, Level: 1,
		Position: location.Location{X: x, Y: y, Z: gateWorldAt.Z},
		Coordinates: []location.Point{
			{X: x - 8, Y: y - 8}, {X: x + 8, Y: y - 8}, {X: x + 8, Y: y + 8}, {X: x - 8, Y: y + 8},
		},
		HP: gateDoorHP, PDef: 100, MDef: 100, Height: 32,
	}
}

func gludioGates() []*door.Template {
	return []*door.Template{
		gludioDoor(gludioGateID, "gludio_castle_outter_001", door.KindDoor, 100),
		gludioDoor(gludioWallID, "gludio_castle_wall_001", door.KindWall, -100),
	}
}

// gateWorld boots the shipped castles with their sieges and Gludio's gate
// and wall. The founder leads Knights, registered to attack Gludio; the
// rival leads Rivals, Gludio's lord. Both stand in the world by the doors,
// their entry frames drained. extra adds boot options.
func gateWorld(t *testing.T, extra ...gameservertest.Option) *clanWorld {
	t.Helper()
	datapack.Require(t)
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	at := func(who string) string {
		return `UPDATE characters SET x = ` + itoa(int32(gateWorldAt.X)) + `, y = ` + itoa(int32(gateWorldAt.Y)) +
			`, z = ` + itoa(int32(gateWorldAt.Z)) + ` WHERE ` + who
	}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Founder", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCastles(castles),
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithDoors(gludioGates()...),
		seedRival(t),
		seedAllianceClans(t,
			`UPDATE clan_data SET hasCastle = 1 WHERE clan_id = `+itoa(rivalsClanID),
			`INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (1, `+itoa(knightsClanID)+`, 'ATTACKER')`,
			at(`char_name = 'Founder'`),
			at(`obj_Id = `+itoa(rivalLeaderID)),
		),
	}, extra...)...)
	w := &clanWorld{srv: srv, leader: srv.Client, leaderID: srv.SoleObjectID(t), memberID: rivalLeaderID, at: gateWorldAt}
	w.member = srv.DialClient(t, "player2", 1)
	startInWorld(t, w.leader)
	startInWorld(t, w.member)
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	return w
}

func gludioSiegeOf(t *testing.T, w *clanWorld) *siege.Siege {
	t.Helper()
	s, ok := w.srv.Sieges.Get(1)
	if !ok {
		t.Fatal("no Gludio siege")
	}
	return s
}

func gateObject(t *testing.T, w *clanWorld, id int) *door.Object {
	t.Helper()
	obj, ok := w.srv.WorldObjects.Door(id)
	if !ok {
		t.Fatalf("door %d is not spawned", id)
	}
	return obj
}

// gateFrame is one door frame a player got: DoorStatusUpdate's fields,
// DoorInfo's object, door id, show-HP flag, closed flag and HP, or a
// Revive's object.
type gateFrame struct {
	opcode                           byte
	objectID, closed, damage, showHP int32
	doorID, maxHP, hp                int32
}

// gateFrames decodes the frames of frames about the door objectID, in
// order: its DoorStatusUpdate, DoorInfo and Revive frames.
func gateFrames(t *testing.T, frames [][]byte, objectID int32) []gateFrame {
	t.Helper()
	var out []gateFrame
	for _, f := range frames {
		if len(f) < 5 || wire.NewReader(f[1:]).ReadInt32() != objectID {
			continue
		}
		r := wire.NewReader(f[1:])
		g := gateFrame{opcode: f[0], objectID: r.ReadInt32()}
		switch f[0] {
		case serverpackets.OpcodeDoorStatusUpdate:
			g.closed, g.damage, g.showHP = r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
			g.doorID, g.maxHP, g.hp = r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
		case serverpackets.OpcodeDoorInfo:
			g.doorID, g.showHP = r.ReadInt32(), r.ReadInt32()
			r.ReadInt32()
			g.closed, g.maxHP, g.hp = r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
		case serverpackets.OpcodeRevive:
		default:
			continue
		}
		if err := r.Err(); err != nil {
			t.Fatalf("decode door frame %#x: %v", f[0], err)
		}
		out = append(out, g)
	}
	return out
}

// status is the DoorStatusUpdate of a closed door with hp left.
func status(objectID int32, doorID int, hp, showHP int32) gateFrame {
	damage := max(0, min(6, 6-int32(math.Ceil(float64(hp)/gateDoorHP*6))))
	return gateFrame{
		opcode: serverpackets.OpcodeDoorStatusUpdate, objectID: objectID, closed: 1, damage: damage, showHP: showHP,
		doorID: int32(doorID), maxHP: gateDoorHP, hp: hp,
	}
}

func revive(objectID int32) gateFrame {
	return gateFrame{opcode: serverpackets.OpcodeRevive, objectID: objectID}
}

func expectGateFrames(t *testing.T, what string, got []gateFrame, want ...gateFrame) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: frames %+v, want %+v", what, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: frame %d = %+v, want %+v", what, i, got[i], want[i])
		}
	}
}

// systemMessages lists the ids of the SystemMessage frames of frames.
func systemMessages(frames [][]byte) []int32 {
	var ids []int32
	for _, f := range frames {
		if len(f) >= 5 && f[0] == serverpackets.OpcodeSystemMessage {
			ids = append(ids, wire.NewReader(f[1:]).ReadInt32())
		}
	}
	return ids
}

func containsID(ids []int32, id int32) bool { return slices.Contains(ids, id) }

// recordInfoDoors sends RequestRecordInfo for c and returns the DoorInfo
// show-HP flag it got for each door object.
func recordInfoDoors(t *testing.T, c *testsupport.ScriptedClient, objectIDs ...int32) []int32 {
	t.Helper()
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestRecordInfo).Bytes())
	frames := drainFrames(t, c)
	out := make([]int32, len(objectIDs))
	for i, id := range objectIDs {
		out[i] = -1
		for _, g := range gateFrames(t, frames, id) {
			if g.opcode == serverpackets.OpcodeDoorInfo {
				out[i] = g.showHP
			}
		}
	}
	return out
}

// TestSiegeGatesThroughSiege drives Gludio's siege over its gate and
// wall: before the siege no hit lowers the gate and no one may attack it;
// the siege's start closes the opened gate, its status now showing HP; the
// attacking founder may attack the gate but not the wall, the defending
// rival neither; the gate broken down tells the rival 288 and the wall
// 1686, the founder neither; the siege's end revives both, at the
// configured 70% then full HP, no longer showing it.
func TestSiegeGatesThroughSiege(t *testing.T) {
	t.Parallel()
	w := gateWorld(t)
	s := gludioSiegeOf(t, w)
	gate, wall := gateObject(t, w, gludioGateID), gateObject(t, w, gludioWallID)

	w.srv.HitDoor(t, gludioGateID, w.leaderID, 100)
	if !w.srv.WorldObjects.SetDoorOpen(gludioGateID, true) {
		t.Fatal("the gate did not open before the siege")
	}
	frames := drainFrames(t, w.leader)
	expectGateFrames(t, "gate before the siege", gateFrames(t, frames, gate.ObjectID()), gateFrame{
		opcode: serverpackets.OpcodeDoorStatusUpdate, objectID: gate.ObjectID(), doorID: gludioGateID, maxHP: gateDoorHP, hp: gateDoorHP,
	})
	if got := recordInfoDoors(t, w.leader, gate.ObjectID(), wall.ObjectID()); got[0] != 0 || got[1] != 0 {
		t.Fatalf("DoorInfo show HP before the siege = %v, want [0 0]", got)
	}
	drainFrames(t, w.member)

	s.Start()
	if !s.InProgress() {
		t.Fatal("Gludio's siege did not start")
	}
	frames = drainFrames(t, w.leader)
	expectGateFrames(t, "gate at the siege's start", gateFrames(t, frames, gate.ObjectID()), status(gate.ObjectID(), gludioGateID, gateDoorHP, 1))
	expectGateFrames(t, "wall at the siege's start", gateFrames(t, frames, wall.ObjectID()))
	drainFrames(t, w.member)

	if got := recordInfoDoors(t, w.leader, gate.ObjectID(), wall.ObjectID()); got[0] != 1 || got[1] != 0 {
		t.Fatalf("attacker DoorInfo show HP (gate, wall) = %v, want [1 0]", got)
	}
	if got := recordInfoDoors(t, w.member, gate.ObjectID(), wall.ObjectID()); got[0] != 0 || got[1] != 0 {
		t.Fatalf("defender DoorInfo show HP (gate, wall) = %v, want [0 0]", got)
	}

	w.srv.HitDoor(t, gludioGateID, w.leaderID, 100)
	expectGateFrames(t, "gate hit", gateFrames(t, drainFrames(t, w.leader), gate.ObjectID()), status(gate.ObjectID(), gludioGateID, 500, 1))
	drainFrames(t, w.member)

	w.srv.HitDoor(t, gludioGateID, w.leaderID, 500)
	leaderFrames, memberFrames := drainFrames(t, w.leader), drainFrames(t, w.member)
	if !gate.Dead() {
		t.Fatal("the gate stands after losing all its HP")
	}
	if ids := systemMessages(memberFrames); !containsID(ids, siege.MsgCastleGateBrokenDown) || containsID(ids, siege.MsgCastleWallDamaged) {
		t.Fatalf("defender messages on the gate's death = %v, want 288 alone", ids)
	}
	if ids := systemMessages(leaderFrames); containsID(ids, siege.MsgCastleGateBrokenDown) {
		t.Fatalf("attacker told the gate broke: %v", ids)
	}
	w.srv.HitDoor(t, gludioWallID, w.leaderID, gateDoorHP)
	leaderFrames, memberFrames = drainFrames(t, w.leader), drainFrames(t, w.member)
	if ids := systemMessages(memberFrames); !containsID(ids, siege.MsgCastleWallDamaged) || containsID(ids, siege.MsgCastleGateBrokenDown) {
		t.Fatalf("defender messages on the wall's death = %v, want 1686 alone", ids)
	}
	if ids := systemMessages(leaderFrames); containsID(ids, siege.MsgCastleWallDamaged) {
		t.Fatalf("attacker told the wall broke: %v", ids)
	}

	s.End()
	if s.InProgress() {
		t.Fatal("Gludio's siege still in progress after its end")
	}
	frames = drainFrames(t, w.leader)
	for _, d := range []*door.Object{gate, wall} {
		expectGateFrames(t, "door at the siege's end", gateFrames(t, frames, d.ObjectID()),
			status(d.ObjectID(), d.DoorID(), 420, 0), revive(d.ObjectID()), status(d.ObjectID(), d.DoorID(), gateDoorHP, 0))
		if d.Dead() || d.Opened() || d.HP() != gateDoorHP {
			t.Fatalf("door %d after the siege: dead %t, open %t, HP %d", d.DoorID(), d.Dead(), d.Opened(), d.HP())
		}
	}
	w.srv.HitDoor(t, gludioGateID, w.leaderID, 100)
	if gate.HP() != gateDoorHP {
		t.Fatalf("a hit after the siege lowered the gate to %d", gate.HP())
	}
}

// TestSiegeGatesAtMidVictory takes Gludio from its lord mid-siege: the
// opened, damaged gate closes and is set to half its HP, and the broken
// wall revives and is set to half its HP too, both still showing HP.
func TestSiegeGatesAtMidVictory(t *testing.T) {
	t.Parallel()
	w := gateWorld(t)
	s := gludioSiegeOf(t, w)
	gate, wall := gateObject(t, w, gludioGateID), gateObject(t, w, gludioWallID)
	s.Start()
	w.srv.HitDoor(t, gludioGateID, w.leaderID, 50)
	w.srv.WorldObjects.SetDoorOpen(gludioGateID, true)
	w.srv.HitDoor(t, gludioWallID, w.leaderID, gateDoorHP)
	drainFrames(t, w.leader)
	drainFrames(t, w.member)

	knights, ok := w.srv.Clans.Table().Get(knightsClanID)
	if !ok {
		t.Fatal("no Knights clan")
	}
	if _, ok := w.srv.Castles.SetOwner(s.Castle(), knights); !ok {
		t.Fatal("Knights did not take Gludio")
	}
	s.MidVictory()
	frames := drainFrames(t, w.leader)
	expectGateFrames(t, "gate at mid-victory", gateFrames(t, frames, gate.ObjectID()),
		status(gate.ObjectID(), gludioGateID, 550, 1), status(gate.ObjectID(), gludioGateID, gateDoorHP/2, 1))
	expectGateFrames(t, "wall at mid-victory", gateFrames(t, frames, wall.ObjectID()),
		status(wall.ObjectID(), gludioWallID, 420, 1), revive(wall.ObjectID()), status(wall.ObjectID(), gludioWallID, gateDoorHP/2, 1))
	if gate.Opened() || wall.Dead() || gate.HP() != gateDoorHP/2 || wall.HP() != gateDoorHP/2 {
		t.Fatalf("after mid-victory gate open %t HP %d, wall dead %t HP %d", gate.Opened(), gate.HP(), wall.Dead(), wall.HP())
	}
}

// TestSiegeGatesAfterDoorReload reloads the doors during Gludio's siege:
// the new gate is Gludio's gate still, a hit lowering its HP with the HP
// shown.
func TestSiegeGatesAfterDoorReload(t *testing.T) {
	t.Parallel()
	w := gateWorld(t)
	s := gludioSiegeOf(t, w)
	s.Start()
	table, err := door.NewTable(gludioGates())
	if err != nil {
		t.Fatal(err)
	}
	old := gateObject(t, w, gludioGateID)
	if err := w.srv.WorldObjects.ReloadDoors(table); err != nil {
		t.Fatalf("reload doors: %v", err)
	}
	gate := gateObject(t, w, gludioGateID)
	if gate == old {
		t.Fatal("the reload kept the old gate")
	}
	drainFrames(t, w.leader)
	w.srv.HitDoor(t, gludioGateID, w.leaderID, 100)
	expectGateFrames(t, "reloaded gate hit", gateFrames(t, drainFrames(t, w.leader), gate.ObjectID()), status(gate.ObjectID(), gludioGateID, 500, 1))
}
