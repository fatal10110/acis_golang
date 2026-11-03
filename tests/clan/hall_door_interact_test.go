package clan

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: Creature.onAction attacks a selected door only when it may be
// attacked, and otherwise runs the interact intention (PlayerAI.thinkInteract:
// ActionFailed, the approach walk, MoveToPawn at the interaction distance,
// then Door.onInteract). Door.onInteract asks a member of the clan owning
// the door's clan hall who holds CHP_ENTRY_EXIT_RIGHTS
// (Door.canBeManuallyOpenedBy) ConfirmDlg 1140 to open a closed door or
// 1141 to close an open one, keeping the door as the player's requested
// gate. DlgAnswer 1140/1141 (Player.activateGate) opens or closes it on a
// yes while the player still has it selected and may still open it, and
// drops the request on any answer.

const (
	hallDoorHall   = 61
	hallDoorNear   = 25_000_001
	hallDoorFar    = 25_000_002
	hallDoorMember = 0x70000061
	hallDoorRank   = 5
)

// hallDoorSetup is the world a hall door interact boots with: Newbie leads
// the clan owning the hall unless rank is set, which makes Newbie a member
// of that grade holding privs; foreign has another clan own the hall.
type hallDoorSetup struct {
	rank, privs int
	foreign     bool
	nearOpen    bool
}

// hallDoorTable is one clan hall whose gates are the two test doors.
func hallDoorTable(t *testing.T) *hallmodel.Table {
	t.Helper()
	h, err := hallmodel.NewHall(hallmodel.HallAttrs{
		ID: hallDoorHall, Alias: "test_hall", Name: "Test Hall", Description: "A hall", Town: "Gludio",
		Lease: 100, Grade: 1, Gates: []string{"test_hall_gate_near", "test_hall_gate_far"},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	table, err := hallmodel.NewTable([]*hallmodel.Hall{h})
	if err != nil {
		t.Fatal(err)
	}
	return table
}

func hallDoor(id int, name string, x int, open bool) *door.Template {
	return &door.Template{
		ID: id, Name: name, Kind: door.KindDoor, Level: 1,
		Position: location.Location{X: x, Y: 100, Z: gameservertest.SpawnZ},
		Coordinates: []location.Point{
			{X: x - 8, Y: 92}, {X: x + 8, Y: 92}, {X: x + 8, Y: 108}, {X: x - 8, Y: 108},
		},
		HP: 100, PDef: 10, MDef: 10, Height: 32, Opened: open, OpenKind: door.OpenNPC,
	}
}

type hallDoorWorld struct {
	srv  *gameservertest.Server
	c    *testsupport.ScriptedClient
	near *door.Object
	far  *door.Object
}

func bootHallDoor(t *testing.T, s hallDoorSetup) *hallDoorWorld {
	t.Helper()
	halls := hallDoorTable(t)
	decos, err := hallmodel.NewDecoTable(nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithClanHalls(halls, decos),
		gameservertest.WithResidences(halls, nil),
		gameservertest.WithDoors(
			hallDoor(hallDoorNear, "test_hall_gate_near", -60, s.nearOpen),
			hallDoor(hallDoorFar, "test_hall_gate_far", 600, false),
		),
		gameservertest.WithClanSeed(func(db *sql.DB) { seedHallDoor(t, db, s) }),
	)
	startInWorld(t, srv.Client)
	w := &hallDoorWorld{srv: srv, c: srv.Client}
	var ok bool
	if w.near, ok = srv.WorldObjects.Door(hallDoorNear); !ok {
		t.Fatal("near door not spawned")
	}
	if w.far, ok = srv.WorldObjects.Door(hallDoorFar); !ok {
		t.Fatal("far door not spawned")
	}
	return w
}

func seedHallDoor(t *testing.T, db *sql.DB, s hallDoorSetup) {
	t.Helper()
	exec := func(q string, args ...any) {
		if _, err := db.ExecContext(context.Background(), q, args...); err != nil {
			t.Fatalf("seed hall door: %s: %v", q, err)
		}
	}
	if s.rank == 0 {
		exec("UPDATE characters SET clanid = ? WHERE char_name = 'Newbie'", hallClanID)
		exec(`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id)
			SELECT ?, 'Hallkeepers', 4, obj_Id FROM characters WHERE char_name = 'Newbie'`, hallClanID)
	} else {
		exec("UPDATE characters SET clanid = ?, power_grade = ? WHERE char_name = 'Newbie'", hallClanID, s.rank)
		exec(`INSERT INTO characters (account_name, obj_Id, char_name, level, clanid, power_grade) VALUES ('keeper', ?, 'Keeper', 40, ?, 0)`,
			hallDoorMember, hallClanID)
		exec("INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) VALUES (?, 'Hallkeepers', 4, ?)", hallClanID, hallDoorMember)
		exec("INSERT INTO clan_privs (clan_id, ranking, privs) VALUES (?, ?, ?)", hallClanID, s.rank, s.privs)
	}
	owner := hallClanID
	if s.foreign {
		owner = rivalHallClanID
		exec("INSERT INTO clan_data (clan_id, clan_name, clan_level) VALUES (?, 'Rivals', 4)", rivalHallClanID)
	}
	exec("INSERT INTO clanhall (id, ownerId, paid, paidUntil) VALUES (?, ?, 1, ?)", hallDoorHall, owner, time.Now().Add(30*24*time.Hour).UnixMilli())
}

// click sends an Action on d and returns the frames up to the server going
// quiet.
func (w *hallDoorWorld) click(t *testing.T, d *door.Object) [][]byte {
	t.Helper()
	x, y, z := d.Position()
	w.c.Send(encodeAction(d.ObjectID(), location.Location{X: x, Y: y, Z: z}))
	return drainFrames(t, w.c)
}

// answer sends DlgAnswer for message id with answer and returns the frames
// up to the server going quiet.
func (w *hallDoorWorld) answer(t *testing.T, id, answer int32) [][]byte {
	t.Helper()
	pw := wire.NewPacketWriter(clientpackets.OpcodeDlgAnswer)
	pw.WriteInt32(id)
	pw.WriteInt32(answer)
	pw.WriteInt32(0)
	w.c.Send(pw.Bytes())
	return drainFrames(t, w.c)
}

// gateQuestion is the ConfirmDlg asking message id, with no parameter.
func gateQuestion(id int32) []byte {
	b := []byte{serverpackets.OpcodeConfirmDlg}
	for _, v := range []int32{id, 0, 0, 0} {
		b = binary.LittleEndian.AppendUint32(b, uint32(v))
	}
	return b
}

// wantInteract fails unless frames are the interact of a player in reach:
// ActionFailed, then MoveToPawn onto d, then question when it is set and
// nothing more.
func wantInteract(t *testing.T, frames [][]byte, d *door.Object, question []byte) {
	t.Helper()
	want := 2
	if question != nil {
		want = 3
	}
	if len(frames) != want || frames[0][0] != serverpackets.OpcodeActionFailed || frames[1][0] != serverpackets.OpcodeMoveToPawn {
		t.Fatalf("interact frames = % x, want ActionFailed, MoveToPawn%s", opcodes(frames), map[bool]string{true: ", ConfirmDlg"}[question != nil])
	}
	if target := int32(binary.LittleEndian.Uint32(frames[1][5:])); target != d.ObjectID() {
		t.Fatalf("MoveToPawn target = %d, want door %d", target, d.ObjectID())
	}
	if question != nil && !bytes.Equal(frames[2], question) {
		t.Fatalf("question = % x, want % x", frames[2], question)
	}
}

func TestHallDoorInteractAsksOwnerAndAnswers(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		setup    hallDoorSetup
		question int32
	}{
		{name: "leader opens a closed gate", setup: hallDoorSetup{}, question: serverpackets.ConfirmDlgOpenGate},
		{name: "leader closes an open gate", setup: hallDoorSetup{nearOpen: true}, question: serverpackets.ConfirmDlgCloseGate},
		{name: "member with entry rights", setup: hallDoorSetup{rank: hallDoorRank, privs: int(clan.PrivHallEntryExit)}, question: serverpackets.ConfirmDlgOpenGate},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := bootHallDoor(t, tc.setup)
			w.click(t, w.near)
			wantInteract(t, w.click(t, w.near), w.near, gateQuestion(tc.question))
			w.answer(t, tc.question, 1)
			if open := w.near.Opened(); open != !tc.setup.nearOpen {
				t.Fatalf("gate open = %v after a yes, want %v", open, !tc.setup.nearOpen)
			}
		})
	}
}

// TestHallDoorAnswerNoDropsRequest answers no: the gate stays shut, and a
// yes sent afterwards finds no request to act on.
func TestHallDoorAnswerNoDropsRequest(t *testing.T) {
	t.Parallel()
	w := bootHallDoor(t, hallDoorSetup{})
	w.click(t, w.near)
	wantInteract(t, w.click(t, w.near), w.near, gateQuestion(serverpackets.ConfirmDlgOpenGate))
	w.answer(t, serverpackets.ConfirmDlgOpenGate, 0)
	w.answer(t, serverpackets.ConfirmDlgOpenGate, 1)
	if w.near.Opened() {
		t.Fatal("gate opened by a yes sent after the request was answered no")
	}
}

// TestHallDoorAnswerNeedsGateSelected deselects the gate before saying yes:
// the gate stays shut.
func TestHallDoorAnswerNeedsGateSelected(t *testing.T) {
	t.Parallel()
	w := bootHallDoor(t, hallDoorSetup{})
	w.click(t, w.near)
	wantInteract(t, w.click(t, w.near), w.near, gateQuestion(serverpackets.ConfirmDlgOpenGate))
	w.click(t, w.far)
	w.answer(t, serverpackets.ConfirmDlgOpenGate, 1)
	if w.near.Opened() {
		t.Fatal("gate opened by a yes once the player selected another door")
	}
}

// TestHallDoorInteractRefusesOthers: a member lacking the entry rights, or
// a member of a clan not owning the hall, walks up to the gate and is
// asked nothing; a yes answer they send anyway opens nothing.
func TestHallDoorInteractRefusesOthers(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		setup hallDoorSetup
	}{
		{name: "no entry rights", setup: hallDoorSetup{rank: hallDoorRank, privs: int(clan.PrivHallFunctions)}},
		{name: "foreign clan", setup: hallDoorSetup{foreign: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := bootHallDoor(t, tc.setup)
			w.click(t, w.near)
			wantInteract(t, w.click(t, w.near), w.near, nil)
			w.answer(t, serverpackets.ConfirmDlgOpenGate, 1)
			if w.near.Opened() {
				t.Fatal("gate opened for a player who may not open it")
			}
		})
	}
}

// TestHallDoorInteractWalksToGate clicks a gate out of reach: the player
// walks to it and, on arrival, is asked to open it.
func TestHallDoorInteractWalksToGate(t *testing.T) {
	t.Parallel()
	w := bootHallDoor(t, hallDoorSetup{})
	w.click(t, w.far)
	x, y, z := w.far.Position()
	w.c.Send(encodeAction(w.far.ObjectID(), location.Location{X: x, Y: y, Z: z}))
	first := w.c.Read()
	if first[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("first answer = %#x, want ActionFailed", first[0])
	}
	question := gateQuestion(serverpackets.ConfirmDlgOpenGate)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		frame := w.c.ReadWithTimeout(time.Second)
		if frame == nil {
			continue
		}
		if frame[0] == serverpackets.OpcodeConfirmDlg {
			if !bytes.Equal(frame, question) {
				t.Fatalf("question = % x, want % x", frame, question)
			}
			px, _, _ := w.srv.PlayerPosition(t, w.srv.SoleObjectID(t))
			if px < x-200 {
				t.Fatalf("asked at x=%d, still far from the gate at x=%d", px, x)
			}
			return
		}
	}
	t.Fatal("never asked to open the gate after walking to it")
}
